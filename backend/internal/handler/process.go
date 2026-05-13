package handler

import (
	"encoding/json"
	"net/http"

	"github.com/docinsight/backend/internal/config"
	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/queue"
	"github.com/docinsight/backend/internal/store"
	"github.com/google/uuid"
)

type ProcessHandler struct {
	store store.Store
	queue *queue.Queue
	cfg   *config.Config
}

func NewProcessHandler(s store.Store, q *queue.Queue, cfg *config.Config) *ProcessHandler {
	return &ProcessHandler{store: s, queue: q, cfg: cfg}
}

type processRequest struct {
	DocumentID string `json:"documentId"`
}

func (h *ProcessHandler) Process(w http.ResponseWriter, r *http.Request) {
	var req processRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	docID, err := uuid.Parse(req.DocumentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid documentId")
		return
	}

	doc, err := h.store.GetDocument(r.Context(), docID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "Document not found")
		return
	}

	if doc.Status == model.StatusProcessing {
		writeError(w, http.StatusConflict, "Document is already being processed")
		return
	}

	// Mark as processing
	if err := h.store.UpdateDocumentStatus(r.Context(), docID, model.StatusProcessing, nil); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update status")
		return
	}

	// Enqueue job
	job := queue.NewProcessJob(docID, h.cfg.MaxRetries)
	if err := h.queue.Enqueue(job); err != nil {
		// Queue full — revert status
		_ = h.store.UpdateDocumentStatus(r.Context(), docID, model.StatusPending, nil)
		writeError(w, http.StatusServiceUnavailable, "Processing queue is full, try again later")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":    "Processing started",
		"documentId": docID,
	})
}
