package worker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/docinsight/backend/internal/chunker"
	"github.com/docinsight/backend/internal/config"
	"github.com/docinsight/backend/internal/embedder"
	"github.com/docinsight/backend/internal/events"
	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/ocr"
	pdfpkg "github.com/docinsight/backend/internal/pdf"
	"github.com/docinsight/backend/internal/queue"
	"github.com/docinsight/backend/internal/scraper"
	"github.com/docinsight/backend/internal/store"
	"golang.org/x/sync/errgroup"
)

type Processor struct {
	store     store.Store
	extractor pdfpkg.Extractor
	scraper   scraper.Scraper
	embedder  embedder.Embedder
	ocr       *ocr.Processor
	broker    *events.Broker
	queue     *queue.Queue
	cfg       *config.Config
}

func NewProcessor(
	s store.Store,
	ext pdfpkg.Extractor,
	scr scraper.Scraper,
	emb embedder.Embedder,
	ocrProc *ocr.Processor,
	broker *events.Broker,
	q *queue.Queue,
	cfg *config.Config,
) *Processor {
	return &Processor{
		store:     s,
		extractor: ext,
		scraper:   scr,
		embedder:  emb,
		ocr:       ocrProc,
		broker:    broker,
		queue:     q,
		cfg:       cfg,
	}
}

func (p *Processor) Process(ctx context.Context, job queue.Job) {
	docID := job.DocumentID
	logger := slog.With("document_id", docID, "job_id", job.ID)

	doc, err := p.store.GetDocument(ctx, docID)
	if err != nil || doc == nil {
		logger.Error("failed to get document", "error", err)
		return
	}

	defer func() {
		if r := recover(); r != nil {
			logger.Error("processor panicked", "panic", r)
			errMsg := fmt.Sprintf("panic: %v", r)
			_ = p.store.UpdateDocumentStatus(ctx, docID, model.StatusFailed, &errMsg)
		}
	}()

	if err := p.processDocument(ctx, doc, logger); err != nil {
		logger.Error("processing failed", "error", err)
		errMsg := err.Error()
		_ = p.store.UpdateDocumentStatus(ctx, docID, model.StatusFailed, &errMsg)
		p.publishEvent("document.failed", map[string]interface{}{
			"document_id": docID.String(),
			"name":        doc.Name,
			"error":       errMsg,
		})

		// Retry if attempts remaining
		if job.Attempts < job.MaxRetries {
			retryJob := job
			retryJob.Attempts++
			backoff := time.Duration(1<<uint(retryJob.Attempts)) * time.Second
			logger.Info("retrying job", "attempt", retryJob.Attempts, "backoff", backoff)

			go func() {
				select {
				case <-time.After(backoff):
					_ = p.store.UpdateDocumentStatus(ctx, docID, model.StatusProcessing, nil)
					if err := p.queue.Enqueue(retryJob); err != nil {
						logger.Error("failed to re-enqueue job", "error", err)
					}
				case <-ctx.Done():
				}
			}()
		}
	}
}

