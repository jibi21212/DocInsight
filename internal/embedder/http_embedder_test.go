package embedder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbed_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embed" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		var req embedRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := embedResponse{
			Embeddings: make([][]float32, len(req.Texts)),
		}
		for i := range req.Texts {
			resp.Embeddings[i] = []float32{1.0, 0.0, 0.0, 0.0}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	emb := NewHTTPEmbedder(server.URL)
	results, err := emb.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(results))
	}
	if len(results[0]) != 4 {
		t.Errorf("expected 4 dimensions, got %d", len(results[0]))
	}
}

func TestEmbed_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	emb := NewHTTPEmbedder(server.URL)
	_, err := emb.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for server 500")
	}
}

func TestEmbed_Unreachable(t *testing.T) {
	emb := NewHTTPEmbedder("http://localhost:1") // port 1 — unreachable
	_, err := emb.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestEmbedSingle_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := embedResponse{
			Embeddings: [][]float32{{0.5, 0.5, 0.0, 0.0}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	emb := NewHTTPEmbedder(server.URL)
	result, err := emb.EmbedSingle(context.Background(), "test")
	if err != nil {
		t.Fatalf("EmbedSingle failed: %v", err)
	}
	if len(result) != 4 {
		t.Errorf("expected 4 dimensions, got %d", len(result))
	}
}

func TestEmbedSingle_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := embedResponse{Embeddings: [][]float32{}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	emb := NewHTTPEmbedder(server.URL)
	_, err := emb.EmbedSingle(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for empty embeddings")
	}
}

func TestHealthCheck_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	emb := NewHTTPEmbedder(server.URL)
	err := emb.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
}

func TestHealthCheck_Unhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	emb := NewHTTPEmbedder(server.URL)
	err := emb.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("expected error for unhealthy sidecar")
	}
}

func TestHealthCheck_Unreachable(t *testing.T) {
	emb := NewHTTPEmbedder("http://localhost:1")
	err := emb.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("expected error for unreachable sidecar")
	}
}
