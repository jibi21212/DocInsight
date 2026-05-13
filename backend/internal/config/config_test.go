package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Clear env vars that might be set
	envVars := []string{
		"PORT", "CORS_ORIGIN", "DATABASE_URL", "SQLITE_PATH",
		"UPLOAD_DIR", "MAX_UPLOAD_SIZE_MB",
		"WORKER_COUNT", "QUEUE_CAPACITY", "MAX_RETRIES",
		"EMBEDDING_SIDECAR_URL", "EMBEDDING_BATCH_SIZE", "EMBEDDING_CONCURRENCY",
		"CHUNK_SIZE", "CHUNK_OVERLAP",
		"SEARCH_TOP_K", "SIMILARITY_THRESHOLD",
	}
	for _, v := range envVars {
		os.Unsetenv(v)
	}

	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want %q", cfg.Port, "8080")
	}
	if cfg.CORSOrigin != "http://localhost:3000" {
		t.Errorf("CORSOrigin = %q, want %q", cfg.CORSOrigin, "http://localhost:3000")
	}
	if cfg.DatabaseURL != "" {
		t.Errorf("DatabaseURL = %q, want empty", cfg.DatabaseURL)
	}
	if cfg.SQLitePath != "./docinsight.db" {
		t.Errorf("SQLitePath = %q, want %q", cfg.SQLitePath, "./docinsight.db")
	}
	if cfg.UploadDir != "./uploads" {
		t.Errorf("UploadDir = %q, want %q", cfg.UploadDir, "./uploads")
	}
	if cfg.MaxUploadSizeMB != 50 {
		t.Errorf("MaxUploadSizeMB = %d, want 50", cfg.MaxUploadSizeMB)
	}
	if cfg.WorkerCount != 4 {
		t.Errorf("WorkerCount = %d, want 4", cfg.WorkerCount)
	}
	if cfg.QueueCapacity != 100 {
		t.Errorf("QueueCapacity = %d, want 100", cfg.QueueCapacity)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.EmbeddingSidecarURL != "http://localhost:8000" {
		t.Errorf("EmbeddingSidecarURL = %q, want %q", cfg.EmbeddingSidecarURL, "http://localhost:8000")
	}
	if cfg.EmbeddingBatchSize != 32 {
		t.Errorf("EmbeddingBatchSize = %d, want 32", cfg.EmbeddingBatchSize)
	}
	if cfg.EmbeddingConcurrency != 2 {
		t.Errorf("EmbeddingConcurrency = %d, want 2", cfg.EmbeddingConcurrency)
	}
	if cfg.ChunkSize != 1000 {
		t.Errorf("ChunkSize = %d, want 1000", cfg.ChunkSize)
	}
	if cfg.ChunkOverlap != 200 {
		t.Errorf("ChunkOverlap = %d, want 200", cfg.ChunkOverlap)
	}
	if cfg.SearchTopK != 10 {
		t.Errorf("SearchTopK = %d, want 10", cfg.SearchTopK)
	}
	if cfg.SimilarityThreshold != 0.5 {
		t.Errorf("SimilarityThreshold = %f, want 0.5", cfg.SimilarityThreshold)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("CORS_ORIGIN", "https://example.com")
	t.Setenv("DATABASE_URL", "postgresql://localhost/test")
	t.Setenv("WORKER_COUNT", "8")
	t.Setenv("CHUNK_SIZE", "500")
	t.Setenv("SIMILARITY_THRESHOLD", "0.7")

	cfg := Load()

	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want %q", cfg.Port, "9090")
	}
	if cfg.CORSOrigin != "https://example.com" {
		t.Errorf("CORSOrigin = %q, want %q", cfg.CORSOrigin, "https://example.com")
	}
	if cfg.DatabaseURL != "postgresql://localhost/test" {
		t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, "postgresql://localhost/test")
	}
	if cfg.WorkerCount != 8 {
		t.Errorf("WorkerCount = %d, want 8", cfg.WorkerCount)
	}
	if cfg.ChunkSize != 500 {
		t.Errorf("ChunkSize = %d, want 500", cfg.ChunkSize)
	}
	if cfg.SimilarityThreshold != 0.7 {
		t.Errorf("SimilarityThreshold = %f, want 0.7", cfg.SimilarityThreshold)
	}
}

func TestEnvOrDefaultInt_Invalid(t *testing.T) {
	t.Setenv("TEST_INT", "not_a_number")
	result := envOrDefaultInt("TEST_INT", 42)
	if result != 42 {
		t.Errorf("expected fallback 42, got %d", result)
	}
}

func TestEnvOrDefaultFloat_Invalid(t *testing.T) {
	t.Setenv("TEST_FLOAT", "not_a_float")
	result := envOrDefaultFloat("TEST_FLOAT", 3.14)
	if result != 3.14 {
		t.Errorf("expected fallback 3.14, got %f", result)
	}
}

func TestEnvOrDefaultInt64_Invalid(t *testing.T) {
	t.Setenv("TEST_INT64", "xyz")
	result := envOrDefaultInt64("TEST_INT64", 99)
	if result != 99 {
		t.Errorf("expected fallback 99, got %d", result)
	}
}
