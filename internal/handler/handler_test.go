package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/docinsight/backend/internal/config"
	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/pdf"
	"github.com/docinsight/backend/internal/queue"
	"github.com/docinsight/backend/internal/scraper"
	"github.com/docinsight/backend/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// newTestConfig returns a config suitable for testing.
func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Port:            "8080",
		CORSOrigin:      "http://localhost:3000",
		UploadDir:       filepath.Join(t.TempDir(), "uploads"),
		MaxUploadSizeMB: 10,
		WorkerCount:     2,
		QueueCapacity:   10,
		MaxRetries:      3,
		ChunkSize:       1000,
		ChunkOverlap:    200,
		SearchTopK:      10,
		SimilarityThreshold: 0.5,
	}
}

// newTestStore creates a temporary SQLite store for testing.
func newTestStore(t *testing.T) store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// --- Document Handler Tests ---

func TestList_Empty(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	h := NewDocumentHandler(s, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/documents", nil)
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp model.PaginatedResponse[model.Document]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Total)
	}
	if len(resp.Data) != 0 {
		t.Errorf("data length = %d, want 0", len(resp.Data))
	}
}

func TestList_WithDocuments(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	h := NewDocumentHandler(s, cfg)

	// Insert a document
	doc := &model.Document{
		ID: uuid.New(), Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusPending,
	}
	s.InsertDocument(context.Background(), doc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/documents", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp model.PaginatedResponse[model.Document]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Total)
	}
}

func TestGetByID_Found(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	h := NewDocumentHandler(s, cfg)

	docID := uuid.New()
	doc := &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusPending,
	}
	s.InsertDocument(context.Background(), doc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/documents/"+docID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", docID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.GetByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	if result["document"] == nil {
		t.Error("expected document in response")
	}
}

func TestGetByID_NotFound(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	h := NewDocumentHandler(s, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/documents/"+uuid.New().String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.GetByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetByID_InvalidID(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	h := NewDocumentHandler(s, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/documents/invalid", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "not-a-uuid")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.GetByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	h := NewDocumentHandler(s, cfg)

	docID := uuid.New()
	// Create a temporary file
	tmpFile := filepath.Join(t.TempDir(), docID.String()+".pdf")
	os.WriteFile(tmpFile, []byte("pdf"), 0o644)

	doc := &model.Document{
		ID: docID, Name: "test.pdf", FilePath: tmpFile, FileSize: 3, Status: model.StatusPending,
	}
	s.InsertDocument(context.Background(), doc, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/documents/"+docID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", docID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	// Verify file deleted
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error("expected file to be deleted")
	}
}

