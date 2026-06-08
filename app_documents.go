package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/queue"
	"github.com/google/uuid"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type DocumentsPage struct {
	Data  []model.Document `json:"data"`
	Total int              `json:"total"`
}

type DocumentDetail struct {
	Document   *model.Document `json:"document"`
	Chunks     []model.Chunk   `json:"chunks"`
	ChunkCount int             `json:"chunkCount"`
}

type AddDocumentsResult struct {
	Documents []model.Document `json:"documents"`
}

// ListDocuments returns a page of the local user's documents, optionally scoped
// to a folder. An empty folderID lists across all folders. Ported from
// DocumentHandler.List.
func (a *App) ListDocuments(page int, pageSize int, folderID string) (*DocumentsPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	var folderUUID *uuid.UUID
	if folderID != "" {
		parsed, err := uuid.Parse(folderID)
		if err != nil {
			return nil, fmt.Errorf("invalid folder ID: %w", err)
		}
		folderUUID = &parsed
	}

	docs, total, err := a.store.ListDocuments(a.ctx, page, pageSize, nil, a.userID, folderUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch documents: %w", err)
	}
	if docs == nil {
		docs = []model.Document{}
	}

	return &DocumentsPage{Data: docs, Total: total}, nil
}

// GetDocument returns a single document with its chunks. Ported from
// DocumentHandler.GetByID. A wrong-user or missing document is reported as
// not-found.
func (a *App) GetDocument(id string) (*DocumentDetail, error) {
	docID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid document ID: %w", err)
	}

	doc, err := a.store.GetDocument(a.ctx, docID, a.userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch document: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("document not found")
	}

	chunks, err := a.store.GetChunksByDocumentID(a.ctx, docID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch chunks: %w", err)
	}
	if chunks == nil {
		chunks = []model.Chunk{}
	}

	return &DocumentDetail{
		Document:   doc,
		Chunks:     chunks,
		ChunkCount: len(chunks),
	}, nil
}

// DeleteDocument removes a document (and its file on disk). Ported from
// DocumentHandler.Delete. A wrong-user or missing document is reported as
// not-found.
func (a *App) DeleteDocument(id string) error {
	docID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid document ID: %w", err)
	}

	filePath, err := a.store.DeleteDocument(a.ctx, docID, a.userID)
	if err != nil {
		return fmt.Errorf("document not found")
	}

	// Remove the backing file from disk (best effort, matching the handler).
	_ = os.Remove(filePath)

	return nil
}

// MoveDocument moves a document into a folder, or unfiles it to the root when
// folderID is empty. Ported from DocumentHandler.Move. A wrong-user or missing
// document is reported as not-found.
func (a *App) MoveDocument(id string, folderID string) error {
	docID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid document ID: %w", err)
	}

	var folderUUID *uuid.UUID
	if folderID != "" {
		parsed, perr := uuid.Parse(folderID)
		if perr != nil {
			return fmt.Errorf("invalid folder ID: %w", perr)
		}
		folderUUID = &parsed
	}

	if err := a.store.MoveDocumentToFolder(a.ctx, docID, folderUUID, a.userID); err != nil {
		return fmt.Errorf("document not found")
	}

	return nil
}

// ProcessDocument enqueues a (re)processing job for a document. Ported from
// ProcessHandler.Process. A wrong-user or missing document is reported as
// not-found.
func (a *App) ProcessDocument(id string) error {
	docID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid document ID: %w", err)
	}

	doc, err := a.store.GetDocument(a.ctx, docID, a.userID)
	if err != nil {
		return fmt.Errorf("database error: %w", err)
	}
	if doc == nil {
		return fmt.Errorf("document not found")
	}

	if doc.Status == model.StatusProcessing {
		return fmt.Errorf("document is already being processed")
	}

	if err := a.store.UpdateDocumentStatus(a.ctx, docID, model.StatusProcessing, nil); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	job := queue.NewProcessJob(docID, a.cfg.MaxRetries)
	if err := a.queue.Enqueue(job); err != nil {
		_ = a.store.UpdateDocumentStatus(a.ctx, docID, model.StatusPending, nil)
		return fmt.Errorf("processing queue is full, try again later")
	}

	return nil
}

