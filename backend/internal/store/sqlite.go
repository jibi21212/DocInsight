package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/docinsight/backend/internal/model"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB

	// In-memory embedding index for brute-force cosine similarity search.
	// SQLite doesn't have pgvector, so we keep embeddings in memory.
	mu         sync.RWMutex
	embeddings map[uuid.UUID][]float32 // chunk_id -> embedding
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Single writer, multiple readers
	db.SetMaxOpenConns(1)

	s := &SQLiteStore{
		db:         db,
		embeddings: make(map[uuid.UUID][]float32),
	}

	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate sqlite: %w", err)
	}

	// Load existing embeddings into memory
	if err := s.loadEmbeddings(); err != nil {
		db.Close()
		return nil, fmt.Errorf("load embeddings: %w", err)
	}

	return s, nil
}

func (s *SQLiteStore) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS documents (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			upload_date TEXT DEFAULT (datetime('now')),
			page_count INTEGER DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
			file_path TEXT NOT NULL,
			file_size INTEGER DEFAULT 0,
			error_message TEXT,
			created_at TEXT DEFAULT (datetime('now')),
			updated_at TEXT DEFAULT (datetime('now'))
		);

		CREATE TABLE IF NOT EXISTS chunks (
			id TEXT PRIMARY KEY,
			document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			content TEXT NOT NULL,
			page_number INTEGER NOT NULL DEFAULT 1,
			chunk_index INTEGER NOT NULL DEFAULT 0,
			metadata TEXT DEFAULT '{}',
			created_at TEXT DEFAULT (datetime('now'))
		);

		CREATE TABLE IF NOT EXISTS embeddings (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			chunk_id TEXT NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
			embedding BLOB NOT NULL,
			created_at TEXT DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS idx_chunks_document_id ON chunks(document_id);
		CREATE INDEX IF NOT EXISTS idx_embeddings_chunk_id ON embeddings(chunk_id);
		CREATE INDEX IF NOT EXISTS idx_documents_status ON documents(status);

		-- Auto-update updated_at on every UPDATE (handled in Go, not via trigger to avoid recursion)
	`)
	if err != nil {
		return err
	}

	// Add source_type and source_url columns (idempotent migration)
	for _, stmt := range []string{
		`ALTER TABLE documents ADD COLUMN source_type TEXT NOT NULL DEFAULT 'pdf'`,
		`ALTER TABLE documents ADD COLUMN source_url TEXT`,
	} {
		_, err := s.db.Exec(stmt)
		if err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return err
		}
	}

	// FTS5 full-text search index
	_, err = s.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
			chunk_id UNINDEXED,
			document_id UNINDEXED,
			content
		);
	`)
	if err != nil {
		return err
	}

	// Backfill FTS5 from existing chunks (idempotent — skip if already populated)
	var ftsCount int
	s.db.QueryRow(`SELECT count(*) FROM chunks_fts`).Scan(&ftsCount)
	if ftsCount == 0 {
		_, err = s.db.Exec(`
			INSERT INTO chunks_fts(chunk_id, document_id, content)
			SELECT id, document_id, content FROM chunks
		`)
		if err != nil {
			return err
		}
	}

	// Users table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			api_key TEXT NOT NULL UNIQUE,
			name TEXT DEFAULT '',
			created_at TEXT DEFAULT (datetime('now'))
		);
	`)
	if err != nil {
		return err
	}

	// Add user_id column to documents (idempotent)
	_, addErr := s.db.Exec(`ALTER TABLE documents ADD COLUMN user_id TEXT REFERENCES users(id)`)
	if addErr != nil && !strings.Contains(addErr.Error(), "duplicate column") {
		return addErr
	}

	// Tags tables
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS tags (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			color TEXT DEFAULT '#6366f1',
			created_at TEXT DEFAULT (datetime('now'))
		);

		CREATE TABLE IF NOT EXISTS document_tags (
			document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
			PRIMARY KEY (document_id, tag_id)
		);

		CREATE INDEX IF NOT EXISTS idx_document_tags_document ON document_tags(document_id);
		CREATE INDEX IF NOT EXISTS idx_document_tags_tag ON document_tags(tag_id);
	`)
	if err != nil {
		return err
	}

	return nil
}

