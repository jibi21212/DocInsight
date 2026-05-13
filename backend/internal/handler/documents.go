package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/docinsight/backend/internal/config"
	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/queue"
	"github.com/docinsight/backend/internal/scraper"
	"github.com/docinsight/backend/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type DocumentHandler struct {
	store store.Store
	cfg   *config.Config
}

func NewDocumentHandler(s store.Store, cfg *config.Config) *DocumentHandler {
	return &DocumentHandler{store: s, cfg: cfg}
}

func (h *DocumentHandler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize < 1 {
		pageSize = 20
	}

	var status *string
	if s := r.URL.Query().Get("status"); s != "" {
		status = &s
	}

	docs, total, err := h.store.ListDocuments(r.Context(), page, pageSize, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch documents")
		return
	}

	if docs == nil {
		docs = []model.Document{}
	}

	resp := model.PaginatedResponse[model.Document]{
		Data:       docs,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: int(math.Ceil(float64(total) / float64(pageSize))),
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *DocumentHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid document ID")
		return
	}

	doc, err := h.store.GetDocument(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch document")
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "Document not found")
		return
	}

	chunks, err := h.store.GetChunksByDocumentID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch chunks")
		return
	}

	if chunks == nil {
		chunks = []model.Chunk{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"document":   doc,
		"chunks":     chunks,
		"chunkCount": len(chunks),
	})
}

func (h *DocumentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid document ID")
		return
	}

	filePath, err := h.store.DeleteDocument(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "Document not found")
		return
	}

	// Remove file from disk
	_ = os.Remove(filePath)

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "Document deleted successfully",
	})
}

func (h *DocumentHandler) Upload(w http.ResponseWriter, r *http.Request) {
	maxBytes := h.cfg.MaxUploadSizeMB * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	if err := r.ParseMultipartForm(maxBytes); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("File too large (max %dMB)", h.cfg.MaxUploadSizeMB))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "No file provided")
		return
	}
	defer file.Close()

	if filepath.Ext(header.Filename) != ".pdf" {
		writeError(w, http.StatusBadRequest, "Only PDF files are supported")
		return
	}

	documentID := uuid.New()

	// Ensure upload directory exists
	if err := os.MkdirAll(h.cfg.UploadDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create upload directory")
		return
	}

	filePath := filepath.Join(h.cfg.UploadDir, documentID.String()+".pdf")
	dst, err := os.Create(filePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save file")
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		os.Remove(filePath)
		writeError(w, http.StatusInternalServerError, "Failed to save file")
		return
	}

	doc := &model.Document{
		ID:       documentID,
		Name:     header.Filename,
		FilePath: filePath,
		FileSize: written,
		Status:   model.StatusPending,
	}

	if err := h.store.InsertDocument(r.Context(), doc); err != nil {
		os.Remove(filePath)
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	// Re-fetch to get server-generated timestamps
	refetched, err := h.store.GetDocument(r.Context(), documentID)
	if err == nil && refetched != nil {
		doc = refetched
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"document": doc,
		"message":  "Document uploaded successfully. Processing will begin shortly.",
	})
}

func (h *DocumentHandler) UploadMultiple(w http.ResponseWriter, r *http.Request) {
	maxBytes := h.cfg.MaxUploadSizeMB * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes*10) // allow 10x for multiple files

	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid multipart request")
		return
	}

	if err := os.MkdirAll(h.cfg.UploadDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create upload directory")
		return
	}

	var documents []*model.Document

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "Error reading multipart data")
			return
		}

		if part.FormName() != "files" {
			part.Close()
			continue
		}

		filename := part.FileName()
		if filepath.Ext(filename) != ".pdf" {
			part.Close()
			writeError(w, http.StatusBadRequest, fmt.Sprintf("Only PDF files are supported: %s", filename))
			return
		}

		doc, err := h.savePart(r, part, filename, maxBytes)
		part.Close()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		documents = append(documents, doc)
	}

	if len(documents) == 0 {
		writeError(w, http.StatusBadRequest, "No PDF files provided")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"documents": documents,
		"message":   fmt.Sprintf("%d document(s) uploaded successfully", len(documents)),
	})
}

func (h *DocumentHandler) savePart(r *http.Request, part *multipart.Part, filename string, maxBytes int64) (*model.Document, error) {
	documentID := uuid.New()
	filePath := filepath.Join(h.cfg.UploadDir, documentID.String()+".pdf")

	dst, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to save file")
	}

	written, err := io.Copy(dst, io.LimitReader(part, maxBytes))
	dst.Close()
	if err != nil {
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to save file")
	}

	doc := &model.Document{
		ID:       documentID,
		Name:     filename,
		FilePath: filePath,
		FileSize: written,
		Status:   model.StatusPending,
	}

	if err := h.store.InsertDocument(r.Context(), doc); err != nil {
		os.Remove(filePath)
		return nil, fmt.Errorf("database error")
	}

	refetched, err := h.store.GetDocument(r.Context(), documentID)
	if err == nil && refetched != nil {
		doc = refetched
	}
	return doc, nil
}

// RefreshHandler handles re-crawling web documents.
type RefreshHandler struct {
	store   store.Store
	scraper scraper.Scraper
	queue   *queue.Queue
	cfg     *config.Config
}

func NewRefreshHandler(s store.Store, scr scraper.Scraper, q *queue.Queue, cfg *config.Config) *RefreshHandler {
	return &RefreshHandler{store: s, scraper: scr, queue: q, cfg: cfg}
}

func (h *RefreshHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid document ID")
		return
	}

	doc, err := h.store.GetDocument(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch document")
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "Document not found")
		return
	}

	if doc.SourceType != model.SourceTypeWeb || doc.SourceURL == nil {
		writeError(w, http.StatusBadRequest, "Only web documents can be refreshed")
		return
	}

	if doc.Status == model.StatusProcessing {
		writeError(w, http.StatusConflict, "Document is currently being processed")
		return
	}

	// Re-scrape the URL
	result, err := h.scraper.Scrape(*doc.SourceURL)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("Failed to re-fetch URL: %v", err))
		return
	}

	// Overwrite HTML file on disk
	if err := os.WriteFile(doc.FilePath, result.RawHTML, 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save updated content")
		return
	}

	// Delete old chunks (cascades to embeddings)
	if err := h.store.DeleteChunksByDocumentID(r.Context(), id); err != nil {
		slog.Error("failed to delete old chunks during refresh", "error", err)
	}

	// Update document name if title changed
	if result.Title != "" && result.Title != doc.Name {
		doc.Name = result.Title
	}

	// Reset status and enqueue for re-processing
	if err := h.store.UpdateDocumentStatus(r.Context(), id, model.StatusProcessing, nil); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update status")
		return
	}

	job := queue.NewProcessJob(id, h.cfg.MaxRetries)
	if err := h.queue.Enqueue(job); err != nil {
		_ = h.store.UpdateDocumentStatus(r.Context(), id, model.StatusPending, nil)
		writeError(w, http.StatusServiceUnavailable, "Processing queue is full, try again later")
		return
	}

	// Re-fetch to get updated timestamps
	refetched, err := h.store.GetDocument(r.Context(), id)
	if err == nil && refetched != nil {
		doc = refetched
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"document": doc,
		"message":  "Document queued for re-processing",
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
