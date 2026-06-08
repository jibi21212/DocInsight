package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port        string
	CORSOrigin  string
	DatabaseURL string
	SQLitePath  string

	UploadDir      string
	MaxUploadSizeMB int64

	WorkerCount  int
	QueueCapacity int
	MaxRetries   int

	EmbeddingSidecarURL  string
	EmbeddingBatchSize   int
	EmbeddingConcurrency int

	ChunkSize    int
	ChunkOverlap int

	SearchTopK           int
	SimilarityThreshold  float64

	ScraperTimeoutSec int
	ScraperUserAgent  string
	MaxIngestURLs     int

	OCREnabled      bool
	TesseractPath   string
	OCRMinTextRatio float64

	MaxCrawlDepth int
	MaxCrawlPages int

	AuthEnabled bool
}

func Load() *Config {
	return &Config{
		Port:        envOrDefault("PORT", "8080"),
		CORSOrigin:  envOrDefault("CORS_ORIGIN", "http://localhost:3000"),
		DatabaseURL: envOrDefault("DATABASE_URL", ""),
		SQLitePath:  envOrDefault("SQLITE_PATH", "./docinsight.db"),

		UploadDir:      envOrDefault("UPLOAD_DIR", "./uploads"),
		MaxUploadSizeMB: envOrDefaultInt64("MAX_UPLOAD_SIZE_MB", 50),

		WorkerCount:  envOrDefaultInt("WORKER_COUNT", 4),
		QueueCapacity: envOrDefaultInt("QUEUE_CAPACITY", 100),
		MaxRetries:   envOrDefaultInt("MAX_RETRIES", 3),

		EmbeddingSidecarURL:  envOrDefault("EMBEDDING_SIDECAR_URL", "http://localhost:8000"),
		EmbeddingBatchSize:   envOrDefaultInt("EMBEDDING_BATCH_SIZE", 32),
		EmbeddingConcurrency: envOrDefaultInt("EMBEDDING_CONCURRENCY", 2),

		ChunkSize:    envOrDefaultInt("CHUNK_SIZE", 1000),
		ChunkOverlap: envOrDefaultInt("CHUNK_OVERLAP", 200),

		SearchTopK:          envOrDefaultInt("SEARCH_TOP_K", 10),
		SimilarityThreshold: envOrDefaultFloat("SIMILARITY_THRESHOLD", 0.25),

		ScraperTimeoutSec: envOrDefaultInt("SCRAPER_TIMEOUT_SEC", 30),
		ScraperUserAgent:  envOrDefault("SCRAPER_USER_AGENT", "DocInsight/1.0"),
		MaxIngestURLs:     envOrDefaultInt("MAX_INGEST_URLS", 10),

		OCREnabled:      envOrDefault("OCR_ENABLED", "true") == "true",
		TesseractPath:   envOrDefault("TESSERACT_PATH", "tesseract"),
		OCRMinTextRatio: envOrDefaultFloat("OCR_MIN_TEXT_RATIO", 0.02),

		MaxCrawlDepth: envOrDefaultInt("MAX_CRAWL_DEPTH", 3),
		MaxCrawlPages: envOrDefaultInt("MAX_CRAWL_PAGES", 50),

		AuthEnabled: envOrDefault("AUTH_ENABLED", "false") == "true",
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrDefaultInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envOrDefaultInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func envOrDefaultFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}
