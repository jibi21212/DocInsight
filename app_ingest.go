package main

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"

	"github.com/docinsight/backend/internal/crawler"
	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/queue"
	"github.com/google/uuid"
)

type IngestResult struct {
	Documents []model.Document `json:"documents"`
	Message   string           `json:"message"`
}

// IngestURLs fetches one or more web pages, persists each as a web-source
// document, and enqueues it for background processing. When crawl is true and a
// single start URL is supplied, the crawler first discovers same-domain pages
// (up to maxDepth hops / maxPages total) and ingests those instead.
//
// maxDepth and maxPages are advisory: a value <= 0 means "use the configured
// default" (a.cfg.MaxCrawlDepth / a.cfg.MaxCrawlPages).
//
// Ported from internal/handler/ingest.go (IngestHandler.Ingest).
func (a *App) IngestURLs(urls []string, crawl bool, maxDepth int, maxPages int) (*IngestResult, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("at least one URL is required")
	}

	if len(urls) > a.cfg.MaxIngestURLs {
		return nil, fmt.Errorf("maximum %d URLs allowed per request", a.cfg.MaxIngestURLs)
	}

	for _, rawURL := range urls {
		parsed, err := url.Parse(rawURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, fmt.Errorf("invalid URL: %s (must be http or https)", rawURL)
		}
	}

	if crawl && len(urls) == 1 {
		depth := a.cfg.MaxCrawlDepth
		if maxDepth > 0 {
			depth = maxDepth
		}
		pages := a.cfg.MaxCrawlPages
		if maxPages > 0 {
			pages = maxPages
		}

		c := crawler.NewCrawler(a.cfg.ScraperTimeoutSec)
		result, err := c.Crawl(a.ctx, crawler.CrawlOptions{
			StartURL:  urls[0],
			MaxDepth:  depth,
			MaxPages:  pages,
			UserAgent: a.cfg.ScraperUserAgent,
		})
		if err != nil {
			slog.Warn("crawl encountered an error, using partial results", "error", err)
		}
		if result != nil && len(result.URLs) > 0 {
			urls = result.URLs
			slog.Info("crawler discovered URLs", "count", len(urls))
		}
	}

	if err := os.MkdirAll(a.cfg.UploadDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create upload directory: %w", err)
	}

	var documents []model.Document

	for _, rawURL := range urls {
		result, err := a.scr.Scrape(rawURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch %s: %w", rawURL, err)
		}

		docID := uuid.New()

		htmlPath := filepath.Join(a.cfg.UploadDir, docID.String()+".html")
		if err := os.WriteFile(htmlPath, result.RawHTML, 0o644); err != nil {
			return nil, fmt.Errorf("failed to save web content: %w", err)
		}

		name := result.Title
		if name == "" {
			parsed, _ := url.Parse(rawURL)
			name = parsed.Host + parsed.Path
		}

		urlCopy := rawURL
		doc := &model.Document{
			ID:         docID,
			Name:       name,
			FilePath:   htmlPath,
			FileSize:   int64(len(result.RawHTML)),
			Status:     model.StatusPending,
			PageCount:  len(result.Pages),
			SourceType: model.SourceTypeWeb,
			SourceURL:  &urlCopy,
		}

		if err := a.store.InsertDocument(a.ctx, doc, a.userID); err != nil {
			os.Remove(htmlPath)
			return nil, fmt.Errorf("database error: %w", err)
		}

		if err := a.store.UpdateDocumentStatus(a.ctx, docID, model.StatusProcessing, nil); err != nil {
			return nil, fmt.Errorf("failed to update status: %w", err)
		}

		job := queue.NewProcessJob(docID, a.cfg.MaxRetries)
		if err := a.queue.Enqueue(job); err != nil {
			_ = a.store.UpdateDocumentStatus(a.ctx, docID, model.StatusPending, nil)
			return nil, fmt.Errorf("processing queue is full, try again later: %w", err)
		}

		// Re-fetch to get timestamps.
		refetched, err := a.store.GetDocument(a.ctx, docID, a.userID)
		if err == nil && refetched != nil {
			doc = refetched
		}
		documents = append(documents, *doc)
	}

	return &IngestResult{
		Documents: documents,
		Message:   fmt.Sprintf("%d URL(s) queued for processing", len(documents)),
	}, nil
}
