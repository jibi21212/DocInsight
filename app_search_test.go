package main

import (
	"context"
	"testing"

	"github.com/docinsight/backend/internal/model"
	"github.com/google/uuid"
)

// fakeEmbedder returns a preset vector for any input, letting the semantic search
// path run without the real Python sidecar.
type fakeEmbedder struct{ vec []float32 }

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = f.vec
	}
	return out, nil
}

func (f *fakeEmbedder) EmbedSingle(_ context.Context, _ string) ([]float32, error) {
	return f.vec, nil
}

// seedEmbeddedChunk inserts a completed document with one embedded chunk owned by
// the app's local user (so it's visible to the userID-scoped search).
func seedEmbeddedChunk(t *testing.T, a *App, content string, emb []float32) {
	t.Helper()
	ctx := context.Background()
	docID := uuid.New()
	if err := a.store.InsertDocument(ctx, &model.Document{
		ID: docID, Name: "doc.pdf", FilePath: "/tmp/doc.pdf", FileSize: 100, Status: model.StatusCompleted,
	}, a.userID); err != nil {
		t.Fatalf("InsertDocument: %v", err)
	}
	chunkID := uuid.New()
	if _, err := a.store.InsertChunks(ctx, []model.Chunk{
		{ID: chunkID, DocumentID: docID, Content: content, PageNumber: 1, ChunkIndex: 0},
	}); err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}
	if err := a.store.InsertEmbeddings(ctx, []uuid.UUID{chunkID}, [][]float32{emb}); err != nil {
		t.Fatalf("InsertEmbeddings: %v", err)
	}
}

// TestSearch_SemanticNeverEmpty proves the top-K fallback: when no chunk clears
// the similarity threshold, semantic-only still returns the closest chunks
// instead of coming back empty.
func TestSearch_SemanticNeverEmpty(t *testing.T) {
	a := newTestApp(t)
	fe := &fakeEmbedder{}
	a.emb = fe

	seedEmbeddedChunk(t, a, "alpha content", []float32{1, 0, 0, 0})
	seedEmbeddedChunk(t, a, "beta content", []float32{0, 1, 0, 0})

	// Query orthogonal to both chunks => cosine 0, below any positive threshold.
	fe.vec = []float32{0, 0, 1, 0}
	resp, err := a.Search("anything", 10, 0.5, "semantic", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("semantic returned no results; the top-K fallback did not engage")
	}
	for _, r := range resp.Results {
		if r.MatchType != "semantic" {
			t.Fatalf("expected match_type=semantic, got %q", r.MatchType)
		}
	}

	// Positive control: a query aligned with one chunk clears the threshold the
	// normal way (no fallback needed).
	fe.vec = []float32{1, 0, 0, 0}
	resp, err = a.Search("anything", 10, 0.5, "semantic", "")
	if err != nil {
		t.Fatalf("Search (aligned): %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("aligned semantic query returned nothing")
	}
}
