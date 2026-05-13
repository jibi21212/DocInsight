package scraper

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const sampleHTML = `<!DOCTYPE html>
<html>
<head><title>Test Article</title></head>
<body>
<nav>Navigation links here</nav>
<article>
<h1>Test Article Title</h1>
<p>This is the first paragraph of a test article. It contains enough text to be recognized as article content by the readability algorithm. We need several sentences to make this work properly.</p>
<p>This is the second paragraph. It also has meaningful content that should be extracted. The readability library needs substantial content to identify the main article body correctly.</p>
<p>Third paragraph continues with more information. Testing the extraction of web content into a format suitable for semantic search and chunking. This paragraph adds more depth to the article.</p>
<p>Fourth paragraph wraps up the article with concluding remarks. The extraction should capture all of these paragraphs as readable content from the web page.</p>
</article>
<footer>Footer content here</footer>
<script>console.log("should be stripped")</script>
</body>
</html>`

const minimalHTML = `<!DOCTYPE html><html><body><p>Short</p></body></html>`

func TestScrape_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(sampleHTML))
	}))
	defer server.Close()

	s := NewReadabilityScraper(10, "TestAgent/1.0")
	result, err := s.Scrape(server.URL)
	if err != nil {
		t.Fatalf("Scrape failed: %v", err)
	}

	if result.Title == "" {
		t.Error("expected non-empty title")
	}
	if len(result.Pages) == 0 {
		t.Fatal("expected at least one page")
	}
	if len(result.RawHTML) == 0 {
		t.Error("expected non-empty raw HTML")
	}
	if result.URL != server.URL {
		t.Errorf("URL = %q, want %q", result.URL, server.URL)
	}

	// Verify text content was extracted
	totalText := ""
	for _, p := range result.Pages {
		totalText += p.Text
	}
	if len(totalText) < 50 {
		t.Errorf("extracted text too short (%d chars): %q", len(totalText), totalText)
	}
}

func TestScrape_InvalidScheme(t *testing.T) {
	s := NewReadabilityScraper(10, "TestAgent/1.0")
	_, err := s.Scrape("ftp://example.com")
	if err == nil {
		t.Fatal("expected error for ftp scheme")
	}
}

func TestScrape_InvalidURL(t *testing.T) {
	s := NewReadabilityScraper(10, "TestAgent/1.0")
	_, err := s.Scrape("not-a-url")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestScrape_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	s := NewReadabilityScraper(10, "TestAgent/1.0")
	_, err := s.Scrape(server.URL)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestScrape_Unreachable(t *testing.T) {
	s := NewReadabilityScraper(2, "TestAgent/1.0")
	_, err := s.Scrape("http://localhost:1")
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestExtractFromHTML_Success(t *testing.T) {
	s := NewReadabilityScraper(10, "TestAgent/1.0")
	result, err := s.ExtractFromHTML([]byte(sampleHTML), "https://example.com/article")
	if err != nil {
		t.Fatalf("ExtractFromHTML failed: %v", err)
	}

	if len(result.Pages) == 0 {
		t.Fatal("expected at least one page")
	}
	if result.PageCount != len(result.Pages) {
		t.Errorf("PageCount = %d, but len(Pages) = %d", result.PageCount, len(result.Pages))
	}

	// Verify page numbering starts at 1
	if result.Pages[0].Number != 1 {
		t.Errorf("first page number = %d, want 1", result.Pages[0].Number)
	}
}

func TestExtractFromHTML_EmptyContent(t *testing.T) {
	s := NewReadabilityScraper(10, "TestAgent/1.0")
	_, err := s.ExtractFromHTML([]byte(`<html><body></body></html>`), "https://example.com")
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestSplitIntoSections(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		minParts int
	}{
		{
			name:     "short text - single section",
			input:    "Hello world. Short paragraph.",
			minParts: 1,
		},
		{
			name:     "multiple paragraphs",
			input:    "First paragraph.\n\nSecond paragraph.\n\nThird paragraph.",
			minParts: 1,
		},
		{
			name:     "empty",
			input:    "",
			minParts: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitIntoSections(tt.input)
			if len(result) < tt.minParts {
				t.Errorf("expected at least %d parts, got %d", tt.minParts, len(result))
			}
		})
	}
}

func TestNewReadabilityScraper_Defaults(t *testing.T) {
	s := NewReadabilityScraper(0, "")
	if s.userAgent != "DocInsight/1.0" {
		t.Errorf("userAgent = %q, want DocInsight/1.0", s.userAgent)
	}
	if s.client.Timeout.Seconds() != 30 {
		t.Errorf("timeout = %v, want 30s", s.client.Timeout)
	}
}
