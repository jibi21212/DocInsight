package handler

import (
	"encoding/json"
	"net/http"

	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type FolderHandler struct {
	store store.Store
}

func NewFolderHandler(s store.Store) *FolderHandler {
	return &FolderHandler{store: s}
}

// List returns folders. With ?parent_id=... it returns the direct children of
// that folder; otherwise it returns top-level (parent IS NULL) folders.
func (h *FolderHandler) List(w http.ResponseWriter, r *http.Request) {
	var parentID *uuid.UUID
	if pid := r.URL.Query().Get("parent_id"); pid != "" {
		parsed, err := uuid.Parse(pid)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid parent_id")
			return
		}
		parentID = &parsed
	}

	uid := userIDFromContext(r.Context())
	folders, err := h.store.ListFolders(r.Context(), uid, parentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch folders")
		return
	}
	if folders == nil {
		folders = []model.Folder{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"folders": folders})
}

// Create creates a new folder.
func (h *FolderHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string  `json:"name"`
		ParentID *string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Folder name is required")
		return
	}

	uid := userIDFromContext(r.Context())

	var parentID *uuid.UUID
	if req.ParentID != nil && *req.ParentID != "" {
		parsed, err := uuid.Parse(*req.ParentID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid parent_id")
			return
		}
		// Verify parent belongs to user
		parent, err := h.store.GetFolder(r.Context(), parsed, uid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to verify parent folder")
			return
		}
		if parent == nil {
			writeError(w, http.StatusBadRequest, "Parent folder not found")
			return
		}
		parentID = &parsed
	}

	folder := &model.Folder{
		ID:       uuid.New(),
		UserID:   uid,
		ParentID: parentID,
		Name:     req.Name,
	}
	if err := h.store.CreateFolder(r.Context(), folder); err != nil {
		writeError(w, http.StatusConflict, "Folder with this name already exists in the parent")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"folder": folder})
}

// Delete removes a folder. Descendant folders cascade-delete; contained
// documents have their folder_id reset to NULL.
func (h *FolderHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid folder ID")
		return
	}
	if err := h.store.DeleteFolder(r.Context(), id, userIDFromContext(r.Context())); err != nil {
		writeError(w, http.StatusNotFound, "Folder not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Folder deleted"})
}
