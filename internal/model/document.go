package model

import (
	"time"

	"github.com/google/uuid"
)

type DocumentStatus string

const (
	StatusPending    DocumentStatus = "pending"
	StatusProcessing DocumentStatus = "processing"
	StatusCompleted  DocumentStatus = "completed"
	StatusFailed     DocumentStatus = "failed"
)

const (
	SourceTypePDF = "pdf"
	SourceTypeWeb = "web"
)

type Document struct {
	ID           uuid.UUID      `json:"id"`
	Name         string         `json:"name"`
	UploadDate   time.Time      `json:"upload_date"`
	PageCount    int            `json:"page_count"`
	Status       DocumentStatus `json:"status"`
	FilePath     string         `json:"file_path"`
	FileSize     int64          `json:"file_size"`
	ErrorMessage *string        `json:"error_message"`
	SourceType   string         `json:"source_type"`
	SourceURL    *string        `json:"source_url"`
	UserID       *uuid.UUID     `json:"user_id,omitempty"`
	FolderID     *uuid.UUID     `json:"folder_id,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}
