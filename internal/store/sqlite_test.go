package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/docinsight/backend/internal/model"
	"github.com/google/uuid"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNewSQLiteStore(t *testing.T) {
	s := newTestStore(t)
	if s == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestInsertAndGetDocument(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	doc := &model.Document{
		ID:       uuid.New(),
		Name:     "test.pdf",
		FilePath: "/tmp/test.pdf",
		FileSize: 1024,
		Status:   model.StatusPending,
	}

	if err := s.InsertDocument(ctx, doc, nil); err != nil {
		t.Fatalf("InsertDocument failed: %v", err)
	}

	fetched, err := s.GetDocument(ctx, doc.ID, nil)
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}
	if fetched == nil {
		t.Fatal("expected non-nil document")
	}
	if fetched.Name != "test.pdf" {
		t.Errorf("Name = %q, want %q", fetched.Name, "test.pdf")
	}
	if fetched.Status != model.StatusPending {
		t.Errorf("Status = %q, want %q", fetched.Status, model.StatusPending)
	}
	if fetched.FileSize != 1024 {
		t.Errorf("FileSize = %d, want 1024", fetched.FileSize)
	}
}

func TestGetDocument_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	doc, err := s.GetDocument(ctx, uuid.New(), nil)
	if err != nil {
		t.Fatalf("GetDocument should not error for missing doc: %v", err)
	}
	if doc != nil {
		t.Error("expected nil for non-existent document")
	}
}

func TestListDocuments(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert 3 documents
	for i := 0; i < 3; i++ {
		doc := &model.Document{
			ID:       uuid.New(),
			Name:     "doc.pdf",
			FilePath: "/tmp/doc.pdf",
			FileSize: 100,
			Status:   model.StatusPending,
		}
		if err := s.InsertDocument(ctx, doc, nil); err != nil {
			t.Fatalf("InsertDocument %d failed: %v", i, err)
		}
	}

	docs, total, err := s.ListDocuments(ctx, 1, 20, nil, nil, nil)
	if err != nil {
		t.Fatalf("ListDocuments failed: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(docs) != 3 {
		t.Errorf("len(docs) = %d, want 3", len(docs))
	}
}

func TestListDocuments_WithStatusFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert 2 pending, 1 completed
	for i := 0; i < 2; i++ {
		doc := &model.Document{
			ID: uuid.New(), Name: "pending.pdf", FilePath: "/tmp/p.pdf", FileSize: 100, Status: model.StatusPending,
		}
		s.InsertDocument(ctx, doc, nil)
	}
	doc := &model.Document{
		ID: uuid.New(), Name: "completed.pdf", FilePath: "/tmp/c.pdf", FileSize: 100, Status: model.StatusCompleted,
	}
	s.InsertDocument(ctx, doc, nil)

	status := string(model.StatusPending)
	docs, total, err := s.ListDocuments(ctx, 1, 20, &status, nil, nil)
	if err != nil {
		t.Fatalf("ListDocuments failed: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(docs) != 2 {
		t.Errorf("len(docs) = %d, want 2", len(docs))
	}
}

func TestListDocuments_Pagination(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		doc := &model.Document{
			ID: uuid.New(), Name: "doc.pdf", FilePath: "/tmp/doc.pdf", FileSize: 100, Status: model.StatusPending,
		}
		s.InsertDocument(ctx, doc, nil)
	}

	docs, total, err := s.ListDocuments(ctx, 1, 2, nil, nil, nil)
	if err != nil {
		t.Fatalf("ListDocuments failed: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(docs) != 2 {
		t.Errorf("page 1: len(docs) = %d, want 2", len(docs))
	}

	docs, _, _ = s.ListDocuments(ctx, 3, 2, nil, nil, nil)
	if len(docs) != 1 {
		t.Errorf("page 3: len(docs) = %d, want 1", len(docs))
	}
}

func TestUpdateDocumentStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	doc := &model.Document{
		ID: uuid.New(), Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusPending,
	}
	s.InsertDocument(ctx, doc, nil)

	errMsg := "something went wrong"
	if err := s.UpdateDocumentStatus(ctx, doc.ID, model.StatusFailed, &errMsg); err != nil {
		t.Fatalf("UpdateDocumentStatus failed: %v", err)
	}

	fetched, _ := s.GetDocument(ctx, doc.ID, nil)
	if fetched.Status != model.StatusFailed {
		t.Errorf("Status = %q, want %q", fetched.Status, model.StatusFailed)
	}
	if fetched.ErrorMessage == nil || *fetched.ErrorMessage != errMsg {
		t.Errorf("ErrorMessage = %v, want %q", fetched.ErrorMessage, errMsg)
	}
}

func TestUpdateDocumentPageCount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	doc := &model.Document{
		ID: uuid.New(), Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusPending,
	}
	s.InsertDocument(ctx, doc, nil)

	if err := s.UpdateDocumentPageCount(ctx, doc.ID, 42); err != nil {
		t.Fatalf("UpdateDocumentPageCount failed: %v", err)
	}

	fetched, _ := s.GetDocument(ctx, doc.ID, nil)
	if fetched.PageCount != 42 {
		t.Errorf("PageCount = %d, want 42", fetched.PageCount)
	}
}

func TestDeleteDocument(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	doc := &model.Document{
		ID: uuid.New(), Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusPending,
	}
	s.InsertDocument(ctx, doc, nil)

	filePath, err := s.DeleteDocument(ctx, doc.ID, nil)
	if err != nil {
		t.Fatalf("DeleteDocument failed: %v", err)
	}
	if filePath != "/tmp/test.pdf" {
		t.Errorf("filePath = %q, want %q", filePath, "/tmp/test.pdf")
	}

	// Should be gone
	fetched, _ := s.GetDocument(ctx, doc.ID, nil)
	if fetched != nil {
		t.Error("expected nil after deletion")
	}
}

func TestDeleteDocument_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.DeleteDocument(ctx, uuid.New(), nil)
	if err == nil {
		t.Error("expected error for non-existent document")
	}
}

func TestInsertAndGetChunks(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert document first
	docID := uuid.New()
	doc := &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusPending,
	}
	s.InsertDocument(ctx, doc, nil)

	chunks := []model.Chunk{
		{ID: uuid.New(), DocumentID: docID, Content: "chunk 1", PageNumber: 1, ChunkIndex: 0, Metadata: model.ChunkMetadata{CharCount: 7, WordCount: 2, StartPage: 1, EndPage: 1}},
		{ID: uuid.New(), DocumentID: docID, Content: "chunk 2", PageNumber: 1, ChunkIndex: 1, Metadata: model.ChunkMetadata{CharCount: 7, WordCount: 2, StartPage: 1, EndPage: 1}},
	}

	ids, err := s.InsertChunks(ctx, chunks)
	if err != nil {
		t.Fatalf("InsertChunks failed: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}

	fetched, err := s.GetChunksByDocumentID(ctx, docID)
	if err != nil {
		t.Fatalf("GetChunksByDocumentID failed: %v", err)
	}
	if len(fetched) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(fetched))
	}
	if fetched[0].Content != "chunk 1" {
		t.Errorf("first chunk content = %q, want %q", fetched[0].Content, "chunk 1")
	}
	if fetched[1].Content != "chunk 2" {
		t.Errorf("second chunk content = %q, want %q", fetched[1].Content, "chunk 2")
	}
}

