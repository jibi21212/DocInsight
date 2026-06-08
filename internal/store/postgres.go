package store

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/docinsight/backend/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	config.MaxConns = 20
	config.MinConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Close() {
	s.pool.Close()
}

func (s *PostgresStore) InsertDocument(ctx context.Context, doc *model.Document, userID *uuid.UUID) error {
	sourceType := doc.SourceType
	if sourceType == "" {
		sourceType = model.SourceTypePDF
	}
	var uid *uuid.UUID
	if userID != nil {
		uid = userID
	} else if doc.UserID != nil {
		uid = doc.UserID
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO documents (id, name, file_path, file_size, status, page_count, source_type, source_url, user_id, folder_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		doc.ID, doc.Name, doc.FilePath, doc.FileSize, doc.Status, doc.PageCount, sourceType, doc.SourceURL, uid, doc.FolderID,
	)
	if err == nil && uid != nil {
		doc.UserID = uid
	}
	return err
}

func (s *PostgresStore) GetDocument(ctx context.Context, id uuid.UUID, userID *uuid.UUID) (*model.Document, error) {
	doc := &model.Document{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, upload_date, page_count, status, file_path, file_size,
		        error_message, source_type, source_url, user_id, folder_id, created_at, updated_at
		 FROM documents WHERE id = $1 AND ($2::uuid IS NULL OR user_id = $2)`, id, userID,
	).Scan(
		&doc.ID, &doc.Name, &doc.UploadDate, &doc.PageCount, &doc.Status,
		&doc.FilePath, &doc.FileSize, &doc.ErrorMessage, &doc.SourceType, &doc.SourceURL,
		&doc.UserID, &doc.FolderID, &doc.CreatedAt, &doc.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return doc, err
}

func (s *PostgresStore) ListDocuments(ctx context.Context, page, pageSize int, status *string, userID *uuid.UUID, folderID *uuid.UUID) ([]model.Document, int, error) {
	// Count query: always include user filter as $1
	countQuery := "SELECT COUNT(*) FROM documents WHERE ($1::uuid IS NULL OR user_id = $1)"
	args := []interface{}{userID}
	paramIdx := 2
	if status != nil {
		countQuery += fmt.Sprintf(" AND status = $%d", paramIdx)
		args = append(args, *status)
		paramIdx++
	}
	if folderID != nil {
		ids, err := s.FolderDescendants(ctx, *folderID)
		if err != nil {
			return nil, 0, err
		}
		if len(ids) == 0 {
			return []model.Document{}, 0, nil
		}
		countQuery += fmt.Sprintf(" AND folder_id = ANY($%d)", paramIdx)
		args = append(args, ids)
		paramIdx++
	}

	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Data query
	dataQuery := "SELECT id, name, upload_date, page_count, status, file_path, file_size, error_message, source_type, source_url, user_id, folder_id, created_at, updated_at FROM documents WHERE ($1::uuid IS NULL OR user_id = $1)"
	dataArgs := []interface{}{userID}
	dataIdx := 2

	if status != nil {
		dataQuery += fmt.Sprintf(" AND status = $%d", dataIdx)
		dataArgs = append(dataArgs, *status)
		dataIdx++
	}
	if folderID != nil {
		ids, err := s.FolderDescendants(ctx, *folderID)
		if err != nil {
			return nil, 0, err
		}
		dataQuery += fmt.Sprintf(" AND folder_id = ANY($%d)", dataIdx)
		dataArgs = append(dataArgs, ids)
		dataIdx++
	}

	dataQuery += " ORDER BY created_at DESC"
	dataQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", dataIdx, dataIdx+1)

	offset := (page - 1) * pageSize
	dataArgs = append(dataArgs, pageSize, offset)

	rows, err := s.pool.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var docs []model.Document
	for rows.Next() {
		var doc model.Document
		if err := rows.Scan(
			&doc.ID, &doc.Name, &doc.UploadDate, &doc.PageCount, &doc.Status,
			&doc.FilePath, &doc.FileSize, &doc.ErrorMessage, &doc.SourceType, &doc.SourceURL,
			&doc.UserID, &doc.FolderID, &doc.CreatedAt, &doc.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		docs = append(docs, doc)
	}

	return docs, total, rows.Err()
}

func (s *PostgresStore) UpdateDocumentStatus(ctx context.Context, id uuid.UUID, status model.DocumentStatus, errMsg *string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE documents SET status = $1, error_message = $2 WHERE id = $3`,
		status, errMsg, id,
	)
	return err
}

func (s *PostgresStore) UpdateDocumentPageCount(ctx context.Context, id uuid.UUID, pageCount int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE documents SET page_count = $1 WHERE id = $2`,
		pageCount, id,
	)
	return err
}

func (s *PostgresStore) DeleteDocument(ctx context.Context, id uuid.UUID, userID *uuid.UUID) (string, error) {
	var filePath string
	err := s.pool.QueryRow(ctx,
		`SELECT file_path FROM documents WHERE id = $1 AND ($2::uuid IS NULL OR user_id = $2)`,
		id, userID,
	).Scan(&filePath)
	if err == pgx.ErrNoRows {
		return "", fmt.Errorf("document not found")
	}
	if err != nil {
		return "", err
	}

	_, err = s.pool.Exec(ctx,
		`DELETE FROM documents WHERE id = $1 AND ($2::uuid IS NULL OR user_id = $2)`,
		id, userID,
	)
	if err != nil {
		return "", err
	}

	return filePath, nil
}

func (s *PostgresStore) InsertChunks(ctx context.Context, chunks []model.Chunk) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(chunks))

	batch := &pgx.Batch{}
	for i := range chunks {
		c := &chunks[i]
		metaJSON, err := json.Marshal(c.Metadata)
		if err != nil {
			return nil, fmt.Errorf("marshal chunk metadata: %w", err)
		}
		batch.Queue(
			`INSERT INTO chunks (id, document_id, content, page_number, chunk_index, metadata)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			c.ID, c.DocumentID, c.Content, c.PageNumber, c.ChunkIndex, metaJSON,
		)
		ids = append(ids, c.ID)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range chunks {
		if _, err := br.Exec(); err != nil {
			return nil, fmt.Errorf("insert chunk: %w", err)
		}
	}

	return ids, nil
}

