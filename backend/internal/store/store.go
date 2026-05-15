package store

import (
	"context"

	"github.com/docinsight/backend/internal/model"
	"github.com/google/uuid"
)

type Store interface {
	// Documents
	InsertDocument(ctx context.Context, doc *model.Document, userID *uuid.UUID) error
	GetDocument(ctx context.Context, id uuid.UUID, userID *uuid.UUID) (*model.Document, error)
	ListDocuments(ctx context.Context, page, pageSize int, status *string, userID *uuid.UUID, folderID *uuid.UUID) ([]model.Document, int, error)
	UpdateDocumentStatus(ctx context.Context, id uuid.UUID, status model.DocumentStatus, errMsg *string) error
	UpdateDocumentPageCount(ctx context.Context, id uuid.UUID, pageCount int) error
	DeleteDocument(ctx context.Context, id uuid.UUID, userID *uuid.UUID) (string, error) // returns file_path

	// Chunks
	InsertChunks(ctx context.Context, chunks []model.Chunk) ([]uuid.UUID, error)
	GetChunksByDocumentID(ctx context.Context, documentID uuid.UUID) ([]model.Chunk, error)
	GetChunkByID(ctx context.Context, chunkID uuid.UUID, userID *uuid.UUID) (*model.Chunk, error)
	DeleteChunksByDocumentID(ctx context.Context, documentID uuid.UUID) error

	// Embeddings
	InsertEmbeddings(ctx context.Context, chunkIDs []uuid.UUID, embeddings [][]float32) error

	// Search
	MatchEmbeddings(ctx context.Context, queryEmb []float32, threshold float64, limit int, docIDs []uuid.UUID, userID *uuid.UUID, folderID *uuid.UUID) ([]model.SearchResult, error)
	KeywordSearch(ctx context.Context, queryText string, limit int, docIDs []uuid.UUID, userID *uuid.UUID, folderID *uuid.UUID) ([]model.SearchResult, error)
	HybridSearch(ctx context.Context, queryEmb []float32, queryText string, threshold float64, limit int, docIDs []uuid.UUID, userID *uuid.UUID, folderID *uuid.UUID) ([]model.SearchResult, error)

	// Tags
	CreateTag(ctx context.Context, tag *model.Tag) error
	ListTags(ctx context.Context) ([]model.Tag, error)
	DeleteTag(ctx context.Context, id uuid.UUID) error
	AddDocumentTag(ctx context.Context, documentID, tagID uuid.UUID) error
	RemoveDocumentTag(ctx context.Context, documentID, tagID uuid.UUID) error
	GetDocumentTags(ctx context.Context, documentID uuid.UUID) ([]model.Tag, error)

	// Users
	CreateUser(ctx context.Context, user *model.User) error
	GetUserByAPIKey(ctx context.Context, apiKey string) (*model.User, error)
	GetUserByEmail(ctx context.Context, email string) (*model.User, error)

	// Folders
	CreateFolder(ctx context.Context, folder *model.Folder) error
	GetFolder(ctx context.Context, id uuid.UUID, userID *uuid.UUID) (*model.Folder, error)
	ListFolders(ctx context.Context, userID *uuid.UUID, parentID *uuid.UUID) ([]model.Folder, error)
	DeleteFolder(ctx context.Context, id uuid.UUID, userID *uuid.UUID) error
	MoveDocumentToFolder(ctx context.Context, docID uuid.UUID, folderID *uuid.UUID, userID *uuid.UUID) error
	FolderDescendants(ctx context.Context, folderID uuid.UUID) ([]uuid.UUID, error)

	// Agent sessions
	CreateAgentSession(ctx context.Context, session *model.AgentSession) error
	GetAgentSession(ctx context.Context, id uuid.UUID, userID *uuid.UUID) (*model.AgentSession, error)
	ListAgentSessions(ctx context.Context, userID *uuid.UUID) ([]model.AgentSession, error)
	DeleteAgentSession(ctx context.Context, id uuid.UUID, userID *uuid.UUID) error

	// Agent messages
	InsertAgentMessage(ctx context.Context, msg *model.AgentMessage) error
	ListAgentMessages(ctx context.Context, sessionID uuid.UUID) ([]model.AgentMessage, error)

	// Recovery
	GetProcessingDocumentIDs(ctx context.Context) ([]uuid.UUID, error)

	// Lifecycle
	Close()
}