func TestDeleteChunksByDocumentID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	docID := uuid.New()
	doc := &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusPending,
	}
	s.InsertDocument(ctx, doc, nil)

	chunks := []model.Chunk{
		{ID: uuid.New(), DocumentID: docID, Content: "chunk 1", PageNumber: 1, ChunkIndex: 0},
	}
	s.InsertChunks(ctx, chunks)

	if err := s.DeleteChunksByDocumentID(ctx, docID); err != nil {
		t.Fatalf("DeleteChunksByDocumentID failed: %v", err)
	}

	fetched, _ := s.GetChunksByDocumentID(ctx, docID)
	if len(fetched) != 0 {
		t.Errorf("expected 0 chunks after delete, got %d", len(fetched))
	}
}

func TestInsertAndSearchEmbeddings(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a completed document
	docID := uuid.New()
	doc := &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusCompleted,
	}
	s.InsertDocument(ctx, doc, nil)

	// Insert chunks
	chunk1ID := uuid.New()
	chunk2ID := uuid.New()
	chunks := []model.Chunk{
		{ID: chunk1ID, DocumentID: docID, Content: "about cats and dogs", PageNumber: 1, ChunkIndex: 0, Metadata: model.ChunkMetadata{CharCount: 19, WordCount: 4, StartPage: 1, EndPage: 1}},
		{ID: chunk2ID, DocumentID: docID, Content: "about math formulas", PageNumber: 2, ChunkIndex: 1, Metadata: model.ChunkMetadata{CharCount: 19, WordCount: 3, StartPage: 2, EndPage: 2}},
	}
	s.InsertChunks(ctx, chunks)

	// Create simple embeddings (4 dimensions for testing)
	emb1 := []float32{1.0, 0.0, 0.0, 0.0}
	emb2 := []float32{0.0, 1.0, 0.0, 0.0}

	if err := s.InsertEmbeddings(ctx, []uuid.UUID{chunk1ID, chunk2ID}, [][]float32{emb1, emb2}); err != nil {
		t.Fatalf("InsertEmbeddings failed: %v", err)
	}

	// Search with a query embedding similar to emb1
	queryEmb := []float32{0.9, 0.1, 0.0, 0.0}
	results, err := s.MatchEmbeddings(ctx, queryEmb, 0.0, 10, nil, nil, nil)
	if err != nil {
		t.Fatalf("MatchEmbeddings failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First result should be the one most similar to queryEmb (emb1)
	if results[0].ChunkID != chunk1ID {
		t.Errorf("most similar chunk should be chunk1, got %v", results[0].ChunkID)
	}

	if results[0].Similarity <= results[1].Similarity {
		t.Errorf("results should be ordered by similarity desc: %f <= %f", results[0].Similarity, results[1].Similarity)
	}
}

func TestMatchEmbeddings_WithThreshold(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	docID := uuid.New()
	doc := &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusCompleted,
	}
	s.InsertDocument(ctx, doc, nil)

	chunkID := uuid.New()
	chunks := []model.Chunk{
		{ID: chunkID, DocumentID: docID, Content: "test content", PageNumber: 1, ChunkIndex: 0},
	}
	s.InsertChunks(ctx, chunks)

	emb := []float32{1.0, 0.0, 0.0, 0.0}
	s.InsertEmbeddings(ctx, []uuid.UUID{chunkID}, [][]float32{emb})

	// Query that's very different — should be below threshold
	queryEmb := []float32{0.0, 0.0, 0.0, 1.0}
	results, err := s.MatchEmbeddings(ctx, queryEmb, 0.9, 10, nil, nil, nil)
	if err != nil {
		t.Fatalf("MatchEmbeddings failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results with high threshold, got %d", len(results))
	}
}

func TestMatchEmbeddings_FilterByDocument(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	doc1ID := uuid.New()
	doc2ID := uuid.New()
	s.InsertDocument(ctx, &model.Document{ID: doc1ID, Name: "doc1.pdf", FilePath: "/tmp/doc1.pdf", FileSize: 100, Status: model.StatusCompleted}, nil)
	s.InsertDocument(ctx, &model.Document{ID: doc2ID, Name: "doc2.pdf", FilePath: "/tmp/doc2.pdf", FileSize: 100, Status: model.StatusCompleted}, nil)

	chunk1ID := uuid.New()
	chunk2ID := uuid.New()
	s.InsertChunks(ctx, []model.Chunk{
		{ID: chunk1ID, DocumentID: doc1ID, Content: "doc1 content", PageNumber: 1, ChunkIndex: 0},
	})
	s.InsertChunks(ctx, []model.Chunk{
		{ID: chunk2ID, DocumentID: doc2ID, Content: "doc2 content", PageNumber: 1, ChunkIndex: 0},
	})

	emb := []float32{1.0, 0.0, 0.0, 0.0}
	s.InsertEmbeddings(ctx, []uuid.UUID{chunk1ID}, [][]float32{emb})
	s.InsertEmbeddings(ctx, []uuid.UUID{chunk2ID}, [][]float32{emb})

	// Search only doc1
	results, err := s.MatchEmbeddings(ctx, emb, 0.0, 10, []uuid.UUID{doc1ID}, nil, nil)
	if err != nil {
		t.Fatalf("MatchEmbeddings failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result filtered to doc1, got %d", len(results))
	}
	if results[0].DocumentID != doc1ID {
		t.Errorf("result doc ID = %v, want %v", results[0].DocumentID, doc1ID)
	}
}

func TestGetProcessingDocumentIDs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert one processing, one pending
	processingID := uuid.New()
	s.InsertDocument(ctx, &model.Document{ID: processingID, Name: "p.pdf", FilePath: "/tmp/p.pdf", FileSize: 100, Status: model.StatusProcessing}, nil)
	s.InsertDocument(ctx, &model.Document{ID: uuid.New(), Name: "q.pdf", FilePath: "/tmp/q.pdf", FileSize: 100, Status: model.StatusPending}, nil)

	ids, err := s.GetProcessingDocumentIDs(ctx)
	if err != nil {
		t.Fatalf("GetProcessingDocumentIDs failed: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 processing ID, got %d", len(ids))
	}
	if ids[0] != processingID {
		t.Errorf("processing ID = %v, want %v", ids[0], processingID)
	}
}

func TestCascadeDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	docID := uuid.New()
	s.InsertDocument(ctx, &model.Document{ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusPending}, nil)

	chunkID := uuid.New()
	s.InsertChunks(ctx, []model.Chunk{
		{ID: chunkID, DocumentID: docID, Content: "test", PageNumber: 1, ChunkIndex: 0},
	})
	s.InsertEmbeddings(ctx, []uuid.UUID{chunkID}, [][]float32{{1.0, 0.0}})

	// Delete document — should cascade to chunks and embeddings
	s.DeleteDocument(ctx, docID, nil)

	chunks, _ := s.GetChunksByDocumentID(ctx, docID)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks after cascade delete, got %d", len(chunks))
	}
}

