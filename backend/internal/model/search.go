package model

import "github.com/google/uuid"

type SearchRequest struct {
	Query       string      `json:"query"`
	TopK        *int        `json:"topK,omitempty"`
	Threshold   *float64    `json:"threshold,omitempty"`
	DocumentIDs []uuid.UUID `json:"documentIds,omitempty"`
	SearchMode  string      `json:"searchMode,omitempty"` // "hybrid" (default), "semantic", "keyword"
	FolderID    *uuid.UUID  `json:"folder_id,omitempty"`
}

type SearchResult struct {
	ChunkID         uuid.UUID     `json:"chunk_id"`
	Content         string        `json:"content"`
	Similarity      float64       `json:"similarity"`
	PageNumber      int           `json:"page_number"`
	ChunkIndex      int           `json:"chunk_index"`
	Metadata        ChunkMetadata `json:"metadata"`
	DocumentID      uuid.UUID     `json:"document_id"`
	DocumentName    string        `json:"document_name"`
	SourceType      string        `json:"source_type"`
	SourceURL       *string       `json:"source_url,omitempty"`
	MatchType       string        `json:"match_type"`              // "semantic", "keyword", "hybrid"
	KeywordScore    float64       `json:"keyword_score,omitempty"` // BM25 score
	Snippet         string        `json:"snippet"`                 // best-match window for display
	HighlightTokens []string      `json:"highlight_tokens"`        // normalized query tokens for UI <mark>
}

type SearchResponse struct {
	Results []SearchResult `json:"results"`
	Query   string         `json:"query"`
	Total   int            `json:"total"`
	TookMs  int64          `json:"took_ms"`
}