func TestUpload_Success(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	h := NewDocumentHandler(s, cfg)

	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.pdf")
	part.Write([]byte("%PDF-1.4 test content"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/documents/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	w := httptest.NewRecorder()
	h.Upload(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d\nbody: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var result map[string]interface{}
	json.NewDecoder(w.Body).Decode(&result)
	if result["document"] == nil {
		t.Error("expected document in response")
	}
	if result["message"] == nil {
		t.Error("expected message in response")
	}
}

func TestUpload_NoFile(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	h := NewDocumentHandler(s, cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/documents/upload", nil)
	req.Header.Set("Content-Type", "multipart/form-data")

	w := httptest.NewRecorder()
	h.Upload(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUpload_NonPDF(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	h := NewDocumentHandler(s, cfg)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.txt")
	part.Write([]byte("not a pdf"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/documents/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	w := httptest.NewRecorder()
	h.Upload(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// --- Process Handler Tests ---

func TestProcess_Success(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	q := queue.NewQueue(10)
	defer q.Close()
	h := NewProcessHandler(s, q, cfg)

	docID := uuid.New()
	doc := &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusPending,
	}
	s.InsertDocument(context.Background(), doc, nil)

	body, _ := json.Marshal(map[string]string{"documentId": docID.String()})
	req := httptest.NewRequest(http.MethodPost, "/api/documents/process", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Process(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d\nbody: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify document status changed to processing
	doc, _ = s.GetDocument(context.Background(), docID, nil)
	if doc.Status != model.StatusProcessing {
		t.Errorf("status = %q, want %q", doc.Status, model.StatusProcessing)
	}

	// Verify job was enqueued
	if q.Len() != 1 {
		t.Errorf("queue length = %d, want 1", q.Len())
	}
}

func TestProcess_DocumentNotFound(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	q := queue.NewQueue(10)
	defer q.Close()
	h := NewProcessHandler(s, q, cfg)

	body, _ := json.Marshal(map[string]string{"documentId": uuid.New().String()})
	req := httptest.NewRequest(http.MethodPost, "/api/documents/process", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Process(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestProcess_AlreadyProcessing(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	q := queue.NewQueue(10)
	defer q.Close()
	h := NewProcessHandler(s, q, cfg)

	docID := uuid.New()
	doc := &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusProcessing,
	}
	s.InsertDocument(context.Background(), doc, nil)

	body, _ := json.Marshal(map[string]string{"documentId": docID.String()})
	req := httptest.NewRequest(http.MethodPost, "/api/documents/process", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Process(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestProcess_InvalidBody(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	q := queue.NewQueue(10)
	defer q.Close()
	h := NewProcessHandler(s, q, cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/documents/process", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Process(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestProcess_InvalidDocumentID(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	q := queue.NewQueue(10)
	defer q.Close()
	h := NewProcessHandler(s, q, cfg)

	body, _ := json.Marshal(map[string]string{"documentId": "not-a-uuid"})
	req := httptest.NewRequest(http.MethodPost, "/api/documents/process", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Process(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestProcess_QueueFull(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	q := queue.NewQueue(1) // capacity 1
	defer q.Close()
	h := NewProcessHandler(s, q, cfg)

	// Fill the queue
	q.Enqueue(queue.NewProcessJob(uuid.New(), 3))

	docID := uuid.New()
	s.InsertDocument(context.Background(), &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusPending,
	}, nil)

	body, _ := json.Marshal(map[string]string{"documentId": docID.String()})
	req := httptest.NewRequest(http.MethodPost, "/api/documents/process", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Process(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	// Verify status reverted to pending
	doc, _ := s.GetDocument(context.Background(), docID, nil)
	if doc.Status != model.StatusPending {
		t.Errorf("status should revert to pending, got %q", doc.Status)
	}
}

// --- Search Handler Tests ---

type mockEmbedder struct {
	embedding []float32
	err       error
}

func (m *mockEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	results := make([][]float32, len(texts))
	for i := range texts {
		results[i] = m.embedding
	}
	return results, nil
}

func (m *mockEmbedder) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.embedding, nil
}

func TestSearch_Success(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	emb := &mockEmbedder{embedding: []float32{1.0, 0.0, 0.0, 0.0}}
	h := NewSearchHandler(s, emb, cfg)

	// Set up data
	docID := uuid.New()
	s.InsertDocument(context.Background(), &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusCompleted,
	}, nil)
	chunkID := uuid.New()
	s.InsertChunks(context.Background(), []model.Chunk{
		{ID: chunkID, DocumentID: docID, Content: "test content", PageNumber: 1, ChunkIndex: 0},
	})
	s.InsertEmbeddings(context.Background(), []uuid.UUID{chunkID}, [][]float32{{1.0, 0.0, 0.0, 0.0}})

	body, _ := json.Marshal(model.SearchRequest{Query: "test query"})
	req := httptest.NewRequest(http.MethodPost, "/api/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Search(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d\nbody: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp model.SearchResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Total)
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	emb := &mockEmbedder{embedding: []float32{1.0}}
	h := NewSearchHandler(s, emb, cfg)

	body, _ := json.Marshal(model.SearchRequest{Query: ""})
	req := httptest.NewRequest(http.MethodPost, "/api/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Search(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSearch_EmbedderError(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	emb := &mockEmbedder{err: fmt.Errorf("sidecar down")}
	h := NewSearchHandler(s, emb, cfg)

	body, _ := json.Marshal(model.SearchRequest{Query: "test"})
	req := httptest.NewRequest(http.MethodPost, "/api/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Search(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestSearch_InvalidBody(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	emb := &mockEmbedder{embedding: []float32{1.0}}
	h := NewSearchHandler(s, emb, cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/search", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Search(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// --- Mock Scraper ---

type mockScraper struct {
	result *scraper.ScrapeResult
	err    error

	extractResult *pdf.ExtractResult
	extractErr    error
}

func (m *mockScraper) Scrape(url string) (*scraper.ScrapeResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func (m *mockScraper) ExtractFromHTML(htmlData []byte, sourceURL string) (*pdf.ExtractResult, error) {
	if m.extractErr != nil {
		return nil, m.extractErr
	}
	return m.extractResult, nil
}

// --- Ingest Handler Tests ---

func TestIngest_Success(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	cfg.MaxIngestURLs = 10
	q := queue.NewQueue(10)
	defer q.Close()

	scr := &mockScraper{
		result: &scraper.ScrapeResult{
			Title:   "Test Article",
			Pages:   []pdf.Page{{Number: 1, Text: "Article content for testing."}},
			RawHTML: []byte("<html><body><p>Article content for testing.</p></body></html>"),
			URL:     "https://example.com/article",
		},
	}

	h := NewIngestHandler(s, scr, q, cfg)

	body, _ := json.Marshal(map[string][]string{"urls": {"https://example.com/article"}})
	req := httptest.NewRequest(http.MethodPost, "/api/documents/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Ingest(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d\nbody: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["documents"] == nil {
		t.Error("expected documents in response")
	}
	if resp["message"] == nil {
		t.Error("expected message in response")
	}

	// Verify job was enqueued
	if q.Len() != 1 {
		t.Errorf("queue length = %d, want 1", q.Len())
	}
}

func TestIngest_MultipleURLs(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	cfg.MaxIngestURLs = 10
	q := queue.NewQueue(10)
	defer q.Close()

	scr := &mockScraper{
		result: &scraper.ScrapeResult{
			Title:   "Test",
			Pages:   []pdf.Page{{Number: 1, Text: "Content."}},
			RawHTML: []byte("<html><body><p>Content.</p></body></html>"),
			URL:     "https://example.com",
		},
	}

	h := NewIngestHandler(s, scr, q, cfg)

	body, _ := json.Marshal(map[string][]string{"urls": {
		"https://example.com/a",
		"https://example.com/b",
		"https://example.com/c",
	}})
	req := httptest.NewRequest(http.MethodPost, "/api/documents/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Ingest(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d\nbody: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	if q.Len() != 3 {
		t.Errorf("queue length = %d, want 3", q.Len())
	}
}

func TestIngest_EmptyURLs(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	q := queue.NewQueue(10)
	defer q.Close()
	scr := &mockScraper{}

	h := NewIngestHandler(s, scr, q, cfg)

	body, _ := json.Marshal(map[string][]string{"urls": {}})
	req := httptest.NewRequest(http.MethodPost, "/api/documents/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Ingest(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestIngest_TooManyURLs(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	cfg.MaxIngestURLs = 2
	q := queue.NewQueue(10)
	defer q.Close()
	scr := &mockScraper{}

	h := NewIngestHandler(s, scr, q, cfg)

	body, _ := json.Marshal(map[string][]string{"urls": {
		"https://example.com/a",
		"https://example.com/b",
		"https://example.com/c",
	}})
	req := httptest.NewRequest(http.MethodPost, "/api/documents/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Ingest(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestIngest_InvalidURL(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	cfg.MaxIngestURLs = 10
	q := queue.NewQueue(10)
	defer q.Close()
	scr := &mockScraper{}

	h := NewIngestHandler(s, scr, q, cfg)

	body, _ := json.Marshal(map[string][]string{"urls": {"ftp://example.com/file"}})
	req := httptest.NewRequest(http.MethodPost, "/api/documents/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Ingest(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestIngest_ScraperError(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	cfg.MaxIngestURLs = 10
	q := queue.NewQueue(10)
	defer q.Close()

	scr := &mockScraper{err: fmt.Errorf("connection refused")}
	h := NewIngestHandler(s, scr, q, cfg)

	body, _ := json.Marshal(map[string][]string{"urls": {"https://example.com/down"}})
	req := httptest.NewRequest(http.MethodPost, "/api/documents/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Ingest(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

func TestIngest_InvalidBody(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	q := queue.NewQueue(10)
	defer q.Close()
	scr := &mockScraper{}

	h := NewIngestHandler(s, scr, q, cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/documents/ingest", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Ingest(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestIngest_QueueFull(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	cfg.MaxIngestURLs = 10
	q := queue.NewQueue(1)
	defer q.Close()

	// Fill the queue
	q.Enqueue(queue.NewProcessJob(uuid.New(), 3))

	scr := &mockScraper{
		result: &scraper.ScrapeResult{
			Title:   "Test",
			Pages:   []pdf.Page{{Number: 1, Text: "Content."}},
			RawHTML: []byte("<html><body>Content.</body></html>"),
			URL:     "https://example.com",
		},
	}

	h := NewIngestHandler(s, scr, q, cfg)

	body, _ := json.Marshal(map[string][]string{"urls": {"https://example.com/article"}})
	req := httptest.NewRequest(http.MethodPost, "/api/documents/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Ingest(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// --- Bulk Upload Tests ---

func TestUploadMultiple_Success(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	h := NewDocumentHandler(s, cfg)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for _, name := range []string{"one.pdf", "two.pdf", "three.pdf"} {
		part, _ := writer.CreateFormFile("files", name)
		part.Write([]byte("%PDF-1.4 test"))
	}
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/documents/upload-bulk", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	w := httptest.NewRecorder()
	h.UploadMultiple(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d\nbody: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	docs, ok := resp["documents"].([]interface{})
	if !ok || len(docs) != 3 {
		t.Errorf("expected 3 documents, got %v", resp["documents"])
	}
}

func TestUploadMultiple_NonPDF(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	h := NewDocumentHandler(s, cfg)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("files", "readme.txt")
	part.Write([]byte("not a pdf"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/documents/upload-bulk", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	w := httptest.NewRecorder()
	h.UploadMultiple(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUploadMultiple_NoFiles(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	h := NewDocumentHandler(s, cfg)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/documents/upload-bulk", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	w := httptest.NewRecorder()
	h.UploadMultiple(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// --- Refresh Handler Tests ---

func TestRefresh_Success(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	q := queue.NewQueue(10)
	defer q.Close()

	scr := &mockScraper{
		result: &scraper.ScrapeResult{
			Title:   "Updated Article",
			Pages:   []pdf.Page{{Number: 1, Text: "Updated content."}},
			RawHTML: []byte("<html><body><p>Updated content.</p></body></html>"),
			URL:     "https://example.com/article",
		},
	}

	h := NewRefreshHandler(s, scr, q, cfg)

	// Create a web document
	docID := uuid.New()
	sourceURL := "https://example.com/article"
	htmlFile := filepath.Join(t.TempDir(), docID.String()+".html")
	os.WriteFile(htmlFile, []byte("<html>old</html>"), 0o644)

	s.InsertDocument(context.Background(), &model.Document{
		ID:         docID,
		Name:       "Test Article",
		FilePath:   htmlFile,
		FileSize:   100,
		Status:     model.StatusCompleted,
		SourceType: model.SourceTypeWeb,
		SourceURL:  &sourceURL,
	}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/documents/"+docID.String()+"/refresh", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", docID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.Refresh(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d\nbody: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify job was enqueued
	if q.Len() != 1 {
		t.Errorf("queue length = %d, want 1", q.Len())
	}

	// Verify file was overwritten
	data, _ := os.ReadFile(htmlFile)
	if string(data) == "<html>old</html>" {
		t.Error("HTML file was not overwritten")
	}
}

func TestRefresh_NotWebSource(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	q := queue.NewQueue(10)
	defer q.Close()
	scr := &mockScraper{}

	h := NewRefreshHandler(s, scr, q, cfg)

	docID := uuid.New()
	s.InsertDocument(context.Background(), &model.Document{
		ID:       docID,
		Name:     "test.pdf",
		FilePath: "/tmp/test.pdf",
		FileSize: 100,
		Status:   model.StatusCompleted,
	}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/documents/"+docID.String()+"/refresh", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", docID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.Refresh(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestRefresh_NotFound(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	q := queue.NewQueue(10)
	defer q.Close()
	scr := &mockScraper{}

	h := NewRefreshHandler(s, scr, q, cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/documents/"+uuid.New().String()+"/refresh", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.Refresh(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestRefresh_ScraperError(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	q := queue.NewQueue(10)
	defer q.Close()

	scr := &mockScraper{err: fmt.Errorf("connection refused")}
	h := NewRefreshHandler(s, scr, q, cfg)

	docID := uuid.New()
	sourceURL := "https://example.com/down"
	s.InsertDocument(context.Background(), &model.Document{
		ID:         docID,
		Name:       "Down Page",
		FilePath:   "/tmp/test.html",
		FileSize:   100,
		Status:     model.StatusCompleted,
		SourceType: model.SourceTypeWeb,
		SourceURL:  &sourceURL,
	}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/documents/"+docID.String()+"/refresh", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", docID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.Refresh(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnprocessableEntity)
	}
}

// --- Tag Handler Tests ---

func TestTag_CreateAndList(t *testing.T) {
	s := newTestStore(t)
	h := NewTagHandler(s)

	body, _ := json.Marshal(map[string]string{"name": "important", "color": "#ef4444"})
	req := httptest.NewRequest(http.MethodPost, "/api/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("create: status = %d, want %d\nbody: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	// List
	req = httptest.NewRequest(http.MethodGet, "/api/tags", nil)
	w = httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("list: status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	tags, ok := resp["tags"].([]interface{})
	if !ok || len(tags) != 1 {
		t.Errorf("expected 1 tag, got %v", resp["tags"])
	}
}

func TestTag_CreateEmptyName(t *testing.T) {
	s := newTestStore(t)
	h := NewTagHandler(s)

	body, _ := json.Marshal(map[string]string{"name": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTag_Delete(t *testing.T) {
	s := newTestStore(t)
	h := NewTagHandler(s)

	tag := &model.Tag{ID: uuid.New(), Name: "delete-me", Color: "#000"}
	s.CreateTag(context.Background(), tag)

	req := httptest.NewRequest(http.MethodDelete, "/api/tags/"+tag.ID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", tag.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d\nbody: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestTag_AddToDocument(t *testing.T) {
	s := newTestStore(t)
	h := NewTagHandler(s)

	docID := uuid.New()
	s.InsertDocument(context.Background(), &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusPending,
	}, nil)

	tag := &model.Tag{ID: uuid.New(), Name: "attached", Color: "#f00"}
	s.CreateTag(context.Background(), tag)

	body, _ := json.Marshal(map[string]string{"tagId": tag.ID.String()})
	req := httptest.NewRequest(http.MethodPost, "/api/documents/"+docID.String()+"/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", docID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.AddToDocument(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d\nbody: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	tags, ok := resp["tags"].([]interface{})
	if !ok || len(tags) != 1 {
		t.Errorf("expected 1 tag on document, got %v", resp["tags"])
	}
}

func TestTag_RemoveFromDocument(t *testing.T) {
	s := newTestStore(t)
	h := NewTagHandler(s)

	docID := uuid.New()
	s.InsertDocument(context.Background(), &model.Document{
		ID: docID, Name: "test.pdf", FilePath: "/tmp/test.pdf", FileSize: 100, Status: model.StatusPending,
	}, nil)

	tag := &model.Tag{ID: uuid.New(), Name: "removeable", Color: "#f00"}
	s.CreateTag(context.Background(), tag)
	s.AddDocumentTag(context.Background(), docID, tag.ID)

	req := httptest.NewRequest(http.MethodDelete, "/api/documents/"+docID.String()+"/tags/"+tag.ID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", docID.String())
	rctx.URLParams.Add("tagId", tag.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.RemoveFromDocument(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	tags, ok := resp["tags"].([]interface{})
	if !ok || len(tags) != 0 {
		t.Errorf("expected 0 tags after removal, got %v", resp["tags"])
	}
}

// --- Auth Handler Tests ---

func TestAuth_Register(t *testing.T) {
	s := newTestStore(t)
	h := NewAuthHandler(s)

	body := bytes.NewBufferString(`{"email":"newuser@example.com","name":"New User"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d. Body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	user, ok := resp["user"].(map[string]interface{})
	if !ok {
		t.Fatal("response missing user field")
	}
	if user["email"] != "newuser@example.com" {
		t.Errorf("email = %v, want 'newuser@example.com'", user["email"])
	}
	apiKey, ok := user["api_key"].(string)
	if !ok || apiKey == "" {
		t.Error("expected non-empty API key")
	}
	if len(apiKey) < 10 {
		t.Errorf("API key too short: %q", apiKey)
	}
}

func TestAuth_Register_DuplicateEmail(t *testing.T) {
	s := newTestStore(t)
	h := NewAuthHandler(s)

	body := bytes.NewBufferString(`{"email":"dup@example.com","name":"User"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Register(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("first register: status = %d", w.Code)
	}

	// Second registration with same email should fail
	body2 := bytes.NewBufferString(`{"email":"dup@example.com","name":"User 2"}`)
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/register", body2)
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	h.Register(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Errorf("duplicate register: status = %d, want %d", w2.Code, http.StatusConflict)
	}
}

func TestAuth_Register_InvalidEmail(t *testing.T) {
	s := newTestStore(t)
	h := NewAuthHandler(s)

	body := bytes.NewBufferString(`{"email":"notanemail","name":"User"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Register(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAuth_Me_Authenticated(t *testing.T) {
	s := newTestStore(t)
	h := NewAuthHandler(s)

	// Create a user first
	user := &model.User{
		ID:     uuid.New(),
		Email:  "me@example.com",
		APIKey: "di_mekey",
		Name:   "Me User",
	}
	s.CreateUser(context.Background(), user)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	// Simulate authenticated context
	ctx := context.WithValue(req.Context(), contextKey("user"), user)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.Me(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	u, ok := resp["user"].(map[string]interface{})
	if !ok {
		t.Fatal("response missing user field")
	}
	// API key should be empty or absent in response
	if apiKey, ok := u["api_key"].(string); ok && apiKey != "" {
		t.Errorf("api_key should be empty/absent in /me response, got %v", apiKey)
	}
}

func TestAuth_Me_Unauthenticated(t *testing.T) {
	s := newTestStore(t)
	h := NewAuthHandler(s)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	w := httptest.NewRecorder()
	h.Me(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_Disabled(t *testing.T) {
	s := newTestStore(t)
	mw := AuthMiddleware(s, false)

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/documents", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("expected handler to be called when auth is disabled")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAuthMiddleware_NoToken(t *testing.T) {
	s := newTestStore(t)
	mw := AuthMiddleware(s, true)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/documents", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	s := newTestStore(t)
	mw := AuthMiddleware(s, true)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/documents", nil)
	req.Header.Set("Authorization", "Bearer invalid_key_xyz")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	s := newTestStore(t)

	user := &model.User{
		ID:     uuid.New(),
		Email:  "auth@example.com",
		APIKey: "di_authtest123",
		Name:   "Auth User",
	}
	s.CreateUser(context.Background(), user)

	mw := AuthMiddleware(s, true)

	var gotUser *model.User
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/documents", nil)
	req.Header.Set("Authorization", "Bearer di_authtest123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if gotUser == nil {
		t.Fatal("expected user in context")
	}
	if gotUser.Email != "auth@example.com" {
		t.Errorf("user email = %q, want 'auth@example.com'", gotUser.Email)
	}
}

// --- Folder Handler Tests ---

func TestFolders_Create(t *testing.T) {
	s := newTestStore(t)
	h := NewFolderHandler(s)

	body := bytes.NewBufferString(`{"name":"Research"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/folders", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	folder, ok := resp["folder"].(map[string]interface{})
	if !ok {
		t.Fatal("response missing folder field")
	}
	if folder["name"].(string) != "Research" {
		t.Errorf("name = %v, want 'Research'", folder["name"])
	}
}

func TestFolders_List(t *testing.T) {
	s := newTestStore(t)
	h := NewFolderHandler(s)

	ctx := context.Background()
	root := &model.Folder{ID: uuid.New(), Name: "root"}
	s.CreateFolder(ctx, root)
	child := &model.Folder{ID: uuid.New(), ParentID: &root.ID, Name: "child"}
	s.CreateFolder(ctx, child)

	// List roots
	req := httptest.NewRequest(http.MethodGet, "/api/folders", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	folders, ok := resp["folders"].([]interface{})
	if !ok {
		t.Fatal("response missing folders field")
	}
	if len(folders) != 1 {
		t.Errorf("expected 1 root folder, got %d", len(folders))
	}

	// List children of root
	req = httptest.NewRequest(http.MethodGet, "/api/folders?parent_id="+root.ID.String(), nil)
	w = httptest.NewRecorder()
	h.List(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	json.NewDecoder(w.Body).Decode(&resp)
	folders, _ = resp["folders"].([]interface{})
	if len(folders) != 1 {
		t.Errorf("expected 1 child folder, got %d", len(folders))
	}
}

func TestFolders_Delete(t *testing.T) {
	s := newTestStore(t)
	h := NewFolderHandler(s)

	ctx := context.Background()
	folder := &model.Folder{ID: uuid.New(), Name: "to-delete"}
	s.CreateFolder(ctx, folder)

	req := httptest.NewRequest(http.MethodDelete, "/api/folders/"+folder.ID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", folder.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	got, _ := s.GetFolder(ctx, folder.ID, nil)
	if got != nil {
		t.Error("folder should be deleted")
	}
}

func TestMoveDocument(t *testing.T) {
	s := newTestStore(t)
	cfg := newTestConfig(t)
	h := NewDocumentHandler(s, cfg)

	ctx := context.Background()
	folder := &model.Folder{ID: uuid.New(), Name: "target"}
	s.CreateFolder(ctx, folder)

	doc := &model.Document{
		ID: uuid.New(), Name: "test.pdf", FilePath: "/tmp/m.pdf", FileSize: 100, Status: model.StatusPending,
	}
	s.InsertDocument(ctx, doc, nil)

	body := bytes.NewBufferString(fmt.Sprintf(`{"folder_id":%q}`, folder.ID.String()))
	req := httptest.NewRequest(http.MethodPost, "/api/documents/"+doc.ID.String()+"/move", body)
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", doc.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.Move(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}

	got, _ := s.GetDocument(ctx, doc.ID, nil)
	if got == nil || got.FolderID == nil || *got.FolderID != folder.ID {
		t.Errorf("folder_id not set correctly after move, got %+v", got)
	}
}
