package main

import (
	"fmt"
	"log/slog"

	"github.com/docinsight/backend/internal/model"
	"github.com/google/uuid"
)

// app_tags.go exposes tag management to the frontend. Ported from
// internal/handler/tags.go. Tags are global (not tenant-scoped): the underlying
// store calls take no userID, so none is passed here.

// ListTags returns all tags, never nil.
func (a *App) ListTags() ([]model.Tag, error) {
	tags, err := a.store.ListTags(a.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tags: %w", err)
	}
	if tags == nil {
		tags = []model.Tag{}
	}
	return tags, nil
}

// CreateTag creates a tag. Name is required; an empty color defaults to #6366f1.
func (a *App) CreateTag(name string, color string) (*model.Tag, error) {
	if name == "" {
		return nil, fmt.Errorf("tag name is required")
	}
	if color == "" {
		color = "#6366f1"
	}

	tag := &model.Tag{
		ID:    uuid.New(),
		Name:  name,
		Color: color,
	}
	if err := a.store.CreateTag(a.ctx, tag); err != nil {
		return nil, fmt.Errorf("tag already exists: %w", err)
	}

	return tag, nil
}

func (a *App) DeleteTag(id string) error {
	tagID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid tag ID: %w", err)
	}
	if err := a.store.DeleteTag(a.ctx, tagID); err != nil {
		return fmt.Errorf("tag not found: %w", err)
	}
	return nil
}

// AddTagToDocument attaches a tag to a document and returns the document's tags
// after the change (never nil).
func (a *App) AddTagToDocument(documentID string, tagID string) ([]model.Tag, error) {
	docID, err := uuid.Parse(documentID)
	if err != nil {
		return nil, fmt.Errorf("invalid document ID: %w", err)
	}
	tID, err := uuid.Parse(tagID)
	if err != nil {
		return nil, fmt.Errorf("invalid tag ID: %w", err)
	}

	if err := a.store.AddDocumentTag(a.ctx, docID, tID); err != nil {
		return nil, fmt.Errorf("failed to add tag: %w", err)
	}

	tags, err := a.store.GetDocumentTags(a.ctx, docID)
	if err != nil {
		slog.Error("failed to fetch document tags after add", "document_id", docID, "error", err)
	}
	if tags == nil {
		tags = []model.Tag{}
	}
	return tags, nil
}

// RemoveTagFromDocument detaches a tag from a document and returns the document's
// tags after the change (never nil).
func (a *App) RemoveTagFromDocument(documentID string, tagID string) ([]model.Tag, error) {
	docID, err := uuid.Parse(documentID)
	if err != nil {
		return nil, fmt.Errorf("invalid document ID: %w", err)
	}
	tID, err := uuid.Parse(tagID)
	if err != nil {
		return nil, fmt.Errorf("invalid tag ID: %w", err)
	}

	if err := a.store.RemoveDocumentTag(a.ctx, docID, tID); err != nil {
		return nil, fmt.Errorf("failed to remove tag: %w", err)
	}

	tags, err := a.store.GetDocumentTags(a.ctx, docID)
	if err != nil {
		slog.Error("failed to fetch document tags after remove", "document_id", docID, "error", err)
	}
	if tags == nil {
		tags = []model.Tag{}
	}
	return tags, nil
}