func (s *PostgresStore) GetChunksByDocumentID(ctx context.Context, documentID uuid.UUID) ([]model.Chunk, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, document_id, content, page_number, chunk_index, metadata, created_at
		 FROM chunks WHERE document_id = $1 ORDER BY chunk_index ASC`, documentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []model.Chunk
	for rows.Next() {
		var c model.Chunk
		var metaJSON []byte
		if err := rows.Scan(&c.ID, &c.DocumentID, &c.Content, &c.PageNumber, &c.ChunkIndex, &metaJSON, &c.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metaJSON, &c.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal chunk metadata: %w", err)
		}
		chunks = append(chunks, c)
	}

	return chunks, rows.Err()
}

// GetChunkByID returns a single chunk by ID, scoped to the owning user when
// provided. Returns (nil, nil) when the chunk does not exist or is not visible.
func (s *PostgresStore) GetChunkByID(ctx context.Context, chunkID uuid.UUID, userID *uuid.UUID) (*model.Chunk, error) {
	var c model.Chunk
	var metaJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT c.id, c.document_id, c.content, c.page_number, c.chunk_index, c.metadata, c.created_at
		 FROM chunks c
		 JOIN documents d ON c.document_id = d.id
		 WHERE c.id = $1 AND ($2::uuid IS NULL OR d.user_id = $2)`,
		chunkID, userID,
	).Scan(&c.ID, &c.DocumentID, &c.Content, &c.PageNumber, &c.ChunkIndex, &metaJSON, &c.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(metaJSON, &c.Metadata); err != nil {
		return nil, fmt.Errorf("unmarshal chunk metadata: %w", err)
	}
	return &c, nil
}

func (s *PostgresStore) DeleteChunksByDocumentID(ctx context.Context, documentID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM chunks WHERE document_id = $1`, documentID)
	return err
}

func (s *PostgresStore) InsertEmbeddings(ctx context.Context, chunkIDs []uuid.UUID, embeddings [][]float32) error {
	batch := &pgx.Batch{}
	for i, chunkID := range chunkIDs {
		vec := pgvector.NewVector(embeddings[i])
		batch.Queue(
			`INSERT INTO embeddings (chunk_id, embedding) VALUES ($1, $2)`,
			chunkID, vec,
		)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range chunkIDs {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert embedding: %w", err)
		}
	}

	return nil
}

func (s *PostgresStore) MatchEmbeddings(ctx context.Context, queryEmb []float32, threshold float64, limit int, docIDs []uuid.UUID, userID *uuid.UUID, folderID *uuid.UUID) ([]model.SearchResult, error) {
	_ = userID   // Postgres impl is a stub here; full user filtering would require updating the match_embeddings RPC.
	_ = folderID // Same; folder scoping is enforced at the SQLite path. Postgres impl needs an RPC update.
	vec := pgvector.NewVector(queryEmb)

	var rows pgx.Rows
	var err error

	if len(docIDs) > 0 {
		rows, err = s.pool.Query(ctx,
			`SELECT chunk_id, content, similarity, page_number, chunk_index, metadata, document_id, document_name
			 FROM match_embeddings($1, $2, $3, $4)`,
			vec, threshold, limit, docIDs,
		)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT chunk_id, content, similarity, page_number, chunk_index, metadata, document_id, document_name
			 FROM match_embeddings($1, $2, $3, NULL)`,
			vec, threshold, limit,
		)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.SearchResult
	for rows.Next() {
		var r model.SearchResult
		var metaJSON []byte
		if err := rows.Scan(&r.ChunkID, &r.Content, &r.Similarity, &r.PageNumber, &r.ChunkIndex, &metaJSON, &r.DocumentID, &r.DocumentName); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metaJSON, &r.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal result metadata: %w", err)
		}
		// Round similarity to avoid floating point noise
		r.Similarity = math.Round(r.Similarity*10000) / 10000
		results = append(results, r)
	}

	return results, rows.Err()
}