func (s *SQLiteStore) loadEmbeddings() error {
	rows, err := s.db.Query(`SELECT chunk_id, embedding FROM embeddings`)
	if err != nil {
		return err
	}
	defer rows.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	for rows.Next() {
		var chunkIDStr string
		var embBlob []byte
		if err := rows.Scan(&chunkIDStr, &embBlob); err != nil {
			return err
		}
		chunkID, err := uuid.Parse(chunkIDStr)
		if err != nil {
			continue
		}
		s.embeddings[chunkID] = blobToFloat32s(embBlob)
	}

	return rows.Err()
}

func (s *SQLiteStore) Close() {
	s.db.Close()
}

// --- Documents ---

func (s *SQLiteStore) InsertDocument(ctx context.Context, doc *model.Document) error {
	sourceType := doc.SourceType
	if sourceType == "" {
		sourceType = model.SourceTypePDF
	}
	var sourceURL *string
	if doc.SourceURL != nil {
		sourceURL = doc.SourceURL
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO documents (id, name, file_path, file_size, status, page_count, source_type, source_url) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		doc.ID.String(), doc.Name, doc.FilePath, doc.FileSize, doc.Status, doc.PageCount, sourceType, sourceURL,
	)
	return err
}

func (s *SQLiteStore) GetDocument(ctx context.Context, id uuid.UUID) (*model.Document, error) {
	doc := &model.Document{}
	var idStr, uploadDate, status, createdAt, updatedAt string
	var errMsg, sourceURL sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, upload_date, page_count, status, file_path, file_size, error_message, source_type, source_url, created_at, updated_at
		 FROM documents WHERE id = ?`, id.String(),
	).Scan(&idStr, &doc.Name, &uploadDate, &doc.PageCount, &status, &doc.FilePath, &doc.FileSize, &errMsg, &doc.SourceType, &sourceURL, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	doc.ID, _ = uuid.Parse(idStr)
	doc.Status = model.DocumentStatus(status)
	doc.UploadDate = parseTime(uploadDate)
	doc.CreatedAt = parseTime(createdAt)
	doc.UpdatedAt = parseTime(updatedAt)
	if errMsg.Valid {
		doc.ErrorMessage = &errMsg.String
	}
	if sourceURL.Valid {
		doc.SourceURL = &sourceURL.String
	}

	return doc, nil
}

func (s *SQLiteStore) ListDocuments(ctx context.Context, page, pageSize int, status *string) ([]model.Document, int, error) {
	// Count
	countQuery := "SELECT COUNT(*) FROM documents"
	countArgs := []interface{}{}
	if status != nil {
		countQuery += " WHERE status = ?"
		countArgs = append(countArgs, *status)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Data
	dataQuery := "SELECT id, name, upload_date, page_count, status, file_path, file_size, error_message, source_type, source_url, created_at, updated_at FROM documents"
	dataArgs := []interface{}{}
	if status != nil {
		dataQuery += " WHERE status = ?"
		dataArgs = append(dataArgs, *status)
	}
	dataQuery += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	dataArgs = append(dataArgs, pageSize, (page-1)*pageSize)

	rows, err := s.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var docs []model.Document
	for rows.Next() {
		var doc model.Document
		var idStr, uploadDate, statusStr, createdAt, updatedAt string
		var errMsg, sourceURL sql.NullString

		if err := rows.Scan(&idStr, &doc.Name, &uploadDate, &doc.PageCount, &statusStr, &doc.FilePath, &doc.FileSize, &errMsg, &doc.SourceType, &sourceURL, &createdAt, &updatedAt); err != nil {
			return nil, 0, err
		}

		doc.ID, _ = uuid.Parse(idStr)
		doc.Status = model.DocumentStatus(statusStr)
		doc.UploadDate = parseTime(uploadDate)
		doc.CreatedAt = parseTime(createdAt)
		doc.UpdatedAt = parseTime(updatedAt)
		if errMsg.Valid {
			doc.ErrorMessage = &errMsg.String
		}
		if sourceURL.Valid {
			doc.SourceURL = &sourceURL.String
		}
		docs = append(docs, doc)
	}

	return docs, total, rows.Err()
}

func (s *SQLiteStore) UpdateDocumentStatus(ctx context.Context, id uuid.UUID, status model.DocumentStatus, errMsg *string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE documents SET status = ?, error_message = ?, updated_at = datetime('now') WHERE id = ?`,
		status, errMsg, id.String(),
	)
	return err
}

