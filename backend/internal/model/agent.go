package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AgentSession represents a BYO-LLM chat session.
type AgentSession struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	FolderID  *uuid.UUID `json:"folder_id,omitempty"`
	Title     string     `json:"title"`
	Provider  string     `json:"provider"`
	Model     string     `json:"model"`
	CreatedAt time.Time  `json:"created_at"`
}

// Citation references a chunk that was used to ground an assistant message.
type Citation struct {
	ChunkID      uuid.UUID `json:"chunk_id"`
	DocumentID   uuid.UUID `json:"document_id"`
	DocumentName string    `json:"document_name"`
	Snippet      string    `json:"snippet"`
	PageNumber   int       `json:"page_number"`
	Score        float64   `json:"score"`
}

// AgentMessage is a single message in an agent session conversation.
type AgentMessage struct {
	ID        uuid.UUID  `json:"id"`
	SessionID uuid.UUID  `json:"session_id"`
	Role      string     `json:"role"` // user | assistant | tool
	Content   string     `json:"content"`
	Citations []Citation `json:"citations,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// MarshalCitations serializes Citations as JSON for storage. Returns "" when empty.
func (m *AgentMessage) MarshalCitations() (string, error) {
	if len(m.Citations) == 0 {
		return "", nil
	}
	b, err := json.Marshal(m.Citations)
	return string(b), err
}

// UnmarshalCitations populates Citations from a JSON string. Empty string -> nil.
func (m *AgentMessage) UnmarshalCitations(s string) error {
	if s == "" {
		m.Citations = nil
		return nil
	}
	return json.Unmarshal([]byte(s), &m.Citations)
}