func TestBlobConversion(t *testing.T) {
	original := []float32{1.0, -0.5, 0.0, 3.14159}
	blob := float32sToBlob(original)
	recovered := blobToFloat32s(blob)

	if len(recovered) != len(original) {
		t.Fatalf("length mismatch: %d vs %d", len(recovered), len(original))
	}
	for i := range original {
		if recovered[i] != original[i] {
			t.Errorf("recovered[%d] = %f, want %f", i, recovered[i], original[i])
		}
	}
}

func TestCosineSimilarity(t *testing.T) {
	// Identical vectors → similarity = 1.0
	a := []float32{1.0, 0.0, 0.0}
	sim := cosineSimilarity(a, a)
	if sim < 0.999 {
		t.Errorf("identical vectors similarity = %f, want ~1.0", sim)
	}

	// Orthogonal vectors → similarity = 0.0
	b := []float32{0.0, 1.0, 0.0}
	sim = cosineSimilarity(a, b)
	if sim > 0.001 {
		t.Errorf("orthogonal vectors similarity = %f, want ~0.0", sim)
	}

	// Opposite vectors → similarity = -1.0
	c := []float32{-1.0, 0.0, 0.0}
	sim = cosineSimilarity(a, c)
	if sim > -0.999 {
		t.Errorf("opposite vectors similarity = %f, want ~-1.0", sim)
	}

	// Different lengths → 0
	sim = cosineSimilarity([]float32{1.0}, []float32{1.0, 2.0})
	if sim != 0 {
		t.Errorf("different length similarity = %f, want 0", sim)
	}

	// Zero vector → 0
	sim = cosineSimilarity([]float32{0, 0, 0}, []float32{1, 0, 0})
	if sim != 0 {
		t.Errorf("zero vector similarity = %f, want 0", sim)
	}
}

func TestSQLiteStore_InvalidPath(t *testing.T) {
	_, err := NewSQLiteStore(filepath.Join(os.DevNull, "impossible", "path.db"))
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

// --- Tag Tests ---

func TestTags_CreateAndList(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tag := &model.Tag{ID: uuid.New(), Name: "important", Color: "#ef4444"}
	if err := s.CreateTag(ctx, tag); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	tags, err := s.ListTags(ctx)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	if tags[0].Name != "important" {
		t.Errorf("name = %q, want %q", tags[0].Name, "important")
	}
	if tags[0].Color != "#ef4444" {
		t.Errorf("color = %q, want %q", tags[0].Color, "#ef4444")
	}
}

func TestTags_CreateDuplicate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tag1 := &model.Tag{ID: uuid.New(), Name: "dup", Color: "#000"}
	s.CreateTag(ctx, tag1)
	tag2 := &model.Tag{ID: uuid.New(), Name: "dup", Color: "#fff"}
	err := s.CreateTag(ctx, tag2)
	if err == nil {
		t.Error("expected error for duplicate tag name")
	}
}

func TestTags_Delete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tag := &model.Tag{ID: uuid.New(), Name: "deleteme", Color: "#000"}
	s.CreateTag(ctx, tag)
	if err := s.DeleteTag(ctx, tag.ID); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	tags, _ := s.ListTags(ctx)
	if len(tags) != 0 {
		t.Errorf("expected 0 tags after delete, got %d", len(tags))
	}
}

func TestTags_DeleteNotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.DeleteTag(context.Background(), uuid.New())
	if err == nil {
		t.Error("expected error deleting non-existent tag")
	}
}

func TestDocumentTags_AddAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	docID := uuid.New()
	s.InsertDocument(ctx, &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusPending,
	}, nil)

	tag1 := &model.Tag{ID: uuid.New(), Name: "alpha", Color: "#f00"}
	tag2 := &model.Tag{ID: uuid.New(), Name: "beta", Color: "#0f0"}
	s.CreateTag(ctx, tag1)
	s.CreateTag(ctx, tag2)

	s.AddDocumentTag(ctx, docID, tag1.ID)
	s.AddDocumentTag(ctx, docID, tag2.ID)

	tags, err := s.GetDocumentTags(ctx, docID)
	if err != nil {
		t.Fatalf("GetDocumentTags: %v", err)
	}
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}
}

func TestDocumentTags_Remove(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	docID := uuid.New()
	s.InsertDocument(ctx, &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusPending,
	}, nil)

	tag := &model.Tag{ID: uuid.New(), Name: "remove-me", Color: "#f00"}
	s.CreateTag(ctx, tag)
	s.AddDocumentTag(ctx, docID, tag.ID)
	s.RemoveDocumentTag(ctx, docID, tag.ID)

	tags, _ := s.GetDocumentTags(ctx, docID)
	if len(tags) != 0 {
		t.Errorf("expected 0 tags after removal, got %d", len(tags))
	}
}

func TestDocumentTags_CascadeDeleteDocument(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	docID := uuid.New()
	s.InsertDocument(ctx, &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusPending,
	}, nil)

	tag := &model.Tag{ID: uuid.New(), Name: "cascade", Color: "#f00"}
	s.CreateTag(ctx, tag)
	s.AddDocumentTag(ctx, docID, tag.ID)

	// Delete document — should cascade to document_tags
	s.DeleteDocument(ctx, docID, nil)

	// Tag should still exist
	tags, _ := s.ListTags(ctx)
	if len(tags) != 1 {
		t.Errorf("tag should survive document deletion, got %d tags", len(tags))
	}
}

func TestDocumentTags_CascadeDeleteTag(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	docID := uuid.New()
	s.InsertDocument(ctx, &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusPending,
	}, nil)

	tag := &model.Tag{ID: uuid.New(), Name: "will-delete", Color: "#f00"}
	s.CreateTag(ctx, tag)
	s.AddDocumentTag(ctx, docID, tag.ID)

	// Delete tag — should cascade to document_tags
	s.DeleteTag(ctx, tag.ID)

	docTags, _ := s.GetDocumentTags(ctx, docID)
	if len(docTags) != 0 {
		t.Errorf("expected 0 document tags after tag deletion, got %d", len(docTags))
	}
}

// --- FTS5 / Hybrid Search Tests ---

func TestFTS5_InsertSyncsToFTS(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	docID := uuid.New()
	s.InsertDocument(ctx, &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusCompleted,
	}, nil)

	chunks := []model.Chunk{
		{ID: uuid.New(), DocumentID: docID, Content: "quantum physics and relativity", PageNumber: 1, ChunkIndex: 0},
		{ID: uuid.New(), DocumentID: docID, Content: "classical mechanics newton", PageNumber: 1, ChunkIndex: 1},
	}
	s.InsertChunks(ctx, chunks)

	// FTS should find "quantum"
	var count int
	s.db.QueryRow(`SELECT count(*) FROM chunks_fts WHERE chunks_fts MATCH 'quantum'`).Scan(&count)
	if count != 1 {
		t.Errorf("FTS match for 'quantum': got %d, want 1", count)
	}
}