func (p *Processor) processDocument(ctx context.Context, doc *model.Document, logger *slog.Logger) error {
	// Clean up any previous chunks from a failed attempt
	if err := p.store.DeleteChunksByDocumentID(ctx, doc.ID); err != nil {
		return fmt.Errorf("cleanup previous chunks: %w", err)
	}

	// Stage 1: Extract text from source
	fileData, err := os.ReadFile(doc.FilePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	var result *pdfpkg.ExtractResult
	if doc.SourceType == model.SourceTypeWeb && p.scraper != nil {
		logger.Info("stage 1: extracting web page text")
		sourceURL := ""
		if doc.SourceURL != nil {
			sourceURL = *doc.SourceURL
		}
		result, err = p.scraper.ExtractFromHTML(fileData, sourceURL)
		if err != nil {
			return fmt.Errorf("extract web content: %w", err)
		}
	} else {
		logger.Info("stage 1: extracting PDF text")
		result, err = p.extractor.Extract(fileData)
		if err != nil {
			return fmt.Errorf("extract PDF: %w", err)
		}

		// Check for scanned/image PDFs and fall back to OCR
		if p.ocr != nil && p.cfg.OCREnabled && ocr.IsTextSparse(result.Pages, int64(len(fileData)), p.cfg.OCRMinTextRatio) {
			logger.Info("stage 1b: text is sparse, attempting OCR fallback")
			ocrResult, ocrErr := p.ocr.ExtractFromPDF(ctx, fileData)
			if ocrErr != nil {
				logger.Warn("OCR fallback failed, using original extraction", "error", ocrErr)
			} else {
				result = ocrResult
			}
		}
	}

	if err := p.store.UpdateDocumentPageCount(ctx, doc.ID, result.PageCount); err != nil {
		return fmt.Errorf("update page count: %w", err)
	}

	if len(result.Pages) == 0 {
		return fmt.Errorf("no text content could be extracted from this PDF")
	}

	// Stage 2: Create chunks
	logger.Info("stage 2: creating chunks", "pages", len(result.Pages))
	textChunks := chunker.CreateChunks(result.Pages, p.cfg.ChunkSize, p.cfg.ChunkOverlap)
	if len(textChunks) == 0 {
		return fmt.Errorf("no chunks created from extracted text")
	}
	logger.Info("chunks created", "count", len(textChunks))

	// Stage 3: Insert chunks
	logger.Info("stage 3: inserting chunks")
	modelChunks := chunker.ToModelChunks(textChunks, doc.ID)
	chunkIDs, err := p.store.InsertChunks(ctx, modelChunks)
	if err != nil {
		return fmt.Errorf("insert chunks: %w", err)
	}

	// Stage 4: Generate embeddings concurrently
	logger.Info("stage 4: generating embeddings", "chunks", len(chunkIDs))
	texts := make([]string, len(textChunks))
	for i, tc := range textChunks {
		texts[i] = tc.Content
	}

	allEmbeddings, err := p.generateEmbeddingsConcurrently(ctx, texts)
	if err != nil {
		return fmt.Errorf("generate embeddings: %w", err)
	}

	// Stage 5: Insert embeddings in batches
	logger.Info("stage 5: storing embeddings")
	batchSize := 50
	for i := 0; i < len(chunkIDs); i += batchSize {
		end := i + batchSize
		if end > len(chunkIDs) {
			end = len(chunkIDs)
		}
		if err := p.store.InsertEmbeddings(ctx, chunkIDs[i:end], allEmbeddings[i:end]); err != nil {
			return fmt.Errorf("insert embeddings batch: %w", err)
		}
	}

	// Stage 6: Mark complete
	if err := p.store.UpdateDocumentStatus(ctx, doc.ID, model.StatusCompleted, nil); err != nil {
		return fmt.Errorf("update document status: %w", err)
	}

	logger.Info("processing complete")
	p.publishEvent("document.completed", map[string]interface{}{
		"document_id": doc.ID.String(),
		"name":        doc.Name,
	})
	return nil
}

func (p *Processor) publishEvent(eventType string, data interface{}) {
	if p.broker != nil {
		p.broker.Publish(events.Event{Type: eventType, Data: data})
	}
}

func (p *Processor) generateEmbeddingsConcurrently(ctx context.Context, texts []string) ([][]float32, error) {
	batchSize := p.cfg.EmbeddingBatchSize
	concurrency := p.cfg.EmbeddingConcurrency

	type batchResult struct {
		index      int
		embeddings [][]float32
	}

	numBatches := (len(texts) + batchSize - 1) / batchSize
	results := make([]batchResult, numBatches)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for i := 0; i < numBatches; i++ {
		batchIdx := i
		start := batchIdx * batchSize
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[start:end]

		g.Go(func() error {
			embs, err := p.embedder.Embed(gctx, batch)
			if err != nil {
				return fmt.Errorf("embed batch %d: %w", batchIdx, err)
			}
			results[batchIdx] = batchResult{index: batchIdx, embeddings: embs}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Flatten results in order
	allEmbeddings := make([][]float32, 0, len(texts))
	for _, r := range results {
		allEmbeddings = append(allEmbeddings, r.embeddings...)
	}

	return allEmbeddings, nil
}