func (s *SQLiteStore) UpdateDocumentPageCount(ctx context.Context, id uuid.UUID, pageCount int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE documents SET page_count = ?, updated_at = datetime('now') WHERE id = ?`,
		pageCount, id.String(),
	)
	return err
}

func (s *SQLiteStore) DeleteDocument(ctx context.Context, id uuid.UUID) (string, error) {
	var filePath string
	err := s.db.QueryRowContext(ctx, `SELECT file_path FROM documents WHERE id = ?`, id.String()).Scan(&filePath)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("document not found")
	}
	if err != nil {
		return "", err
	}

	// Get chunk IDs to remove from memory index
	chunkRows, err := s.db.QueryContext(ctx, `SELECT id FROM chunks WHERE document_id = ?`, id.String())
	if err == nil {
		defer chunkRows.Close()
		s.mu.Lock()
		for chunkRows.Next() {
			var cid string
			if chunkRows.Scan(&cid) == nil {
				if parsed, err := uuid.Parse(cid); err == nil {
					delete(s.embeddings, parsed)
				}
			}
		}
		s.mu.Unlock()
	}

	_, err = s.db.ExecContext(ctx, `DELETE FROM documents WHERE id = ?`, id.String())
	return filePath, err
}

// --- Chunks ---

func (s *SQLiteStore) InsertChunks(ctx context.Context, chunks []model.Chunk) ([]uuid.UUID, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO chunks (id, document_id, content, page_number, chunk_index, metadata) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	ftsStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO chunks_fts(chunk_id, document_id, content) VALUES (?, ?, ?)`)
	if err != nil {
		return nil, err
	}
	defer ftsStmt.Close()

	ids := make([]uuid.UUID, len(chunks))
	for i := range chunks {
		c := &chunks[i]
		metaJSON, err := json.Marshal(c.Metadata)
		if err != nil {
			return nil, err
		}
		_, err = stmt.ExecContext(ctx, c.ID.String(), c.DocumentID.String(), c.Content, c.PageNumber, c.ChunkIndex, string(metaJSON))
		if err != nil {
			return nil, fmt.Errorf("insert chunk %d: %w", i, err)
		}
		// Sync FTS5 index
		_, err = ftsStmt.ExecContext(ctx, c.ID.String(), c.DocumentID.String(), c.Content)
		if err != nil {
			return nil, fmt.Errorf("insert fts chunk %d: %w", i, err)
		}
		ids[i] = c.ID
	}

	return ids, tx.Commit()
}

