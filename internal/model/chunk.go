package model

import (
	"time"

	"github.com/google/uuid"
)

type ChunkMetadata struct {
	CharCount int `json:"char_count"`
	WordCount int `json:"word_count"`
	StartPage int `json:"start_page"`
	EndPage   int `json:"end_page"`
}

type Chunk struct {
	ID          uuid.UUID     `json:"id"`
	DocumentID  uuid.UUID     `json:"document_id"`
	Content     string        `json:"content"`
	PageNumber  int           `json:"page_number"`
	ChunkIndex  int           `json:"chunk_index"`
	Metadata    ChunkMetadata `json:"metadata"`
	CreatedAt   time.Time     `json:"created_at"`
}