func TestFTS5_DeleteCleansUpFTS(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	docID := uuid.New()
	s.InsertDocument(ctx, &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusCompleted,
	}, nil)

	chunks := []model.Chunk{
		{ID: uuid.New(), DocumentID: docID, Content: "unique searchable content xyzzy", PageNumber: 1, ChunkIndex: 0},
	}
	s.InsertChunks(ctx, chunks)

	// Verify it's in FTS
	var before int
	s.db.QueryRow(`SELECT count(*) FROM chunks_fts WHERE chunks_fts MATCH 'xyzzy'`).Scan(&before)
	if before != 1 {
		t.Fatalf("expected 1 FTS row before delete, got %d", before)
	}

	// Delete chunks
	s.DeleteChunksByDocumentID(ctx, docID)

	var after int
	s.db.QueryRow(`SELECT count(*) FROM chunks_fts WHERE chunks_fts MATCH 'xyzzy'`).Scan(&after)
	if after != 0 {
		t.Errorf("expected 0 FTS rows after delete, got %d", after)
	}
}

func TestKeywordSearch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	docID := uuid.New()
	s.InsertDocument(ctx, &model.Document{
		ID: docID, Name: "science.pdf", FilePath: "/tmp/s.pdf", FileSize: 100, Status: model.StatusCompleted,
	}, nil)

	chunks := []model.Chunk{
		{ID: uuid.New(), DocumentID: docID, Content: "photosynthesis in plants converts sunlight into energy", PageNumber: 1, ChunkIndex: 0},
		{ID: uuid.New(), DocumentID: docID, Content: "the mitochondria is the powerhouse of the cell", PageNumber: 2, ChunkIndex: 1},
	}
	s.InsertChunks(ctx, chunks)

	results, err := s.KeywordSearch(ctx, "photosynthesis", 10, nil, nil, nil)
	if err != nil {
		t.Fatalf("KeywordSearch: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'photosynthesis', got %d", len(results))
	}
	if results[0].MatchType != "keyword" {
		t.Errorf("match_type = %q, want 'keyword'", results[0].MatchType)
	}
	if results[0].KeywordScore <= 0 {
		t.Errorf("keyword_score should be > 0, got %f", results[0].KeywordScore)
	}
	if results[0].DocumentName != "science.pdf" {
		t.Errorf("document_name = %q, want 'science.pdf'", results[0].DocumentName)
	}
}

func TestKeywordSearch_FilterByDocument(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	doc1ID := uuid.New()
	doc2ID := uuid.New()
	s.InsertDocument(ctx, &model.Document{ID: doc1ID, Name: "a.pdf", FilePath: "/tmp/a.pdf", FileSize: 100, Status: model.StatusCompleted}, nil)
	s.InsertDocument(ctx, &model.Document{ID: doc2ID, Name: "b.pdf", FilePath: "/tmp/b.pdf", FileSize: 100, Status: model.StatusCompleted}, nil)

	s.InsertChunks(ctx, []model.Chunk{
		{ID: uuid.New(), DocumentID: doc1ID, Content: "algorithm complexity analysis", PageNumber: 1, ChunkIndex: 0},
	})
	s.InsertChunks(ctx, []model.Chunk{
		{ID: uuid.New(), DocumentID: doc2ID, Content: "algorithm design patterns", PageNumber: 1, ChunkIndex: 0},
	})

	results, err := s.KeywordSearch(ctx, "algorithm", 10, []uuid.UUID{doc1ID}, nil, nil)
	if err != nil {
		t.Fatalf("KeywordSearch: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result filtered to doc1, got %d", len(results))
	}
	if results[0].DocumentID != doc1ID {
		t.Errorf("result doc ID = %v, want %v", results[0].DocumentID, doc1ID)
	}
}

func TestHybridSearch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	docID := uuid.New()
	s.InsertDocument(ctx, &model.Document{
		ID: docID, Name: "physics.pdf", FilePath: "/tmp/p.pdf", FileSize: 100, Status: model.StatusCompleted,
	}, nil)

	chunk1ID := uuid.New()
	chunk2ID := uuid.New()
	chunk3ID := uuid.New()
	chunks := []model.Chunk{
		{ID: chunk1ID, DocumentID: docID, Content: "quantum entanglement and spooky action", PageNumber: 1, ChunkIndex: 0},
		{ID: chunk2ID, DocumentID: docID, Content: "classical newtonian mechanics and gravity", PageNumber: 2, ChunkIndex: 1},
		{ID: chunk3ID, DocumentID: docID, Content: "thermodynamics and entropy laws", PageNumber: 3, ChunkIndex: 2},
	}
	s.InsertChunks(ctx, chunks)

	// Embeddings: chunk1 similar to query, chunk2 somewhat similar, chunk3 orthogonal
	emb1 := []float32{1.0, 0.0, 0.0, 0.0}
	emb2 := []float32{0.7, 0.7, 0.0, 0.0}
	emb3 := []float32{0.0, 0.0, 0.0, 1.0}
	s.InsertEmbeddings(ctx, []uuid.UUID{chunk1ID, chunk2ID, chunk3ID}, [][]float32{emb1, emb2, emb3})

	queryEmb := []float32{0.9, 0.1, 0.0, 0.0}

	results, err := s.HybridSearch(ctx, queryEmb, "quantum", 0.0, 10, nil, nil, nil)
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least 1 result from hybrid search")
	}

	// chunk1 should rank highest — it matches both semantically and by keyword
	if results[0].ChunkID != chunk1ID {
		t.Errorf("top result should be chunk1 (matches both), got chunk %v", results[0].ChunkID)
	}
	if results[0].MatchType != "hybrid" {
		t.Errorf("top result match_type = %q, want 'hybrid'", results[0].MatchType)
	}
}

func TestHybridSearch_KeywordOnlyResult(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	docID := uuid.New()
	s.InsertDocument(ctx, &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/t.pdf", FileSize: 100, Status: model.StatusCompleted,
	}, nil)

	chunkID := uuid.New()
	s.InsertChunks(ctx, []model.Chunk{
		{ID: chunkID, DocumentID: docID, Content: "bananarama tropical fruit smoothie", PageNumber: 1, ChunkIndex: 0},
	})

	// Embedding is orthogonal to query — high threshold means no semantic match
	s.InsertEmbeddings(ctx, []uuid.UUID{chunkID}, [][]float32{{0.0, 0.0, 0.0, 1.0}})

	queryEmb := []float32{1.0, 0.0, 0.0, 0.0}
	results, err := s.HybridSearch(ctx, queryEmb, "bananarama", 0.9, 10, nil, nil, nil)
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected keyword-only result")
	}
	// Should be keyword match since semantic similarity is below threshold
	if results[0].MatchType != "keyword" {
		t.Errorf("match_type = %q, want 'keyword'", results[0].MatchType)
	}
}

// --- User Tests ---

func TestUser_CreateAndGetByAPIKey(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := &model.User{
		ID:     uuid.New(),
		Email:  "test@example.com",
		APIKey: "di_testkey123",
		Name:   "Test User",
	}
	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	fetched, err := s.GetUserByAPIKey(ctx, "di_testkey123")
	if err != nil {
		t.Fatalf("GetUserByAPIKey: %v", err)
	}
	if fetched == nil {
		t.Fatal("expected non-nil user")
	}
	if fetched.Email != "test@example.com" {
		t.Errorf("email = %q, want 'test@example.com'", fetched.Email)
	}
	if fetched.Name != "Test User" {
		t.Errorf("name = %q, want 'Test User'", fetched.Name)
	}
}

