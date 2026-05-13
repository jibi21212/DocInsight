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

	if err := s.InsertDocument(ctx, doc); err != nil {
		t.Fatalf("InsertDocument failed: %v", err)
	}

	fetched, err := s.GetDocument(ctx, doc.ID)
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

	doc, err := s.GetDocument(ctx, uuid.New())
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
		if err := s.InsertDocument(ctx, doc); err != nil {
			t.Fatalf("InsertDocument %d failed: %v", i, err)
		}
	}

	docs, total, err := s.ListDocuments(ctx, 1, 20, nil)
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
		s.InsertDocument(ctx, doc)
	}
	doc := &model.Document{
		ID: uuid.New(), Name: "completed.pdf", FilePath: "/tmp/c.pdf", FileSize: 100, Status: model.StatusCompleted,
	}
	s.InsertDocument(ctx, doc)

	status := string(model.StatusPending)
	docs, total, err := s.ListDocuments(ctx, 1, 20, &status)
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
		s.InsertDocument(ctx, doc)
	}

	docs, total, err := s.ListDocuments(ctx, 1, 2, nil)
	if err != nil {
		t.Fatalf("ListDocuments failed: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(docs) != 2 {
		t.Errorf("page 1: len(docs) = %d, want 2", len(docs))
	}

	docs, _, _ = s.ListDocuments(ctx, 3, 2, nil)
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
	s.InsertDocument(ctx, doc)

	errMsg := "something went wrong"
	if err := s.UpdateDocumentStatus(ctx, doc.ID, model.StatusFailed, &errMsg); err != nil {
		t.Fatalf("UpdateDocumentStatus failed: %v", err)
	}

	fetched, _ := s.GetDocument(ctx, doc.ID)
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
	s.InsertDocument(ctx, doc)

	if err := s.UpdateDocumentPageCount(ctx, doc.ID, 42); err != nil {
		t.Fatalf("UpdateDocumentPageCount failed: %v", err)
	}

	fetched, _ := s.GetDocument(ctx, doc.ID)
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
	s.InsertDocument(ctx, doc)

	filePath, err := s.DeleteDocument(ctx, doc.ID)
	if err != nil {
		t.Fatalf("DeleteDocument failed: %v", err)
	}
	if filePath != "/tmp/test.pdf" {
		t.Errorf("filePath = %q, want %q", filePath, "/tmp/test.pdf")
	}

	// Should be gone
	fetched, _ := s.GetDocument(ctx, doc.ID)
	if fetched != nil {
		t.Error("expected nil after deletion")
	}
}

func TestDeleteDocument_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.DeleteDocument(ctx, uuid.New())
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
	s.InsertDocument(ctx, doc)

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
	s.InsertDocument(ctx, doc)

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
	s.InsertDocument(ctx, doc)

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
	results, err := s.MatchEmbeddings(ctx, queryEmb, 0.0, 10, nil)
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
	s.InsertDocument(ctx, doc)

	chunkID := uuid.New()
	chunks := []model.Chunk{
		{ID: chunkID, DocumentID: docID, Content: "test content", PageNumber: 1, ChunkIndex: 0},
	}
	s.InsertChunks(ctx, chunks)

	emb := []float32{1.0, 0.0, 0.0, 0.0}
	s.InsertEmbeddings(ctx, []uuid.UUID{chunkID}, [][]float32{emb})

	// Query that's very different — should be below threshold
	queryEmb := []float32{0.0, 0.0, 0.0, 1.0}
	results, err := s.MatchEmbeddings(ctx, queryEmb, 0.9, 10, nil)
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
	s.InsertDocument(ctx, &model.Document{ID: doc1ID, Name: "doc1.pdf", FilePath: "/tmp/doc1.pdf", FileSize: 100, Status: model.StatusCompleted})
	s.InsertDocument(ctx, &model.Document{ID: doc2ID, Name: "doc2.pdf", FilePath: "/tmp/doc2.pdf", FileSize: 100, Status: model.StatusCompleted})

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
	results, err := s.MatchEmbeddings(ctx, emb, 0.0, 10, []uuid.UUID{doc1ID})
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
	s.InsertDocument(ctx, &model.Document{ID: processingID, Name: "p.pdf", FilePath: "/tmp/p.pdf", FileSize: 100, Status: model.StatusProcessing})
	s.InsertDocument(ctx, &model.Document{ID: uuid.New(), Name: "q.pdf", FilePath: "/tmp/q.pdf", FileSize: 100, Status: model.StatusPending})

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
	s.InsertDocument(ctx, &model.Document{ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusPending})

	chunkID := uuid.New()
	s.InsertChunks(ctx, []model.Chunk{
		{ID: chunkID, DocumentID: docID, Content: "test", PageNumber: 1, ChunkIndex: 0},
	})
	s.InsertEmbeddings(ctx, []uuid.UUID{chunkID}, [][]float32{{1.0, 0.0}})

	// Delete document — should cascade to chunks and embeddings
	s.DeleteDocument(ctx, docID)

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
	})

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
	})

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
	})

	tag := &model.Tag{ID: uuid.New(), Name: "cascade", Color: "#f00"}
	s.CreateTag(ctx, tag)
	s.AddDocumentTag(ctx, docID, tag.ID)

	// Delete document — should cascade to document_tags
	s.DeleteDocument(ctx, docID)

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
	})

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
	})

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
	})

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
	})

	chunks := []model.Chunk{
		{ID: uuid.New(), DocumentID: docID, Content: "photosynthesis in plants converts sunlight into energy", PageNumber: 1, ChunkIndex: 0},
		{ID: uuid.New(), DocumentID: docID, Content: "the mitochondria is the powerhouse of the cell", PageNumber: 2, ChunkIndex: 1},
	}
	s.InsertChunks(ctx, chunks)

	results, err := s.KeywordSearch(ctx, "photosynthesis", 10, nil)
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
	s.InsertDocument(ctx, &model.Document{ID: doc1ID, Name: "a.pdf", FilePath: "/tmp/a.pdf", FileSize: 100, Status: model.StatusCompleted})
	s.InsertDocument(ctx, &model.Document{ID: doc2ID, Name: "b.pdf", FilePath: "/tmp/b.pdf", FileSize: 100, Status: model.StatusCompleted})

	s.InsertChunks(ctx, []model.Chunk{
		{ID: uuid.New(), DocumentID: doc1ID, Content: "algorithm complexity analysis", PageNumber: 1, ChunkIndex: 0},
	})
	s.InsertChunks(ctx, []model.Chunk{
		{ID: uuid.New(), DocumentID: doc2ID, Content: "algorithm design patterns", PageNumber: 1, ChunkIndex: 0},
	})

	results, err := s.KeywordSearch(ctx, "algorithm", 10, []uuid.UUID{doc1ID})
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
	})

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

	results, err := s.HybridSearch(ctx, queryEmb, "quantum", 0.0, 10, nil)
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
	})

	chunkID := uuid.New()
	s.InsertChunks(ctx, []model.Chunk{
		{ID: chunkID, DocumentID: docID, Content: "bananarama tropical fruit smoothie", PageNumber: 1, ChunkIndex: 0},
	})

	// Embedding is orthogonal to query — high threshold means no semantic match
	s.InsertEmbeddings(ctx, []uuid.UUID{chunkID}, [][]float32{{0.0, 0.0, 0.0, 1.0}})

	queryEmb := []float32{1.0, 0.0, 0.0, 0.0}
	results, err := s.HybridSearch(ctx, queryEmb, "bananarama", 0.9, 10, nil)
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
	})

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