// RefreshDocument re-crawls a web document, overwrites its stored content,
// clears stale chunks, and re-enqueues it for processing. Ported from
// RefreshHandler.Refresh. A wrong-user or missing document is reported as
// not-found.
func (a *App) RefreshDocument(id string) (*model.Document, error) {
	docID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid document ID: %w", err)
	}

	doc, err := a.store.GetDocument(a.ctx, docID, a.userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch document: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("document not found")
	}

	if doc.SourceType != model.SourceTypeWeb || doc.SourceURL == nil {
		return nil, fmt.Errorf("only web documents can be refreshed")
	}

	if doc.Status == model.StatusProcessing {
		return nil, fmt.Errorf("document is currently being processed")
	}

	result, err := a.scr.Scrape(*doc.SourceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to re-fetch URL: %w", err)
	}

	if err := os.WriteFile(doc.FilePath, result.RawHTML, 0o644); err != nil {
		return nil, fmt.Errorf("failed to save updated content: %w", err)
	}

	// Delete old chunks (cascades to embeddings).
	if err := a.store.DeleteChunksByDocumentID(a.ctx, docID); err != nil {
		slog.Error("failed to delete old chunks during refresh", "error", err)
	}

	if result.Title != "" && result.Title != doc.Name {
		doc.Name = result.Title
	}

	if err := a.store.UpdateDocumentStatus(a.ctx, docID, model.StatusProcessing, nil); err != nil {
		return nil, fmt.Errorf("failed to update status: %w", err)
	}

	job := queue.NewProcessJob(docID, a.cfg.MaxRetries)
	if err := a.queue.Enqueue(job); err != nil {
		_ = a.store.UpdateDocumentStatus(a.ctx, docID, model.StatusPending, nil)
		return nil, fmt.Errorf("processing queue is full, try again later")
	}

	// Re-fetch to pick up updated timestamps.
	if refetched, err := a.store.GetDocument(a.ctx, docID, a.userID); err == nil && refetched != nil {
		doc = refetched
	}

	return doc, nil
}

// AddDocuments opens a native file picker for one or more PDFs, saves each into
// the upload dir, inserts a document row, and enqueues it for processing. This
// replaces the old multipart upload (DocumentHandler.Upload) for the desktop
// app. A cancelled dialog (no selection) returns an empty result and no error.
func (a *App) AddDocuments() (*AddDocumentsResult, error) {
	paths, err := wruntime.OpenMultipleFilesDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "Add PDFs",
		Filters: []wruntime.FileFilter{
			{DisplayName: "PDF Documents (*.pdf)", Pattern: "*.pdf"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open file dialog: %w", err)
	}

	result := &AddDocumentsResult{Documents: []model.Document{}}
	if len(paths) == 0 {
		return result, nil
	}

	if err := os.MkdirAll(a.cfg.UploadDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create upload directory: %w", err)
	}

	maxBytes := a.cfg.MaxUploadSizeMB * 1024 * 1024

	for _, srcPath := range paths {
		if filepath.Ext(srcPath) != ".pdf" {
			return nil, fmt.Errorf("only PDF files are supported: %s", filepath.Base(srcPath))
		}

		data, err := os.ReadFile(srcPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", filepath.Base(srcPath), err)
		}

		if int64(len(data)) > maxBytes {
			return nil, fmt.Errorf("file too large (max %dMB): %s", a.cfg.MaxUploadSizeMB, filepath.Base(srcPath))
		}

		documentID := uuid.New()
		destPath := filepath.Join(a.cfg.UploadDir, documentID.String()+".pdf")
		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			return nil, fmt.Errorf("failed to save file: %w", err)
		}

		doc := &model.Document{
			ID:       documentID,
			Name:     filepath.Base(srcPath),
			FilePath: destPath,
			FileSize: int64(len(data)),
			Status:   model.StatusPending,
		}

		if err := a.store.InsertDocument(a.ctx, doc, a.userID); err != nil {
			_ = os.Remove(destPath)
			return nil, fmt.Errorf("database error: %w", err)
		}

		job := queue.NewProcessJob(documentID, a.cfg.MaxRetries)
		if err := a.queue.Enqueue(job); err != nil {
			return nil, fmt.Errorf("processing queue is full, try again later")
		}

		// Re-fetch to get server-generated timestamps.
		if refetched, err := a.store.GetDocument(a.ctx, documentID, a.userID); err == nil && refetched != nil {
			doc = refetched
		}

		result.Documents = append(result.Documents, *doc)
	}

	return result, nil
}