func TestUser_GetByAPIKey_NotFound(t *testing.T) {
	s := newTestStore(t)
	user, err := s.GetUserByAPIKey(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetUserByAPIKey: %v", err)
	}
	if user != nil {
		t.Error("expected nil for nonexistent key")
	}
}

func TestUser_GetByEmail(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := &model.User{
		ID:     uuid.New(),
		Email:  "lookup@example.com",
		APIKey: "di_lookupkey",
		Name:   "Lookup User",
	}
	s.CreateUser(ctx, user)

	fetched, err := s.GetUserByEmail(ctx, "lookup@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if fetched == nil {
		t.Fatal("expected non-nil user")
	}
	if fetched.ID != user.ID {
		t.Errorf("ID mismatch: %v != %v", fetched.ID, user.ID)
	}
}

func TestUser_GetByEmail_NotFound(t *testing.T) {
	s := newTestStore(t)
	user, err := s.GetUserByEmail(context.Background(), "nobody@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if user != nil {
		t.Error("expected nil for nonexistent email")
	}
}

func TestUser_DuplicateEmail(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user1 := &model.User{ID: uuid.New(), Email: "dup@example.com", APIKey: "di_key1", Name: "User 1"}
	s.CreateUser(ctx, user1)

	user2 := &model.User{ID: uuid.New(), Email: "dup@example.com", APIKey: "di_key2", Name: "User 2"}
	err := s.CreateUser(ctx, user2)
	if err == nil {
		t.Error("expected error for duplicate email")
	}
}

func TestDocumentTags_AddDuplicate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	docID := uuid.New()
	s.InsertDocument(ctx, &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusPending,
	}, nil)

	tag := &model.Tag{ID: uuid.New(), Name: "dup-link", Color: "#f00"}
	s.CreateTag(ctx, tag)
	s.AddDocumentTag(ctx, docID, tag.ID)

	// Adding the same tag again should not error (INSERT OR IGNORE)
	err := s.AddDocumentTag(ctx, docID, tag.ID)
	if err != nil {
		t.Errorf("duplicate AddDocumentTag should not error: %v", err)
	}

	tags, _ := s.GetDocumentTags(ctx, docID)
	if len(tags) != 1 {
		t.Errorf("expected 1 tag (no duplicate), got %d", len(tags))
	}
}

// --- User Scoping Tests ---

func TestListDocuments_FilterByUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userA := uuid.New()
	userB := uuid.New()
	if err := s.CreateUser(ctx, &model.User{ID: userA, Email: "a@example.com", APIKey: "di_a", Name: "A"}); err != nil {
		t.Fatalf("CreateUser A: %v", err)
	}
	if err := s.CreateUser(ctx, &model.User{ID: userB, Email: "b@example.com", APIKey: "di_b", Name: "B"}); err != nil {
		t.Fatalf("CreateUser B: %v", err)
	}

	// 2 docs for userA
	for i := 0; i < 2; i++ {
		doc := &model.Document{
			ID: uuid.New(), Name: "a.pdf", FilePath: "/tmp/a.pdf", FileSize: 100, Status: model.StatusPending,
		}
		if err := s.InsertDocument(ctx, doc, &userA); err != nil {
			t.Fatalf("InsertDocument(userA) failed: %v", err)
		}
	}
	// 1 doc for userB
	docB := &model.Document{
		ID: uuid.New(), Name: "b.pdf", FilePath: "/tmp/b.pdf", FileSize: 100, Status: model.StatusPending,
	}
	if err := s.InsertDocument(ctx, docB, &userB); err != nil {
		t.Fatalf("InsertDocument(userB) failed: %v", err)
	}

	// userA sees 2
	docs, total, err := s.ListDocuments(ctx, 1, 20, nil, &userA, nil)
	if err != nil {
		t.Fatalf("ListDocuments(userA) failed: %v", err)
	}
	if total != 2 || len(docs) != 2 {
		t.Errorf("userA: total=%d len=%d, want 2/2", total, len(docs))
	}

	// userB sees 1
	docs, total, err = s.ListDocuments(ctx, 1, 20, nil, &userB, nil)
	if err != nil {
		t.Fatalf("ListDocuments(userB) failed: %v", err)
	}
	if total != 1 || len(docs) != 1 {
		t.Errorf("userB: total=%d len=%d, want 1/1", total, len(docs))
	}

	// nil userID (auth disabled) sees all 3
	docs, total, err = s.ListDocuments(ctx, 1, 20, nil, nil, nil)
	if err != nil {
		t.Fatalf("ListDocuments(nil) failed: %v", err)
	}
	if total != 3 || len(docs) != 3 {
		t.Errorf("nil userID: total=%d len=%d, want 3/3", total, len(docs))
	}
}

func TestGetDocument_OtherUser_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userA := uuid.New()
	userB := uuid.New()
	s.CreateUser(ctx, &model.User{ID: userA, Email: "a2@example.com", APIKey: "di_a2", Name: "A"})
	s.CreateUser(ctx, &model.User{ID: userB, Email: "b2@example.com", APIKey: "di_b2", Name: "B"})

	doc := &model.Document{
		ID: uuid.New(), Name: "a.pdf", FilePath: "/tmp/a.pdf", FileSize: 100, Status: model.StatusPending,
	}
	if err := s.InsertDocument(ctx, doc, &userA); err != nil {
		t.Fatalf("InsertDocument: %v", err)
	}

	// userB cannot see userA's doc
	got, err := s.GetDocument(ctx, doc.ID, &userB)
	if err != nil {
		t.Fatalf("GetDocument should not error for invisible doc: %v", err)
	}
	if got != nil {
		t.Errorf("userB should not see userA's doc, got %+v", got)
	}

	// userA can see their own doc
	got, err = s.GetDocument(ctx, doc.ID, &userA)
	if err != nil {
		t.Fatalf("GetDocument(userA) failed: %v", err)
	}
	if got == nil {
		t.Fatal("userA should see their own doc")
	}

	// nil userID (auth disabled) can also see it
	got, err = s.GetDocument(ctx, doc.ID, nil)
	if err != nil {
		t.Fatalf("GetDocument(nil) failed: %v", err)
	}
	if got == nil {
		t.Fatal("nil userID should see the doc")
	}
}

func TestDeleteDocument_OtherUser_NoOp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userA := uuid.New()
	userB := uuid.New()
	s.CreateUser(ctx, &model.User{ID: userA, Email: "a3@example.com", APIKey: "di_a3", Name: "A"})
	s.CreateUser(ctx, &model.User{ID: userB, Email: "b3@example.com", APIKey: "di_b3", Name: "B"})

	doc := &model.Document{
		ID: uuid.New(), Name: "a.pdf", FilePath: "/tmp/a.pdf", FileSize: 100, Status: model.StatusPending,
	}
	if err := s.InsertDocument(ctx, doc, &userA); err != nil {
		t.Fatalf("InsertDocument: %v", err)
	}

	// userB attempts delete — must not remove the doc
	_, _ = s.DeleteDocument(ctx, doc.ID, &userB)

	// Verify the doc is still there for userA
	got, err := s.GetDocument(ctx, doc.ID, &userA)
	if err != nil {
		t.Fatalf("GetDocument after foreign delete: %v", err)
	}
	if got == nil {
		t.Fatal("userB's delete should be a no-op; doc still exists for owner")
	}

	// userA can delete their own doc
	_, err = s.DeleteDocument(ctx, doc.ID, &userA)
	if err != nil {
		t.Fatalf("DeleteDocument(userA) failed: %v", err)
	}

	got, _ = s.GetDocument(ctx, doc.ID, &userA)
	if got != nil {
		t.Error("doc should be gone after owner delete")
	}
}