// --- User Methods ---

func (s *PostgresStore) CreateUser(ctx context.Context, user *model.User) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO users (id, email, api_key, name) VALUES ($1, $2, $3, $4)`,
		user.ID, user.Email, user.APIKey, user.Name,
	)
	return err
}

func (s *PostgresStore) GetUserByAPIKey(ctx context.Context, apiKey string) (*model.User, error) {
	var u model.User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, api_key, name, created_at FROM users WHERE api_key = $1`, apiKey,
	).Scan(&u.ID, &u.Email, &u.APIKey, &u.Name, &u.CreatedAt)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	var u model.User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, api_key, name, created_at FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.APIKey, &u.Name, &u.CreatedAt)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (s *PostgresStore) KeywordSearch(ctx context.Context, queryText string, limit int, docIDs []uuid.UUID, userID *uuid.UUID, folderID *uuid.UUID) ([]model.SearchResult, error) {
	_ = userID
	_ = folderID
	// PostgreSQL implementation would use tsvector/tsquery — stub for now
	return []model.SearchResult{}, nil
}

func (s *PostgresStore) HybridSearch(ctx context.Context, queryEmb []float32, queryText string, threshold float64, limit int, docIDs []uuid.UUID, userID *uuid.UUID, folderID *uuid.UUID) ([]model.SearchResult, error) {
	// PostgreSQL implementation would combine pg_trgm/tsvector with pgvector — stub for now
	return s.MatchEmbeddings(ctx, queryEmb, threshold, limit, docIDs, userID, folderID)
}

func (s *PostgresStore) GetProcessingDocumentIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM documents WHERE status = 'processing'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

// --- Tag Methods ---

func (s *PostgresStore) CreateTag(ctx context.Context, tag *model.Tag) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO tags (id, name, color) VALUES ($1, $2, $3)`,
		tag.ID, tag.Name, tag.Color,
	)
	return err
}

func (s *PostgresStore) ListTags(ctx context.Context) ([]model.Tag, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, color, created_at FROM tags ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []model.Tag
	for rows.Next() {
		var t model.Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &t.CreatedAt); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func (s *PostgresStore) DeleteTag(ctx context.Context, id uuid.UUID) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM tags WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("tag not found")
	}
	return nil
}

func (s *PostgresStore) AddDocumentTag(ctx context.Context, documentID, tagID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO document_tags (document_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		documentID, tagID,
	)
	return err
}

func (s *PostgresStore) RemoveDocumentTag(ctx context.Context, documentID, tagID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM document_tags WHERE document_id = $1 AND tag_id = $2`,
		documentID, tagID,
	)
	return err
}

func (s *PostgresStore) GetDocumentTags(ctx context.Context, documentID uuid.UUID) ([]model.Tag, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.name, t.color, t.created_at
		 FROM tags t
		 JOIN document_tags dt ON dt.tag_id = t.id
		 WHERE dt.document_id = $1
		 ORDER BY t.name`,
		documentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []model.Tag
	for rows.Next() {
		var t model.Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &t.CreatedAt); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// --- Folder Methods ---

func (s *PostgresStore) CreateFolder(ctx context.Context, folder *model.Folder) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO folders (id, user_id, parent_id, name) VALUES ($1, $2, $3, $4)`,
		folder.ID, folder.UserID, folder.ParentID, folder.Name,
	)
	return err
}

func (s *PostgresStore) GetFolder(ctx context.Context, id uuid.UUID, userID *uuid.UUID) (*model.Folder, error) {
	var f model.Folder
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, parent_id, name, created_at FROM folders
		 WHERE id = $1 AND ($2::uuid IS NULL OR user_id = $2)`,
		id, userID,
	).Scan(&f.ID, &f.UserID, &f.ParentID, &f.Name, &f.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (s *PostgresStore) ListFolders(ctx context.Context, userID *uuid.UUID, parentID *uuid.UUID) ([]model.Folder, error) {
	var rows pgx.Rows
	var err error
	if parentID == nil {
		rows, err = s.pool.Query(ctx,
			`SELECT id, user_id, parent_id, name, created_at FROM folders
			 WHERE parent_id IS NULL AND ($1::uuid IS NULL OR user_id = $1)
			 ORDER BY name`,
			userID,
		)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT id, user_id, parent_id, name, created_at FROM folders
			 WHERE parent_id = $1 AND ($2::uuid IS NULL OR user_id = $2)
			 ORDER BY name`,
			parentID, userID,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []model.Folder
	for rows.Next() {
		var f model.Folder
		if err := rows.Scan(&f.ID, &f.UserID, &f.ParentID, &f.Name, &f.CreatedAt); err != nil {
			return nil, err
		}
		folders = append(folders, f)
	}
	return folders, rows.Err()
}

