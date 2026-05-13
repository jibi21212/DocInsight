package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docinsight/backend/internal/config"
	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/pdf"
	"github.com/docinsight/backend/internal/queue"
	"github.com/docinsight/backend/internal/scraper"
	"github.com/docinsight/backend/internal/store"
	"github.com/google/uuid"
)

// --- Mock implementations ---

type mockExtractor struct {
	result *pdf.ExtractResult
	err    error
}

func (m *mockExtractor) Extract(data []byte) (*pdf.ExtractResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

type mockEmbedder struct {
	embeddings [][]float32
	err        error
}

func (m *mockEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Return one embedding per text
	result := make([][]float32, len(texts))
	for i := range texts {
		if i < len(m.embeddings) {
			result[i] = m.embeddings[i]
		} else {
			result[i] = []float32{1.0, 0.0, 0.0, 0.0}
		}
	}
	return result, nil
}

func (m *mockEmbedder) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	results, err := m.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return results[0], nil
}

func newTestStore(t *testing.T) store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newTestConfig() *config.Config {
	return &config.Config{
		UploadDir:            "./uploads",
		MaxUploadSizeMB:     10,
		WorkerCount:         2,
		QueueCapacity:       10,
		MaxRetries:          3,
		EmbeddingSidecarURL: "http://localhost:8000",
		EmbeddingBatchSize:  32,
		EmbeddingConcurrency: 2,
		ChunkSize:           1000,
		ChunkOverlap:        200,
		SearchTopK:          10,
		SimilarityThreshold: 0.5,
	}
}

// --- Pool Tests ---

func TestPoolStartAndShutdown(t *testing.T) {
	q := queue.NewQueue(10)
	cfg := newTestConfig()
	s := newTestStore(t)
	ext := &mockExtractor{}
	emb := &mockEmbedder{}
	proc := NewProcessor(s, ext, nil, emb, nil, nil, q, cfg)

	pool := NewPool(2, q, proc)
	pool.Start()

	// Give workers a moment to start
	time.Sleep(50 * time.Millisecond)

	// Shutdown should not hang
	done := make(chan struct{})
	go func() {
		pool.Shutdown()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("pool shutdown timed out")
	}
}

func TestPoolProcessesJob(t *testing.T) {
	q := queue.NewQueue(10)
	cfg := newTestConfig()
	s := newTestStore(t)

	// Create a test PDF file
	docID := uuid.New()
	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, docID.String()+".pdf")
	// We won't actually read this file — the mock extractor handles it
	_ = pdfPath

	ext := &mockExtractor{
		result: &pdf.ExtractResult{
			Pages:     []pdf.Page{{Number: 1, Text: "Hello world. This is a test document with some content."}},
			PageCount: 1,
		},
	}
	emb := &mockEmbedder{
		embeddings: [][]float32{{1.0, 0.0, 0.0, 0.0}},
	}

	// Insert document pointing to a real file
	testFile := filepath.Join(tmpDir, "test.pdf")
	writeTestFile(t, testFile, []byte("%PDF-1.4 test"))

	doc := &model.Document{
		ID:       docID,
		Name:     "test.pdf",
		FilePath: testFile,
		FileSize: 100,
		Status:   model.StatusProcessing,
	}
	if err := s.InsertDocument(context.Background(), doc); err != nil {
		t.Fatalf("InsertDocument: %v", err)
	}

	proc := NewProcessor(s, ext, nil, emb, nil, nil, q, cfg)
	pool := NewPool(1, q, proc)
	pool.Start()

	// Enqueue job
	job := queue.NewProcessJob(docID, 3)
	if err := q.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Wait for processing
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		d, _ := s.GetDocument(context.Background(), docID)
		if d != nil && (d.Status == model.StatusCompleted || d.Status == model.StatusFailed) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	pool.Shutdown()

	// Verify
	d, _ := s.GetDocument(context.Background(), docID)
	if d == nil {
		t.Fatal("document not found after processing")
	}
	if d.Status != model.StatusCompleted {
		errMsg := ""
		if d.ErrorMessage != nil {
			errMsg = *d.ErrorMessage
		}
		t.Errorf("status = %q, want completed (error: %s)", d.Status, errMsg)
	}
}

func TestProcessorFailure_SetsFailedStatus(t *testing.T) {
	q := queue.NewQueue(10)
	cfg := newTestConfig()
	s := newTestStore(t)

	ext := &mockExtractor{err: fmt.Errorf("corrupt PDF")}
	emb := &mockEmbedder{}

	docID := uuid.New()
	testFile := filepath.Join(t.TempDir(), "test.pdf")
	writeTestFile(t, testFile, []byte("%PDF bad"))

	s.InsertDocument(context.Background(), &model.Document{
		ID: docID, Name: "bad.pdf", FilePath: testFile, FileSize: 100, Status: model.StatusProcessing,
	})

	proc := NewProcessor(s, ext, nil, emb, nil, nil, q, cfg)

	// Process directly (no pool needed for this test)
	job := queue.NewProcessJob(docID, 0) // no retries
	proc.Process(context.Background(), job)

	d, _ := s.GetDocument(context.Background(), docID)
	if d.Status != model.StatusFailed {
		t.Errorf("status = %q, want failed", d.Status)
	}
	if d.ErrorMessage == nil {
		t.Error("expected error message on failed document")
	}
}

