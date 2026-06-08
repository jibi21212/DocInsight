package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docinsight/backend/internal/config"
	"github.com/docinsight/backend/internal/embedder"
	"github.com/docinsight/backend/internal/events"
	"github.com/docinsight/backend/internal/ocr"
	"github.com/docinsight/backend/internal/pdf"
	"github.com/docinsight/backend/internal/queue"
	"github.com/docinsight/backend/internal/scraper"
	"github.com/docinsight/backend/internal/server"
	"github.com/docinsight/backend/internal/store"
	"github.com/docinsight/backend/internal/worker"
)

func main() {
	// Human-readable text logging to stdout (visible in terminal)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg := config.Load()

	// Database — auto-select based on DATABASE_URL
	ctx := context.Background()
	var db store.Store

	if cfg.DatabaseURL != "" {
		pgStore, err := store.NewPostgresStore(ctx, cfg.DatabaseURL)
		if err != nil {
			slog.Error("failed to connect to PostgreSQL", "error", err)
			os.Exit(1)
		}
		db = pgStore
		slog.Info("connected to PostgreSQL")
	} else {
		sqliteStore, err := store.NewSQLiteStore(cfg.SQLitePath)
		if err != nil {
			slog.Error("failed to open SQLite database", "error", err)
			os.Exit(1)
		}
		db = sqliteStore
		slog.Info("using SQLite (local dev mode)", "path", cfg.SQLitePath)
	}

	// Embedding sidecar client
	emb := embedder.NewHTTPEmbedder(cfg.EmbeddingSidecarURL)
	slog.Info("embedding sidecar configured", "url", cfg.EmbeddingSidecarURL)

	// PDF extractor
	ext := pdf.NewLedongthucExtractor()

	// Web scraper
	scr := scraper.NewReadabilityScraper(cfg.ScraperTimeoutSec, cfg.ScraperUserAgent)

	// OCR processor (optional — requires tesseract binary)
	var ocrProc *ocr.Processor
	if cfg.OCREnabled {
		ocrProc = ocr.NewProcessor(cfg.TesseractPath)
		if ocrProc.Available() {
			slog.Info("OCR enabled", "path", cfg.TesseractPath)
		} else {
			slog.Warn("OCR enabled but tesseract not found — OCR fallback disabled", "path", cfg.TesseractPath)
			ocrProc = nil
		}
	}

	// Event broker for SSE
	broker := events.NewBroker()

	// Job queue
	jobQueue := queue.NewQueue(cfg.QueueCapacity)

	// Worker pool
	processor := worker.NewProcessor(db, ext, scr, emb, ocrProc, broker, jobQueue, cfg)
	pool := worker.NewPool(cfg.WorkerCount, jobQueue, processor)
	pool.Start()

	// Recover any documents that were processing when we last shut down
	recoverJobs(ctx, db, jobQueue, cfg.MaxRetries)

	// HTTP server
	router := server.NewRouter(db, emb, scr, broker, jobQueue, cfg)
	srv := server.New(router, cfg.Port)

	// Graceful shutdown
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Wait for shutdown signal or server error
	select {
	case <-sigCtx.Done():
		slog.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			slog.Error("server error", "error", err)
		}
	}

	// Shutdown sequence
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}

	pool.Shutdown()
	db.Close()

	slog.Info("server stopped")
}

func recoverJobs(ctx context.Context, db store.Store, q *queue.Queue, maxRetries int) {
	ids, err := db.GetProcessingDocumentIDs(ctx)
	if err != nil {
		slog.Error("failed to recover processing jobs", "error", err)
		return
	}

	for _, id := range ids {
		job := queue.NewProcessJob(id, maxRetries)
		if err := q.Enqueue(job); err != nil {
			slog.Error("failed to re-enqueue recovered job", "document_id", id, "error", err)
		} else {
			slog.Info("recovered processing job", "document_id", id)
		}
	}

	if len(ids) > 0 {
		slog.Info("recovered processing jobs", "count", len(ids))
	}
}
