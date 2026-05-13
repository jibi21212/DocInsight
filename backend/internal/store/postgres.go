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

func (s *PostgresStore) InsertDocument(ctx context.Context, doc *model.Document) error {
	sourceType := doc.SourceType
	if sourceType == "" {
		sourceType = model.SourceTypePDF
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO documents (id, name, file_path, file_size, status, page_count, source_type, source_url)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		doc.ID, doc.Name, doc.FilePath, doc.FileSize, doc.Status, doc.PageCount, sourceType, doc.SourceURL,
	)
	return err
}

func (s *PostgresStore) GetDocument(ctx context.Context, id uuid.UUID) (*model.Document, error) {
	doc := &model.Document{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, upload_date, page_count, status, file_path, file_size,
		        error_message, source_type, source_url, created_at, updated_at
		 FROM documents WHERE id = $1`, id,
	).Scan(
		&doc.ID, &doc.Name, &doc.UploadDate, &doc.PageCount, &doc.Status,
		&doc.FilePath, &doc.FileSize, &doc.ErrorMessage, &doc.SourceType, &doc.SourceURL,
		&doc.CreatedAt, &doc.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return doc, err
}

func (s *PostgresStore) ListDocuments(ctx context.Context, page, pageSize int, status *string) ([]model.Document, int, error) {
	// Count query
	countQuery := "SELECT COUNT(*) FROM documents"
	args := []interface{}{}
	if status != nil {
		countQuery += " WHERE status = $1"
		args = append(args, *status)
	}

	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Data query
	dataQuery := "SELECT id, name, upload_date, page_count, status, file_path, file_size, error_message, source_type, source_url, created_at, updated_at FROM documents"
	dataArgs := []interface{}{}
	paramIdx := 1

	if status != nil {
		dataQuery += fmt.Sprintf(" WHERE status = $%d", paramIdx)
		dataArgs = append(dataArgs, *status)
		paramIdx++
	}

	dataQuery += " ORDER BY created_at DESC"
	dataQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", paramIdx, paramIdx+1)

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
			&doc.CreatedAt, &doc.UpdatedAt,
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

func (s *PostgresStore) DeleteDocument(ctx context.Context, id uuid.UUID) (string, error) {
	var filePath string
	err := s.pool.QueryRow(ctx, `SELECT file_path FROM documents WHERE id = $1`, id).Scan(&filePath)
	if err == pgx.ErrNoRows {
		return "", fmt.Errorf("document not found")
	}
	if err != nil {
		return "", err
	}

	_, err = s.pool.Exec(ctx, `DELETE FROM documents WHERE id = $1`, id)
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

func (s *PostgresStore) MatchEmbeddings(ctx context.Context, queryEmb []float32, threshold float64, limit int, docIDs []uuid.UUID) ([]model.SearchResult, error) {
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

func (s *PostgresStore) KeywordSearch(ctx context.Context, queryText string, limit int, docIDs []uuid.UUID) ([]model.SearchResult, error) {
	// PostgreSQL implementation would use tsvector/tsquery — stub for now
	return []model.SearchResult{}, nil
}

func (s *PostgresStore) HybridSearch(ctx context.Context, queryEmb []float32, queryText string, threshold float64, limit int, docIDs []uuid.UUID) ([]model.SearchResult, error) {
	// PostgreSQL implementation would combine pg_trgm/tsvector with pgvector — stub for now
	return s.MatchEmbeddings(ctx, queryEmb, threshold, limit, docIDs)
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
