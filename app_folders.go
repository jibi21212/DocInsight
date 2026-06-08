package main

import (
	"fmt"

	"github.com/docinsight/backend/internal/model"
	"github.com/google/uuid"
)

// ListFolders returns folders for the current user. A non-empty parentID returns
// the direct children of that folder; an empty parentID returns top-level
// (parent IS NULL) folders. Ported from handler.FolderHandler.List.
func (a *App) ListFolders(parentID string) ([]model.Folder, error) {
	var parent *uuid.UUID
	if parentID != "" {
		parsed, err := uuid.Parse(parentID)
		if err != nil {
			return nil, fmt.Errorf("invalid parent_id: %w", err)
		}
		parent = &parsed
	}

	folders, err := a.store.ListFolders(a.ctx, a.userID, parent)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch folders: %w", err)
	}
	if folders == nil {
		folders = []model.Folder{}
	}
	return folders, nil
}

// CreateFolder creates a new folder for the current user. When parentID is
// non-empty it must reference a folder owned by the current user. Ported from
// handler.FolderHandler.Create.
func (a *App) CreateFolder(name string, parentID string) (*model.Folder, error) {
	if name == "" {
		return nil, fmt.Errorf("folder name is required")
	}

	var parent *uuid.UUID
	if parentID != "" {
		parsed, err := uuid.Parse(parentID)
		if err != nil {
			return nil, fmt.Errorf("invalid parent_id: %w", err)
		}
		// Verify parent belongs to user.
		p, err := a.store.GetFolder(a.ctx, parsed, a.userID)
		if err != nil {
			return nil, fmt.Errorf("failed to verify parent folder: %w", err)
		}
		if p == nil {
			return nil, fmt.Errorf("parent folder not found")
		}
		parent = &parsed
	}

	folder := &model.Folder{
		ID:       uuid.New(),
		UserID:   a.userID,
		ParentID: parent,
		Name:     name,
	}
	if err := a.store.CreateFolder(a.ctx, folder); err != nil {
		return nil, fmt.Errorf("folder with this name already exists in the parent: %w", err)
	}

	return folder, nil
}

// DeleteFolder removes a folder owned by the current user. Descendant folders
// cascade-delete; contained documents have their folder_id reset to NULL. A
// missing or wrong-user folder is reported as not found. Ported from
// handler.FolderHandler.Delete.
func (a *App) DeleteFolder(id string) error {
	folderID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid folder ID: %w", err)
	}
	if err := a.store.DeleteFolder(a.ctx, folderID, a.userID); err != nil {
		return fmt.Errorf("folder not found: %w", err)
	}
	return nil
}