func (s *PostgresStore) DeleteFolder(ctx context.Context, id uuid.UUID, userID *uuid.UUID) error {
	ct, err := s.pool.Exec(ctx,
		`DELETE FROM folders WHERE id = $1 AND ($2::uuid IS NULL OR user_id = $2)`,
		id, userID,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("folder not found")
	}
	return nil
}

func (s *PostgresStore) MoveDocumentToFolder(ctx context.Context, docID uuid.UUID, folderID *uuid.UUID, userID *uuid.UUID) error {
	doc, err := s.GetDocument(ctx, docID, userID)
	if err != nil {
		return err
	}
	if doc == nil {
		return fmt.Errorf("document not found")
	}
	if folderID != nil {
		folder, err := s.GetFolder(ctx, *folderID, userID)
		if err != nil {
			return err
		}
		if folder == nil {
			return fmt.Errorf("folder not found")
		}
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE documents SET folder_id = $1 WHERE id = $2 AND ($3::uuid IS NULL OR user_id = $3)`,
		folderID, docID, userID,
	)
	return err
}

// --- Agent Session Methods ---

func (s *PostgresStore) CreateAgentSession(ctx context.Context, session *model.AgentSession) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agent_sessions (id, user_id, folder_id, title, provider, model)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		session.ID, session.UserID, session.FolderID, session.Title, session.Provider, session.Model,
	)
	return err
}

func (s *PostgresStore) GetAgentSession(ctx context.Context, id uuid.UUID, userID *uuid.UUID) (*model.AgentSession, error) {
	var sess model.AgentSession
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, folder_id, title, provider, model, created_at FROM agent_sessions
		 WHERE id = $1 AND ($2::uuid IS NULL OR user_id = $2)`,
		id, userID,
	).Scan(&sess.ID, &sess.UserID, &sess.FolderID, &sess.Title, &sess.Provider, &sess.Model, &sess.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *PostgresStore) ListAgentSessions(ctx context.Context, userID *uuid.UUID) ([]model.AgentSession, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, folder_id, title, provider, model, created_at FROM agent_sessions
		 WHERE ($1::uuid IS NULL OR user_id = $1)
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []model.AgentSession
	for rows.Next() {
		var sess model.AgentSession
		if err := rows.Scan(&sess.ID, &sess.UserID, &sess.FolderID, &sess.Title, &sess.Provider, &sess.Model, &sess.CreatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

func (s *PostgresStore) DeleteAgentSession(ctx context.Context, id uuid.UUID, userID *uuid.UUID) error {
	ct, err := s.pool.Exec(ctx,
		`DELETE FROM agent_sessions WHERE id = $1 AND ($2::uuid IS NULL OR user_id = $2)`,
		id, userID,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("session not found")
	}
	return nil
}

func (s *PostgresStore) InsertAgentMessage(ctx context.Context, msg *model.AgentMessage) error {
	citationsStr, err := msg.MarshalCitations()
	if err != nil {
		return fmt.Errorf("marshal citations: %w", err)
	}
	var citations *string
	if citationsStr != "" {
		citations = &citationsStr
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO agent_messages (id, session_id, role, content, citations) VALUES ($1, $2, $3, $4, $5)`,
		msg.ID, msg.SessionID, msg.Role, msg.Content, citations,
	)
	return err
}

func (s *PostgresStore) ListAgentMessages(ctx context.Context, sessionID uuid.UUID) ([]model.AgentMessage, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, session_id, role, content, citations, created_at FROM agent_messages
		 WHERE session_id = $1 ORDER BY created_at ASC, id ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []model.AgentMessage
	for rows.Next() {
		var msg model.AgentMessage
		var citations *string
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &citations, &msg.CreatedAt); err != nil {
			return nil, err
		}
		if citations != nil {
			_ = msg.UnmarshalCitations(*citations)
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func (s *PostgresStore) FolderDescendants(ctx context.Context, folderID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `
		WITH RECURSIVE tree(id) AS (
			SELECT id FROM folders WHERE id = $1
			UNION ALL
			SELECT f.id FROM folders f JOIN tree t ON f.parent_id = t.id
		)
		SELECT id FROM tree
	`, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