func TestSearch_UserScoped(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userA := uuid.New()
	userB := uuid.New()
	s.CreateUser(ctx, &model.User{ID: userA, Email: "a4@example.com", APIKey: "di_a4", Name: "A"})
	s.CreateUser(ctx, &model.User{ID: userB, Email: "b4@example.com", APIKey: "di_b4", Name: "B"})

	// userA's doc + chunk
	docA := &model.Document{
		ID: uuid.New(), Name: "a.pdf", FilePath: "/tmp/a.pdf", FileSize: 100, Status: model.StatusCompleted,
	}
	s.InsertDocument(ctx, docA, &userA)
	chunkA := uuid.New()
	s.InsertChunks(ctx, []model.Chunk{
		{ID: chunkA, DocumentID: docA.ID, Content: "alpha shared keyword content", PageNumber: 1, ChunkIndex: 0},
	})
	s.InsertEmbeddings(ctx, []uuid.UUID{chunkA}, [][]float32{{1.0, 0.0, 0.0, 0.0}})

	// userB's doc + chunk
	docB := &model.Document{
		ID: uuid.New(), Name: "b.pdf", FilePath: "/tmp/b.pdf", FileSize: 100, Status: model.StatusCompleted,
	}
	s.InsertDocument(ctx, docB, &userB)
	chunkB := uuid.New()
	s.InsertChunks(ctx, []model.Chunk{
		{ID: chunkB, DocumentID: docB.ID, Content: "beta shared keyword content", PageNumber: 1, ChunkIndex: 0},
	})
	s.InsertEmbeddings(ctx, []uuid.UUID{chunkB}, [][]float32{{1.0, 0.0, 0.0, 0.0}})

	queryEmb := []float32{1.0, 0.0, 0.0, 0.0}

	// MatchEmbeddings: userA sees only their chunk
	res, err := s.MatchEmbeddings(ctx, queryEmb, 0.0, 10, nil, &userA, nil)
	if err != nil {
		t.Fatalf("MatchEmbeddings(userA): %v", err)
	}
	if len(res) != 1 || res[0].ChunkID != chunkA {
		t.Errorf("MatchEmbeddings(userA) = %d results, want 1 (chunkA)", len(res))
	}

	// KeywordSearch: userA sees only their chunk
	res, err = s.KeywordSearch(ctx, "shared", 10, nil, &userA, nil)
	if err != nil {
		t.Fatalf("KeywordSearch(userA): %v", err)
	}
	if len(res) != 1 || res[0].ChunkID != chunkA {
		t.Errorf("KeywordSearch(userA) = %d results, want 1 (chunkA)", len(res))
	}

	// HybridSearch: userB sees only their chunk
	res, err = s.HybridSearch(ctx, queryEmb, "shared", 0.0, 10, nil, &userB, nil)
	if err != nil {
		t.Fatalf("HybridSearch(userB): %v", err)
	}
	if len(res) != 1 || res[0].ChunkID != chunkB {
		t.Errorf("HybridSearch(userB) = %d results, want 1 (chunkB)", len(res))
	}

	// nil userID: sees both chunks across all three search methods
	res, err = s.MatchEmbeddings(ctx, queryEmb, 0.0, 10, nil, nil, nil)
	if err != nil {
		t.Fatalf("MatchEmbeddings(nil): %v", err)
	}
	if len(res) != 2 {
		t.Errorf("MatchEmbeddings(nil) = %d results, want 2", len(res))
	}
	res, err = s.KeywordSearch(ctx, "shared", 10, nil, nil, nil)
	if err != nil {
		t.Fatalf("KeywordSearch(nil): %v", err)
	}
	if len(res) != 2 {
		t.Errorf("KeywordSearch(nil) = %d results, want 2", len(res))
	}
	res, err = s.HybridSearch(ctx, queryEmb, "shared", 0.0, 10, nil, nil, nil)
	if err != nil {
		t.Fatalf("HybridSearch(nil): %v", err)
	}
	if len(res) != 2 {
		t.Errorf("HybridSearch(nil) = %d results, want 2", len(res))
	}
}

// --- Folder Tests ---

func TestCreateFolder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userA := uuid.New()
	s.CreateUser(ctx, &model.User{ID: userA, Email: "fa@example.com", APIKey: "di_fa", Name: "A"})

	f := &model.Folder{ID: uuid.New(), UserID: &userA, Name: "Research"}
	if err := s.CreateFolder(ctx, f); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}

	got, err := s.GetFolder(ctx, f.ID, &userA)
	if err != nil {
		t.Fatalf("GetFolder: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil folder")
	}
	if got.Name != "Research" {
		t.Errorf("name = %q, want 'Research'", got.Name)
	}
}

func TestCreateFolder_DuplicateName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userA := uuid.New()
	s.CreateUser(ctx, &model.User{ID: userA, Email: "fdup@example.com", APIKey: "di_fdup", Name: "A"})

	f1 := &model.Folder{ID: uuid.New(), UserID: &userA, Name: "Same"}
	if err := s.CreateFolder(ctx, f1); err != nil {
		t.Fatalf("CreateFolder f1: %v", err)
	}
	f2 := &model.Folder{ID: uuid.New(), UserID: &userA, Name: "Same"}
	if err := s.CreateFolder(ctx, f2); err == nil {
		t.Error("expected uniqueness violation for duplicate (user, parent, name)")
	}
}

func TestListFolders_Children(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userA := uuid.New()
	s.CreateUser(ctx, &model.User{ID: userA, Email: "fc@example.com", APIKey: "di_fc", Name: "A"})

	root := &model.Folder{ID: uuid.New(), UserID: &userA, Name: "root"}
	s.CreateFolder(ctx, root)
	child1 := &model.Folder{ID: uuid.New(), UserID: &userA, ParentID: &root.ID, Name: "c1"}
	child2 := &model.Folder{ID: uuid.New(), UserID: &userA, ParentID: &root.ID, Name: "c2"}
	s.CreateFolder(ctx, child1)
	s.CreateFolder(ctx, child2)
	grand := &model.Folder{ID: uuid.New(), UserID: &userA, ParentID: &child1.ID, Name: "g"}
	s.CreateFolder(ctx, grand)

	got, err := s.ListFolders(ctx, &userA, &root.ID)
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 direct children, got %d", len(got))
	}
}