func (s *SQLiteStore) GetChunksByDocumentID(ctx context.Context, documentID uuid.UUID) ([]model.Chunk, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, document_id, content, page_number, chunk_index, metadata, created_at
		 FROM chunks WHERE document_id = ? ORDER BY chunk_index ASC`, documentID.String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []model.Chunk
	for rows.Next() {
		var c model.Chunk
		var idStr, docIDStr, metaStr, createdAt string
		if err := rows.Scan(&idStr, &docIDStr, &c.Content, &c.PageNumber, &c.ChunkIndex, &metaStr, &createdAt); err != nil {
			return nil, err
		}
		c.ID, _ = uuid.Parse(idStr)
		c.DocumentID, _ = uuid.Parse(docIDStr)
		c.CreatedAt = parseTime(createdAt)
		json.Unmarshal([]byte(metaStr), &c.Metadata)
		chunks = append(chunks, c)
	}

	return chunks, rows.Err()
}

func (s *SQLiteStore) DeleteChunksByDocumentID(ctx context.Context, documentID uuid.UUID) error {
	// Remove embeddings from memory for these chunks
	chunkRows, err := s.db.QueryContext(ctx, `SELECT id FROM chunks WHERE document_id = ?`, documentID.String())
	if err == nil {
		defer chunkRows.Close()
		s.mu.Lock()
		for chunkRows.Next() {
			var cid string
			if chunkRows.Scan(&cid) == nil {
				if parsed, err := uuid.Parse(cid); err == nil {
					delete(s.embeddings, parsed)
				}
			}
		}
		s.mu.Unlock()
	}

	// Clean FTS5 index
	_, _ = s.db.ExecContext(ctx, `DELETE FROM chunks_fts WHERE document_id = ?`, documentID.String())

	_, err = s.db.ExecContext(ctx, `DELETE FROM chunks WHERE document_id = ?`, documentID.String())
	return err
}

// --- Embeddings ---

func (s *SQLiteStore) InsertEmbeddings(ctx context.Context, chunkIDs []uuid.UUID, embeddings [][]float32) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO embeddings (chunk_id, embedding) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	for i, chunkID := range chunkIDs {
		blob := float32sToBlob(embeddings[i])
		if _, err := stmt.ExecContext(ctx, chunkID.String(), blob); err != nil {
			return fmt.Errorf("insert embedding %d: %w", i, err)
		}
		// Cache in memory for search
		s.embeddings[chunkID] = embeddings[i]
	}

	return tx.Commit()
}

// --- Search (brute-force cosine similarity in Go) ---

func (s *SQLiteStore) MatchEmbeddings(ctx context.Context, queryEmb []float32, threshold float64, limit int, docIDs []uuid.UUID) ([]model.SearchResult, error) {
	// Build set of allowed document IDs
	docIDSet := make(map[string]bool)
	for _, id := range docIDs {
		docIDSet[id.String()] = true
	}
	filterByDoc := len(docIDs) > 0

	// Get all chunks with their document info
	query := `
		SELECT c.id, c.content, c.page_number, c.chunk_index, c.metadata, c.document_id, d.name, d.status, d.source_type, d.source_url
		FROM chunks c
		JOIN documents d ON d.id = c.document_id
		WHERE d.status = 'completed'
	`
	if filterByDoc {
		placeholders := make([]string, len(docIDs))
		for i := range docIDs {
			placeholders[i] = "?"
		}
		query += " AND d.id IN (" + strings.Join(placeholders, ",") + ")"
	}

	args := []interface{}{}
	if filterByDoc {
		for _, id := range docIDs {
			args = append(args, id.String())
		}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type candidate struct {
		result     model.SearchResult
		similarity float64
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var candidates []candidate

	for rows.Next() {
		var idStr, docIDStr, docName, statusStr, metaStr, sourceType string
		var sourceURL sql.NullString
		var content string
		var pageNum, chunkIdx int

		if err := rows.Scan(&idStr, &content, &pageNum, &chunkIdx, &metaStr, &docIDStr, &docName, &statusStr, &sourceType, &sourceURL); err != nil {
			return nil, err
		}

		chunkID, _ := uuid.Parse(idStr)
		emb, exists := s.embeddings[chunkID]
		if !exists {
			continue
		}

		sim := cosineSimilarity(queryEmb, emb)
		if sim < threshold {
			continue
		}

		var meta model.ChunkMetadata
		json.Unmarshal([]byte(metaStr), &meta)

		docID, _ := uuid.Parse(docIDStr)

		result := model.SearchResult{
			ChunkID:      chunkID,
			Content:      content,
			Similarity:   math.Round(sim*10000) / 10000,
			PageNumber:   pageNum,
			ChunkIndex:   chunkIdx,
			Metadata:     meta,
			DocumentID:   docID,
			DocumentName: docName,
			SourceType:   sourceType,
		}
		if sourceURL.Valid {
			result.SourceURL = &sourceURL.String
		}

		candidates = append(candidates, candidate{
			similarity: sim,
			result:     result,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort by similarity descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].similarity > candidates[j].similarity
	})

	// Limit results
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	results := make([]model.SearchResult, len(candidates))
	for i, c := range candidates {
		results[i] = c.result
	}

	return results, nil
}

// --- Keyword Search (FTS5) ---

func (s *SQLiteStore) KeywordSearch(ctx context.Context, queryText string, limit int, docIDs []uuid.UUID) ([]model.SearchResult, error) {
	filterByDoc := len(docIDs) > 0

	query := `
		SELECT f.chunk_id, f.document_id, f.content, c.page_number, c.chunk_index, c.metadata,
		       d.name, d.source_type, d.source_url, bm25(chunks_fts) AS score
		FROM chunks_fts f
		JOIN chunks c ON c.id = f.chunk_id
		JOIN documents d ON d.id = f.document_id
		WHERE chunks_fts MATCH ? AND d.status = 'completed'
	`
	args := []interface{}{queryText}

	if filterByDoc {
		placeholders := make([]string, len(docIDs))
		for i, id := range docIDs {
			placeholders[i] = "?"
			args = append(args, id.String())
		}
		query += " AND d.id IN (" + strings.Join(placeholders, ",") + ")"
	}

	query += " ORDER BY score LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.SearchResult
	for rows.Next() {
		var chunkIDStr, docIDStr, docName, sourceType, metaStr string
		var sourceURL sql.NullString
		var content string
		var pageNum, chunkIdx int
		var bm25Score float64

		if err := rows.Scan(&chunkIDStr, &docIDStr, &content, &pageNum, &chunkIdx, &metaStr, &docName, &sourceType, &sourceURL, &bm25Score); err != nil {
			return nil, err
		}

		chunkID, _ := uuid.Parse(chunkIDStr)
		docID, _ := uuid.Parse(docIDStr)
		var meta model.ChunkMetadata
		json.Unmarshal([]byte(metaStr), &meta)

		r := model.SearchResult{
			ChunkID:      chunkID,
			Content:      content,
			Similarity:   0,
			KeywordScore: math.Abs(bm25Score), // BM25 returns negative values in FTS5
			PageNumber:   pageNum,
			ChunkIndex:   chunkIdx,
			Metadata:     meta,
			DocumentID:   docID,
			DocumentName: docName,
			SourceType:   sourceType,
			MatchType:    "keyword",
		}
		if sourceURL.Valid {
			r.SourceURL = &sourceURL.String
		}
		results = append(results, r)
	}

	return results, rows.Err()
}

// --- Hybrid Search (FTS5 + Cosine Similarity merged via RRF) ---

func (s *SQLiteStore) HybridSearch(ctx context.Context, queryEmb []float32, queryText string, threshold float64, limit int, docIDs []uuid.UUID) ([]model.SearchResult, error) {
	// Run both searches with expanded limit to get enough candidates for fusion
	expandedLimit := limit * 3
	if expandedLimit < 20 {
		expandedLimit = 20
	}

	// Step 1: Semantic search
	semanticResults, err := s.MatchEmbeddings(ctx, queryEmb, threshold, expandedLimit, docIDs)
	if err != nil {
		return nil, fmt.Errorf("semantic search: %w", err)
	}
	for i := range semanticResults {
		semanticResults[i].MatchType = "semantic"
	}

	// Step 2: Keyword search
	keywordResults, err := s.KeywordSearch(ctx, queryText, expandedLimit, docIDs)
	if err != nil {
		// FTS5 may fail on invalid syntax — fall back to semantic only
		return semanticResults, nil
	}

	// Step 3: Reciprocal Rank Fusion
	// RRF score = 1/(k+rank_semantic) + 1/(k+rank_keyword), k=60
	const k = 60.0

	type fusedResult struct {
		result   model.SearchResult
		rrfScore float64
	}

	resultMap := make(map[uuid.UUID]*fusedResult)

	// Add semantic results with their rank
	for rank, r := range semanticResults {
		rrfScore := 1.0 / (k + float64(rank))
		resultMap[r.ChunkID] = &fusedResult{
			result:   r,
			rrfScore: rrfScore,
		}
	}

	// Merge keyword results
	for rank, r := range keywordResults {
		rrfScore := 1.0 / (k + float64(rank))
		if existing, ok := resultMap[r.ChunkID]; ok {
			// Found in both — hybrid match
			existing.rrfScore += rrfScore
			existing.result.MatchType = "hybrid"
			existing.result.KeywordScore = r.KeywordScore
		} else {
			r.MatchType = "keyword"
			resultMap[r.ChunkID] = &fusedResult{
				result:   r,
				rrfScore: rrfScore,
			}
		}
	}

	// Sort by RRF score descending
	fused := make([]fusedResult, 0, len(resultMap))
	for _, fr := range resultMap {
		fused = append(fused, *fr)
	}
	sort.Slice(fused, func(i, j int) bool {
		return fused[i].rrfScore > fused[j].rrfScore
	})

	// Limit
	if len(fused) > limit {
		fused = fused[:limit]
	}

	results := make([]model.SearchResult, len(fused))
	for i, f := range fused {
		r := f.result
		r.Similarity = math.Round(f.rrfScore*10000) / 10000
		results[i] = r
	}

	return results, nil
}

// --- Recovery ---

func (s *SQLiteStore) GetProcessingDocumentIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM documents WHERE status = 'processing'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var idStr string
		if err := rows.Scan(&idStr); err != nil {
			return nil, err
		}
		if id, err := uuid.Parse(idStr); err == nil {
			ids = append(ids, id)
		}
	}

	return ids, rows.Err()
}

// --- Helpers ---

func parseTime(s string) time.Time {
	t, _ := time.Parse("2006-01-02 15:04:05", s)
	return t
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func float32sToBlob(floats []float32) []byte {
	buf := make([]byte, len(floats)*4)
	for i, f := range floats {
		bits := math.Float32bits(f)
		buf[i*4] = byte(bits)
		buf[i*4+1] = byte(bits >> 8)
		buf[i*4+2] = byte(bits >> 16)
		buf[i*4+3] = byte(bits >> 24)
	}
	return buf
}

func blobToFloat32s(buf []byte) []float32 {
	n := len(buf) / 4
	floats := make([]float32, n)
	for i := 0; i < n; i++ {
		bits := uint32(buf[i*4]) | uint32(buf[i*4+1])<<8 | uint32(buf[i*4+2])<<16 | uint32(buf[i*4+3])<<24
		floats[i] = math.Float32frombits(bits)
	}
	return floats
}

// --- User Methods ---

func (s *SQLiteStore) CreateUser(ctx context.Context, user *model.User) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, email, api_key, name) VALUES (?, ?, ?, ?)`,
		user.ID.String(), user.Email, user.APIKey, user.Name,
	)
	return err
}

