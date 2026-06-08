package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/docinsight/backend/internal/config"
	"github.com/docinsight/backend/internal/embedder"
	"github.com/docinsight/backend/internal/events"
	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/ocr"
	"github.com/docinsight/backend/internal/pdf"
	"github.com/docinsight/backend/internal/queue"
	"github.com/docinsight/backend/internal/scraper"
	"github.com/docinsight/backend/internal/store"
	"github.com/docinsight/backend/internal/worker"
	"github.com/google/uuid"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const localUserEmail = "local@docinsight.app"

// App is the Wails application backend. Bound methods (in app_*.go) are exposed
// to the frontend as JavaScript functions. All data access is scoped to a single
// implicit local user (a.userID).
type App struct {
	ctx     context.Context
	cfg     *config.Config
	store   store.Store
	emb     embedder.Embedder
	scr     scraper.Scraper
	queue   *queue.Queue
	broker  *events.Broker
	pool    *worker.Pool
	sidecar *sidecarProcess
	userID  *uuid.UUID // the single local user; passed to all store calls
	dataDir string
}

// NewApp creates a new App application struct.
func NewApp() *App { return &App{} }

// startup wires up all dependencies when the Wails runtime is ready.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	cfg := config.Load()

	// All mutable data lives under the OS user-config dir (e.g. %APPDATA%\DocInsight).
	dataDir, err := appDataDir()
	if err != nil {
		slog.Error("failed to resolve data dir", "error", err)
		return
	}
	a.dataDir = dataDir
	cfg.SQLitePath = filepath.Join(dataDir, "docinsight.db")
	cfg.UploadDir = filepath.Join(dataDir, "uploads")
	if err := os.MkdirAll(cfg.UploadDir, 0o755); err != nil {
		slog.Error("failed to create upload dir", "error", err)
	}
	a.cfg = cfg

	// Database (SQLite).
	db, err := store.NewSQLiteStore(cfg.SQLitePath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		return
	}
	a.store = db

	// Embedding sidecar (Python) — spawn + health-check. Non-fatal on failure:
	// the window still opens and the UI surfaces the error.
	if sc, err := startSidecar(ctx); err != nil {
		slog.Error("embedding sidecar failed to start (run setup.ps1); search & ingest disabled until it's up", "error", err)
		wruntime.EventsEmit(ctx, "sidecar.error", map[string]string{"error": err.Error()})
	} else {
		a.sidecar = sc
		cfg.EmbeddingSidecarURL = sc.url
		slog.Info("embedding sidecar ready", "url", sc.url)
	}
	a.emb = embedder.NewHTTPEmbedder(cfg.EmbeddingSidecarURL)

	// Services.
	ext := pdf.NewLedongthucExtractor()
	a.scr = scraper.NewReadabilityScraper(cfg.ScraperTimeoutSec, cfg.ScraperUserAgent)
	var ocrProc *ocr.Processor
	if cfg.OCREnabled {
		if p := ocr.NewProcessor(cfg.TesseractPath); p.Available() {
			ocrProc = p
		}
	}

	// Event broker + background worker pool.
	a.broker = events.NewBroker()
	a.queue = queue.NewQueue(cfg.QueueCapacity)
	processor := worker.NewProcessor(db, ext, a.scr, a.emb, ocrProc, a.broker, a.queue, cfg)
	a.pool = worker.NewPool(cfg.WorkerCount, a.queue, processor)
	a.pool.Start()

	a.recoverJobs()

	if err := a.ensureLocalUser(); err != nil {
		slog.Error("failed to provision local user", "error", err)
	}

	// Forward backend events to the frontend via the Wails runtime.
	go a.forwardEvents()

	slog.Info("DocInsight ready", "dataDir", dataDir)
}

// shutdown tears everything down cleanly on window close.
func (a *App) shutdown(ctx context.Context) {
	if a.pool != nil {
		a.pool.Shutdown()
	}
	if a.store != nil {
		a.store.Close()
	}
	if a.sidecar != nil {
		a.sidecar.stop()
	}
}

// forwardEvents bridges the internal SSE-style broker to Wails runtime events.
// Each event is emitted under its Type (e.g. "document.completed", "agent.delta")
// with its Data payload; the frontend subscribes via EventsOn(type).
func (a *App) forwardEvents() {
	ch := a.broker.Subscribe("wails")
	defer a.broker.Unsubscribe("wails")
	for {
		select {
		case <-a.ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			wruntime.EventsEmit(a.ctx, evt.Type, evt.Data)
		}
	}
}

// recoverJobs re-enqueues any documents that were mid-processing at last shutdown.
func (a *App) recoverJobs() {
	ids, err := a.store.GetProcessingDocumentIDs(a.ctx)
	if err != nil {
		slog.Error("failed to recover processing jobs", "error", err)
		return
	}
	for _, id := range ids {
		if err := a.queue.Enqueue(queue.NewProcessJob(id, a.cfg.MaxRetries)); err != nil {
			slog.Error("failed to re-enqueue recovered job", "document_id", id, "error", err)
		}
	}
	if len(ids) > 0 {
		slog.Info("recovered processing jobs", "count", len(ids))
	}
}

// ensureLocalUser provisions (once) the single implicit local user and caches its ID.
func (a *App) ensureLocalUser() error {
	u, err := a.store.GetUserByEmail(a.ctx, localUserEmail)
	if err != nil {
		return err
	}
	if u == nil {
		u = &model.User{
			ID:     uuid.New(),
			Email:  localUserEmail,
			APIKey: "di_local",
			Name:   "Local",
		}
		if err := a.store.CreateUser(a.ctx, u); err != nil {
			return err
		}
	}
	id := u.ID
	a.userID = &id
	return nil
}

// appDataDir returns (creating if needed) the per-user data directory.
func appDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "DocInsight")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// Ping is a trivial connectivity check exposed to the frontend.
func (a *App) Ping() string { return "ok" }
