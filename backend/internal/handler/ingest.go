package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/docinsight/backend/internal/config"
	"github.com/docinsight/backend/internal/crawler"
	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/queue"
	"github.com/docinsight/backend/internal/scraper"
	"github.com/docinsight/backend/internal/store"
	"github.com/google/uuid"
)

type IngestHandler struct {
	store   store.Store
	scraper scraper.Scraper
	queue   *queue.Queue
	cfg     *config.Config
}

func NewIngestHandler(s store.Store, scr scraper.Scraper, q *queue.Queue, cfg *config.Config) *IngestHandler {
	return &IngestHandler{store: s, scraper: scr, queue: q, cfg: cfg}
}

type ingestRequest struct {
	URLs     []string `json:"urls"`
	Crawl    bool     `json:"crawl,omitempty"`
	MaxDepth *int     `json:"maxDepth,omitempty"`
	MaxPages *int     `json:"maxPages,omitempty"`
}

func (h *IngestHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	var req ingestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if len(req.URLs) == 0 {
		writeError(w, http.StatusBadRequest, "At least one URL is required")
		return
	}

	if len(req.URLs) > h.cfg.MaxIngestURLs {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Maximum %d URLs allowed per request", h.cfg.MaxIngestURLs))
		return
	}

	// Validate all URLs first
	for _, rawURL := range req.URLs {
		parsed, err := url.Parse(rawURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid URL: %s (must be http or https)", rawURL))
			return
		}
	}

	// If crawl mode is enabled with a single URL, discover more URLs first
	if req.Crawl && len(req.URLs) == 1 {
		depth := h.cfg.MaxCrawlDepth
		if req.MaxDepth != nil && *req.MaxDepth > 0 {
			depth = *req.MaxDepth
		}
		maxPages := h.cfg.MaxCrawlPages
		if req.MaxPages != nil && *req.MaxPages > 0 {
			maxPages = *req.MaxPages
		}

		c := crawler.NewCrawler(h.cfg.ScraperTimeoutSec)
		result, err := c.Crawl(r.Context(), crawler.CrawlOptions{
			StartURL:  req.URLs[0],
			MaxDepth:  depth,
			MaxPages:  maxPages,
			UserAgent: h.cfg.ScraperUserAgent,
		})
		if err != nil {
			slog.Warn("crawl encountered an error, using partial results", "error", err)
		}
		if result != nil && len(result.URLs) > 0 {
			req.URLs = result.URLs
			slog.Info("crawler discovered URLs", "count", len(req.URLs))
		}
	}

	// Ensure upload directory exists
	if err := os.MkdirAll(h.cfg.UploadDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create upload directory")
		return
	}

	var documents []*model.Document

	for _, rawURL := range req.URLs {
		// Scrape the URL
		result, err := h.scraper.Scrape(rawURL)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("Failed to fetch %s: %v", rawURL, err))
			return
		}

		docID := uuid.New()

		// Save raw HTML to disk
		htmlPath := filepath.Join(h.cfg.UploadDir, docID.String()+".html")
		if err := os.WriteFile(htmlPath, result.RawHTML, 0o644); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to save web content")
			return
		}

		// Determine name
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

		if err := h.store.InsertDocument(r.Context(), doc); err != nil {
			os.Remove(htmlPath)
			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}

		// Mark as processing and enqueue
		if err := h.store.UpdateDocumentStatus(r.Context(), docID, model.StatusProcessing, nil); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to update status")
			return
		}

		job := queue.NewProcessJob(docID, h.cfg.MaxRetries)
		if err := h.queue.Enqueue(job); err != nil {
			_ = h.store.UpdateDocumentStatus(r.Context(), docID, model.StatusPending, nil)
			writeError(w, http.StatusServiceUnavailable, "Processing queue is full, try again later")
			return
		}

		// Re-fetch to get timestamps
		refetched, err := h.store.GetDocument(r.Context(), docID)
		if err == nil && refetched != nil {
			doc = refetched
		}
		documents = append(documents, doc)
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"documents": documents,
		"message":   fmt.Sprintf("%d URL(s) queued for processing", len(documents)),
	})
}