func TestListFolders_Roots(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userA := uuid.New()
	s.CreateUser(ctx, &model.User{ID: userA, Email: "fr@example.com", APIKey: "di_fr", Name: "A"})

	r1 := &model.Folder{ID: uuid.New(), UserID: &userA, Name: "r1"}
	r2 := &model.Folder{ID: uuid.New(), UserID: &userA, Name: "r2"}
	s.CreateFolder(ctx, r1)
	s.CreateFolder(ctx, r2)
	child := &model.Folder{ID: uuid.New(), UserID: &userA, ParentID: &r1.ID, Name: "c"}
	s.CreateFolder(ctx, child)

	got, err := s.ListFolders(ctx, &userA, nil)
	if err != nil {
		t.Fatalf("ListFolders(roots): %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(got))
	}
}

func TestFolderDescendants_DeepNesting(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userA := uuid.New()
	s.CreateUser(ctx, &model.User{ID: userA, Email: "fd@example.com", APIKey: "di_fd", Name: "A"})

	root := &model.Folder{ID: uuid.New(), UserID: &userA, Name: "lvl0"}
	s.CreateFolder(ctx, root)
	lvl1 := &model.Folder{ID: uuid.New(), UserID: &userA, ParentID: &root.ID, Name: "lvl1"}
	s.CreateFolder(ctx, lvl1)
	lvl2 := &model.Folder{ID: uuid.New(), UserID: &userA, ParentID: &lvl1.ID, Name: "lvl2"}
	s.CreateFolder(ctx, lvl2)
	lvl3 := &model.Folder{ID: uuid.New(), UserID: &userA, ParentID: &lvl2.ID, Name: "lvl3"}
	s.CreateFolder(ctx, lvl3)

	ids, err := s.FolderDescendants(ctx, root.ID)
	if err != nil {
		t.Fatalf("FolderDescendants: %v", err)
	}
	if len(ids) != 4 {
		t.Fatalf("expected 4 descendants (self + 3), got %d", len(ids))
	}
	want := map[uuid.UUID]bool{root.ID: true, lvl1.ID: true, lvl2.ID: true, lvl3.ID: true}
	for _, id := range ids {
		if !want[id] {
			t.Errorf("unexpected id in descendants: %v", id)
		}
		delete(want, id)
	}
	if len(want) != 0 {
		t.Errorf("missing ids from descendants: %v", want)
	}
}

func TestDeleteFolder_CascadesToDescendants(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userA := uuid.New()
	s.CreateUser(ctx, &model.User{ID: userA, Email: "fdel@example.com", APIKey: "di_fdel", Name: "A"})

	root := &model.Folder{ID: uuid.New(), UserID: &userA, Name: "root"}
	s.CreateFolder(ctx, root)
	child := &model.Folder{ID: uuid.New(), UserID: &userA, ParentID: &root.ID, Name: "c"}
	s.CreateFolder(ctx, child)
	grand := &model.Folder{ID: uuid.New(), UserID: &userA, ParentID: &child.ID, Name: "g"}
	s.CreateFolder(ctx, grand)

	if err := s.DeleteFolder(ctx, root.ID, &userA); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}

	// All descendants should be gone
	for _, id := range []uuid.UUID{root.ID, child.ID, grand.ID} {
		got, _ := s.GetFolder(ctx, id, &userA)
		if got != nil {
			t.Errorf("folder %v should be deleted via cascade", id)
		}
	}
}

func TestDeleteFolder_DocumentsBecomeUnfiled(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userA := uuid.New()
	s.CreateUser(ctx, &model.User{ID: userA, Email: "fdoc@example.com", APIKey: "di_fdoc", Name: "A"})

	folder := &model.Folder{ID: uuid.New(), UserID: &userA, Name: "f1"}
	s.CreateFolder(ctx, folder)

	doc := &model.Document{
		ID: uuid.New(), Name: "d.pdf", FilePath: "/tmp/d.pdf", FileSize: 100, Status: model.StatusPending,
		FolderID: &folder.ID,
	}
	if err := s.InsertDocument(ctx, doc, &userA); err != nil {
		t.Fatalf("InsertDocument: %v", err)
	}

	if err := s.DeleteFolder(ctx, folder.ID, &userA); err != nil {
		t.Fatalf("DeleteFolder: %v", err)
	}

	got, err := s.GetDocument(ctx, doc.ID, &userA)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if got == nil {
		t.Fatal("document should not be deleted when its folder is deleted")
	}
	if got.FolderID != nil {
		t.Errorf("expected folder_id to be NULL after folder deletion, got %v", got.FolderID)
	}
}

func TestMoveDocument_HappyPath(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userA := uuid.New()
	s.CreateUser(ctx, &model.User{ID: userA, Email: "mvh@example.com", APIKey: "di_mvh", Name: "A"})

	folder := &model.Folder{ID: uuid.New(), UserID: &userA, Name: "moved"}
	s.CreateFolder(ctx, folder)

	doc := &model.Document{
		ID: uuid.New(), Name: "d.pdf", FilePath: "/tmp/d.pdf", FileSize: 100, Status: model.StatusPending,
	}
	if err := s.InsertDocument(ctx, doc, &userA); err != nil {
		t.Fatalf("InsertDocument: %v", err)
	}

	if err := s.MoveDocumentToFolder(ctx, doc.ID, &folder.ID, &userA); err != nil {
		t.Fatalf("MoveDocumentToFolder: %v", err)
	}

	got, _ := s.GetDocument(ctx, doc.ID, &userA)
	if got == nil || got.FolderID == nil || *got.FolderID != folder.ID {
		t.Errorf("folder_id not set correctly after move, got %+v", got)
	}

	// Move back to root (unfiled)
	if err := s.MoveDocumentToFolder(ctx, doc.ID, nil, &userA); err != nil {
		t.Fatalf("MoveDocumentToFolder(nil): %v", err)
	}
	got, _ = s.GetDocument(ctx, doc.ID, &userA)
	if got == nil || got.FolderID != nil {
		t.Errorf("folder_id should be nil after moving back to root, got %+v", got)
	}
}

func TestMoveDocument_CrossUser_Denied(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userA := uuid.New()
	userB := uuid.New()
	s.CreateUser(ctx, &model.User{ID: userA, Email: "mvA@example.com", APIKey: "di_mvA", Name: "A"})
	s.CreateUser(ctx, &model.User{ID: userB, Email: "mvB@example.com", APIKey: "di_mvB", Name: "B"})

	// A's folder
	folderA := &model.Folder{ID: uuid.New(), UserID: &userA, Name: "Aroot"}
	s.CreateFolder(ctx, folderA)

	// A's doc
	doc := &model.Document{
		ID: uuid.New(), Name: "d.pdf", FilePath: "/tmp/d.pdf", FileSize: 100, Status: model.StatusPending,
	}
	s.InsertDocument(ctx, doc, &userA)

	// userB cannot move userA's doc
	err := s.MoveDocumentToFolder(ctx, doc.ID, &folderA.ID, &userB)
	if err == nil {
		t.Error("expected error when userB attempts to move userA's doc")
	}

	got, _ := s.GetDocument(ctx, doc.ID, &userA)
	if got == nil || got.FolderID != nil {
		t.Errorf("doc should remain unfiled after denied move, got %+v", got)
	}
}