func (s *SQLiteStore) GetUserByAPIKey(ctx context.Context, apiKey string) (*model.User, error) {
	var u model.User
	var id, createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, api_key, name, created_at FROM users WHERE api_key = ?`, apiKey,
	).Scan(&id, &u.Email, &u.APIKey, &u.Name, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.ID, _ = uuid.Parse(id)
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return &u, nil
}

func (s *SQLiteStore) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	var u model.User
	var id, createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, api_key, name, created_at FROM users WHERE email = ?`, email,
	).Scan(&id, &u.Email, &u.APIKey, &u.Name, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.ID, _ = uuid.Parse(id)
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	return &u, nil
}

// --- Tag Methods ---

func (s *SQLiteStore) CreateTag(ctx context.Context, tag *model.Tag) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tags (id, name, color) VALUES (?, ?, ?)`,
		tag.ID.String(), tag.Name, tag.Color,
	)
	return err
}

func (s *SQLiteStore) ListTags(ctx context.Context) ([]model.Tag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, color, created_at FROM tags ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []model.Tag
	for rows.Next() {
		var t model.Tag
		var id, createdAt string
		if err := rows.Scan(&id, &t.Name, &t.Color, &createdAt); err != nil {
			return nil, err
		}
		t.ID, _ = uuid.Parse(id)
		t.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func (s *SQLiteStore) DeleteTag(ctx context.Context, id uuid.UUID) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, id.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("tag not found")
	}
	return nil
}

func (s *SQLiteStore) AddDocumentTag(ctx context.Context, documentID, tagID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO document_tags (document_id, tag_id) VALUES (?, ?)`,
		documentID.String(), tagID.String(),
	)
	return err
}

func (s *SQLiteStore) RemoveDocumentTag(ctx context.Context, documentID, tagID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM document_tags WHERE document_id = ? AND tag_id = ?`,
		documentID.String(), tagID.String(),
	)
	return err
}

func (s *SQLiteStore) GetDocumentTags(ctx context.Context, documentID uuid.UUID) ([]model.Tag, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.name, t.color, t.created_at
		 FROM tags t
		 JOIN document_tags dt ON dt.tag_id = t.id
		 WHERE dt.document_id = ?
		 ORDER BY t.name`,
		documentID.String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []model.Tag
	for rows.Next() {
		var t model.Tag
		var id, createdAt string
		if err := rows.Scan(&id, &t.Name, &t.Color, &createdAt); err != nil {
			return nil, err
		}
		t.ID, _ = uuid.Parse(id)
		t.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		tags = append(tags, t)
	}
	return tags, rows.Err()
}
