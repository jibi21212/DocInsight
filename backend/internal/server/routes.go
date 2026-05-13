package server

import (
	"net/http"

	"github.com/docinsight/backend/internal/config"
	"github.com/docinsight/backend/internal/embedder"
	"github.com/docinsight/backend/internal/events"
	"github.com/docinsight/backend/internal/handler"
	"github.com/docinsight/backend/internal/queue"
	"github.com/docinsight/backend/internal/scraper"
	"github.com/docinsight/backend/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(s store.Store, emb embedder.Embedder, scr scraper.Scraper, broker *events.Broker, q *queue.Queue, cfg *config.Config) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(handler.CORSMiddleware(cfg.CORSOrigin))
	r.Use(handler.LoggingMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Auth middleware (no-op when AUTH_ENABLED=false)
	r.Use(handler.AuthMiddleware(s, cfg.AuthEnabled))

	// Document handlers
	docHandler := handler.NewDocumentHandler(s, cfg)
	processHandler := handler.NewProcessHandler(s, q, cfg)
	searchHandler := handler.NewSearchHandler(s, emb, cfg)
	ingestHandler := handler.NewIngestHandler(s, scr, q, cfg)
	refreshHandler := handler.NewRefreshHandler(s, scr, q, cfg)
	tagHandler := handler.NewTagHandler(s)
	authHandler := handler.NewAuthHandler(s)

	r.Route("/api", func(r chi.Router) {
		// Auth endpoints
		r.Post("/auth/register", authHandler.Register)
		r.Get("/auth/me", authHandler.Me)
		// Documents
		r.Route("/documents", func(r chi.Router) {
			r.Get("/", docHandler.List)
			r.Post("/upload", docHandler.Upload)
			r.Post("/upload-bulk", docHandler.UploadMultiple)
			r.Post("/process", processHandler.Process)
			r.Post("/ingest", ingestHandler.Ingest)
			r.Get("/{id}", docHandler.GetByID)
			r.Delete("/{id}", docHandler.Delete)
			r.Post("/{id}/refresh", refreshHandler.Refresh)
			r.Post("/{id}/tags", tagHandler.AddToDocument)
			r.Delete("/{id}/tags/{tagId}", tagHandler.RemoveFromDocument)
		})

		// Tags
		r.Route("/tags", func(r chi.Router) {
			r.Get("/", tagHandler.List)
			r.Post("/", tagHandler.Create)
			r.Delete("/{id}", tagHandler.Delete)
		})

		// Search
		r.Post("/search", searchHandler.Search)

		// SSE events
		if broker != nil {
			sseHandler := handler.NewSSEHandler(broker)
			r.Get("/events", sseHandler.Stream)
		}
	})

	return r
}