func TestProcessorRetry(t *testing.T) {
	q := queue.NewQueue(10)
	cfg := newTestConfig()
	s := newTestStore(t)

	ext := &mockExtractor{err: fmt.Errorf("transient error")}
	emb := &mockEmbedder{}

	docID := uuid.New()
	testFile := filepath.Join(t.TempDir(), "test.pdf")
	writeTestFile(t, testFile, []byte("%PDF test"))

	s.InsertDocument(context.Background(), &model.Document{
		ID: docID, Name: "retry.pdf", FilePath: testFile, FileSize: 100, Status: model.StatusProcessing,
	})

	proc := NewProcessor(s, ext, nil, emb, nil, nil, q, cfg)

	// Process with retries available
	job := queue.NewProcessJob(docID, 3)
	job.Attempts = 0
	proc.Process(context.Background(), job)

	// Wait briefly for the retry goroutine's backoff
	time.Sleep(3 * time.Second)

	// A retry job should have been enqueued
	if q.Len() != 1 {
		t.Errorf("queue length = %d, want 1 (retry job)", q.Len())
	}
}

// --- Mock Scraper ---

type mockScraper struct {
	result     *pdf.ExtractResult
	extractErr error
}

func (m *mockScraper) Scrape(url string) (*scraper.ScrapeResult, error) {
	return nil, fmt.Errorf("not implemented in test")
}

func (m *mockScraper) ExtractFromHTML(htmlData []byte, sourceURL string) (*pdf.ExtractResult, error) {
	if m.extractErr != nil {
		return nil, m.extractErr
	}
	return m.result, nil
}

// --- Web Document Processor Tests ---

func TestProcessorWebDocument_Success(t *testing.T) {
	q := queue.NewQueue(10)
	cfg := newTestConfig()
	s := newTestStore(t)

	ext := &mockExtractor{} // not used for web documents
	scr := &mockScraper{
		result: &pdf.ExtractResult{
			Pages:     []pdf.Page{{Number: 1, Text: "Web article content. This is extracted from a web page for semantic search."}},
			PageCount: 1,
		},
	}
	emb := &mockEmbedder{
		embeddings: [][]float32{{1.0, 0.0, 0.0, 0.0}},
	}

	docID := uuid.New()
	tmpDir := t.TempDir()
	htmlFile := filepath.Join(tmpDir, docID.String()+".html")
	writeTestFile(t, htmlFile, []byte("<html><body><p>Web article content.</p></body></html>"))

	sourceURL := "https://example.com/article"
	doc := &model.Document{
		ID:         docID,
		Name:       "Test Article",
		FilePath:   htmlFile,
		FileSize:   100,
		Status:     model.StatusProcessing,
		SourceType: model.SourceTypeWeb,
		SourceURL:  &sourceURL,
	}
	if err := s.InsertDocument(context.Background(), doc); err != nil {
		t.Fatalf("InsertDocument: %v", err)
	}

	proc := NewProcessor(s, ext, scr, emb, nil, nil, q, cfg)
	pool := NewPool(1, q, proc)
	pool.Start()

	job := queue.NewProcessJob(docID, 3)
	if err := q.Enqueue(job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Wait for processing
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		d, _ := s.GetDocument(context.Background(), docID)
		if d != nil && (d.Status == model.StatusCompleted || d.Status == model.StatusFailed) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	pool.Shutdown()

	d, _ := s.GetDocument(context.Background(), docID)
	if d == nil {
		t.Fatal("document not found after processing")
	}
	if d.Status != model.StatusCompleted {
		errMsg := ""
		if d.ErrorMessage != nil {
			errMsg = *d.ErrorMessage
		}
		t.Errorf("status = %q, want completed (error: %s)", d.Status, errMsg)
	}
	if d.SourceType != model.SourceTypeWeb {
		t.Errorf("source_type = %q, want %q", d.SourceType, model.SourceTypeWeb)
	}
}

func TestProcessorWebDocument_ScraperFailure(t *testing.T) {
	q := queue.NewQueue(10)
	cfg := newTestConfig()
	s := newTestStore(t)

	ext := &mockExtractor{}
	scr := &mockScraper{extractErr: fmt.Errorf("failed to parse HTML")}
	emb := &mockEmbedder{}

	docID := uuid.New()
	htmlFile := filepath.Join(t.TempDir(), "test.html")
	writeTestFile(t, htmlFile, []byte("<html></html>"))

	sourceURL := "https://example.com/bad"
	s.InsertDocument(context.Background(), &model.Document{
		ID:         docID,
		Name:       "Bad Page",
		FilePath:   htmlFile,
		FileSize:   50,
		Status:     model.StatusProcessing,
		SourceType: model.SourceTypeWeb,
		SourceURL:  &sourceURL,
	})

	proc := NewProcessor(s, ext, scr, emb, nil, nil, q, cfg)
	job := queue.NewProcessJob(docID, 0) // no retries
	proc.Process(context.Background(), job)

	d, _ := s.GetDocument(context.Background(), docID)
	if d.Status != model.StatusFailed {
		t.Errorf("status = %q, want failed", d.Status)
	}
	if d.ErrorMessage == nil {
		t.Error("expected error message on failed web document")
	}
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}
