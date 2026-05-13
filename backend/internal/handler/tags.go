package handler

import (
	"encoding/json"
	"net/http"

	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type TagHandler struct {
	store store.Store
}

func NewTagHandler(s store.Store) *TagHandler {
	return &TagHandler{store: s}
}

func (h *TagHandler) List(w http.ResponseWriter, r *http.Request) {
	tags, err := h.store.ListTags(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch tags")
		return
	}
	if tags == nil {
		tags = []model.Tag{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"tags": tags})
}

func (h *TagHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Tag name is required")
		return
	}
	if req.Color == "" {
		req.Color = "#6366f1"
	}

	tag := &model.Tag{
		ID:    uuid.New(),
		Name:  req.Name,
		Color: req.Color,
	}
	if err := h.store.CreateTag(r.Context(), tag); err != nil {
		writeError(w, http.StatusConflict, "Tag already exists")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"tag": tag})
}

func (h *TagHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid tag ID")
		return
	}
	if err := h.store.DeleteTag(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "Tag not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Tag deleted"})
}

func (h *TagHandler) AddToDocument(w http.ResponseWriter, r *http.Request) {
	docID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid document ID")
		return
	}

	var req struct {
		TagID string `json:"tagId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	tagID, err := uuid.Parse(req.TagID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid tag ID")
		return
	}

	if err := h.store.AddDocumentTag(r.Context(), docID, tagID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to add tag")
		return
	}

	tags, _ := h.store.GetDocumentTags(r.Context(), docID)
	if tags == nil {
		tags = []model.Tag{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"tags": tags})
}

func (h *TagHandler) RemoveFromDocument(w http.ResponseWriter, r *http.Request) {
	docID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid document ID")
		return
	}
	tagID, err := uuid.Parse(chi.URLParam(r, "tagId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid tag ID")
		return
	}

	if err := h.store.RemoveDocumentTag(r.Context(), docID, tagID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to remove tag")
		return
	}

	tags, _ := h.store.GetDocumentTags(r.Context(), docID)
	if tags == nil {
		tags = []model.Tag{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"tags": tags})
}