func TestListDocuments_FilterByFolder_IncludesDescendants(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userA := uuid.New()
	s.CreateUser(ctx, &model.User{ID: userA, Email: "lfd@example.com", APIKey: "di_lfd", Name: "A"})

	root := &model.Folder{ID: uuid.New(), UserID: &userA, Name: "root"}
	s.CreateFolder(ctx, root)
	child := &model.Folder{ID: uuid.New(), UserID: &userA, ParentID: &root.ID, Name: "child"}
	s.CreateFolder(ctx, child)

	doc1 := &model.Document{
		ID: uuid.New(), Name: "in-root.pdf", FilePath: "/tmp/r.pdf", FileSize: 100, Status: model.StatusPending,
		FolderID: &root.ID,
	}
	doc2 := &model.Document{
		ID: uuid.New(), Name: "in-child.pdf", FilePath: "/tmp/c.pdf", FileSize: 100, Status: model.StatusPending,
		FolderID: &child.ID,
	}
	doc3 := &model.Document{
		ID: uuid.New(), Name: "unfiled.pdf", FilePath: "/tmp/u.pdf", FileSize: 100, Status: model.StatusPending,
	}
	s.InsertDocument(ctx, doc1, &userA)
	s.InsertDocument(ctx, doc2, &userA)
	s.InsertDocument(ctx, doc3, &userA)

	docs, total, err := s.ListDocuments(ctx, 1, 20, nil, &userA, &root.ID)
	if err != nil {
		t.Fatalf("ListDocuments(folder=root): %v", err)
	}
	if total != 2 || len(docs) != 2 {
		t.Errorf("filter by root: total=%d len=%d, want 2/2", total, len(docs))
	}

	docs, total, err = s.ListDocuments(ctx, 1, 20, nil, &userA, &child.ID)
	if err != nil {
		t.Fatalf("ListDocuments(folder=child): %v", err)
	}
	if total != 1 || len(docs) != 1 {
		t.Errorf("filter by child: total=%d len=%d, want 1/1", total, len(docs))
	}
}

func TestHybridSearch_FolderScoped(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userA := uuid.New()
	s.CreateUser(ctx, &model.User{ID: userA, Email: "hsf@example.com", APIKey: "di_hsf", Name: "A"})

	target := &model.Folder{ID: uuid.New(), UserID: &userA, Name: "in-scope"}
	other := &model.Folder{ID: uuid.New(), UserID: &userA, Name: "out-of-scope"}
	s.CreateFolder(ctx, target)
	s.CreateFolder(ctx, other)

	docIn := &model.Document{
		ID: uuid.New(), Name: "in.pdf", FilePath: "/tmp/i.pdf", FileSize: 100, Status: model.StatusCompleted,
		FolderID: &target.ID,
	}
	docOut := &model.Document{
		ID: uuid.New(), Name: "out.pdf", FilePath: "/tmp/o.pdf", FileSize: 100, Status: model.StatusCompleted,
		FolderID: &other.ID,
	}
	s.InsertDocument(ctx, docIn, &userA)
	s.InsertDocument(ctx, docOut, &userA)

	chunkIn := uuid.New()
	chunkOut := uuid.New()
	s.InsertChunks(ctx, []model.Chunk{
		{ID: chunkIn, DocumentID: docIn.ID, Content: "alpha quantum keyword", PageNumber: 1, ChunkIndex: 0},
	})
	s.InsertChunks(ctx, []model.Chunk{
		{ID: chunkOut, DocumentID: docOut.ID, Content: "beta quantum keyword", PageNumber: 1, ChunkIndex: 0},
	})
	emb := []float32{1.0, 0.0, 0.0, 0.0}
	s.InsertEmbeddings(ctx, []uuid.UUID{chunkIn}, [][]float32{emb})
	s.InsertEmbeddings(ctx, []uuid.UUID{chunkOut}, [][]float32{emb})

	results, err := s.HybridSearch(ctx, emb, "quantum", 0.0, 10, nil, &userA, &target.ID)
	if err != nil {
		t.Fatalf("HybridSearch folder-scoped: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result scoped to target folder, got %d", len(results))
	}
	if results[0].ChunkID != chunkIn {
		t.Errorf("expected chunkIn in results, got %v", results[0].ChunkID)
	}
}

// --- GetChunkByID ---

func TestGetChunkByID_HappyPath(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	userID := uuid.New()
	if err := s.CreateUser(ctx, &model.User{ID: userID, Email: "gcid@example.com", APIKey: "di_gcid", Name: "u"}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	docID := uuid.New()
	if err := s.InsertDocument(ctx, &model.Document{ID: docID, Name: "d.pdf", FilePath: "/tmp/d.pdf", FileSize: 1, Status: model.StatusCompleted, UserID: &userID}, &userID); err != nil {
		t.Fatalf("InsertDocument: %v", err)
	}
	chunkID := uuid.New()
	if _, err := s.InsertChunks(ctx, []model.Chunk{{ID: chunkID, DocumentID: docID, Content: "hello", PageNumber: 1, ChunkIndex: 0}}); err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}

	c, err := s.GetChunkByID(ctx, chunkID, &userID)
	if err != nil {
		t.Fatalf("GetChunkByID: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil chunk")
	}
	if c.ID != chunkID {
		t.Errorf("ID = %v, want %v", c.ID, chunkID)
	}
	if c.Content != "hello" {
		t.Errorf("Content = %q, want %q", c.Content, "hello")
	}
	if c.DocumentID != docID {
		t.Errorf("DocumentID = %v, want %v", c.DocumentID, docID)
	}
}

func TestGetChunkByID_NotFound(t *testing.T) {
	s := newTestStore(t)
	c, err := s.GetChunkByID(context.Background(), uuid.New(), nil)
	if err != nil {
		t.Fatalf("GetChunkByID should not error for missing chunk: %v", err)
	}
	if c != nil {
		t.Errorf("expected nil for non-existent chunk")
	}
}

func TestGetChunkByID_WrongUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	owner := uuid.New()
	intruder := uuid.New()
	if err := s.CreateUser(ctx, &model.User{ID: owner, Email: "gcio@example.com", APIKey: "di_gcio", Name: "o"}); err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	if err := s.CreateUser(ctx, &model.User{ID: intruder, Email: "gcii@example.com", APIKey: "di_gcii", Name: "i"}); err != nil {
		t.Fatalf("CreateUser intruder: %v", err)
	}
	docID := uuid.New()
	if err := s.InsertDocument(ctx, &model.Document{ID: docID, Name: "d.pdf", FilePath: "/tmp/d.pdf", FileSize: 1, Status: model.StatusCompleted, UserID: &owner}, &owner); err != nil {
		t.Fatalf("InsertDocument: %v", err)
	}
	chunkID := uuid.New()
	if _, err := s.InsertChunks(ctx, []model.Chunk{{ID: chunkID, DocumentID: docID, Content: "secret", PageNumber: 1, ChunkIndex: 0}}); err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}

	c, err := s.GetChunkByID(ctx, chunkID, &intruder)
	if err != nil {
		t.Fatalf("GetChunkByID: %v", err)
	}
	if c != nil {
		t.Errorf("intruder should not see chunk; got %+v", c)
	}

	// Sanity: the owner still does see it.
	c, err = s.GetChunkByID(ctx, chunkID, &owner)
	if err != nil {
		t.Fatalf("GetChunkByID for owner: %v", err)
	}
	if c == nil {
		t.Fatal("owner should see their own chunk")
	}
}
