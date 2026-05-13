package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCrawl_SinglePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><h1>Hello</h1></body></html>`)
	}))
	defer srv.Close()

	c := NewCrawler(10)
	result, err := c.Crawl(context.Background(), CrawlOptions{
		StartURL: srv.URL,
		MaxDepth: 2,
		MaxPages: 10,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if len(result.URLs) != 1 {
		t.Errorf("expected 1 URL, got %d", len(result.URLs))
	}
}

func TestCrawl_FollowsLinks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/", "":
			fmt.Fprint(w, `<html><body><a href="/about">About</a><a href="/contact">Contact</a></body></html>`)
		case "/about":
			fmt.Fprint(w, `<html><body><h1>About</h1><a href="/team">Team</a></body></html>`)
		case "/contact":
			fmt.Fprint(w, `<html><body><h1>Contact</h1></body></html>`)
		case "/team":
			fmt.Fprint(w, `<html><body><h1>Team</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewCrawler(10)
	result, err := c.Crawl(context.Background(), CrawlOptions{
		StartURL: srv.URL,
		MaxDepth: 2,
		MaxPages: 50,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if len(result.URLs) != 4 {
		t.Errorf("expected 4 URLs (/, /about, /contact, /team), got %d: %v", len(result.URLs), result.URLs)
	}
}

func TestCrawl_RespectsMaxDepth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/", "":
			fmt.Fprint(w, `<html><body><a href="/level1">Level 1</a></body></html>`)
		case "/level1":
			fmt.Fprint(w, `<html><body><a href="/level2">Level 2</a></body></html>`)
		case "/level2":
			fmt.Fprint(w, `<html><body><a href="/level3">Level 3</a></body></html>`)
		case "/level3":
			fmt.Fprint(w, `<html><body><h1>Deep</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewCrawler(10)
	result, err := c.Crawl(context.Background(), CrawlOptions{
		StartURL: srv.URL,
		MaxDepth: 1, // Only follow 1 level deep
		MaxPages: 50,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	// Should get: / (depth 0) and /level1 (depth 1), but NOT /level2 (depth 2)
	if len(result.URLs) != 2 {
		t.Errorf("expected 2 URLs with MaxDepth=1, got %d: %v", len(result.URLs), result.URLs)
	}
}

func TestCrawl_RespectsMaxPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// Home page links to 10 other pages
		if r.URL.Path == "/" || r.URL.Path == "" {
			links := ""
			for i := 0; i < 10; i++ {
				links += fmt.Sprintf(`<a href="/page%d">Page %d</a>`, i, i)
			}
			fmt.Fprintf(w, `<html><body>%s</body></html>`, links)
		} else {
			fmt.Fprint(w, `<html><body><h1>Page</h1></body></html>`)
		}
	}))
	defer srv.Close()

	c := NewCrawler(10)
	result, err := c.Crawl(context.Background(), CrawlOptions{
		StartURL: srv.URL,
		MaxDepth: 3,
		MaxPages: 3,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if len(result.URLs) > 3 {
		t.Errorf("expected <= 3 URLs with MaxPages=3, got %d", len(result.URLs))
	}
}

func TestCrawl_SameDomainOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>
			<a href="/local">Local</a>
			<a href="https://external.example.com/page">External</a>
		</body></html>`)
	}))
	defer srv.Close()

	c := NewCrawler(10)
	result, err := c.Crawl(context.Background(), CrawlOptions{
		StartURL: srv.URL,
		MaxDepth: 2,
		MaxPages: 50,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	for _, u := range result.URLs {
		if strings.Contains(u, "external.example.com") {
			t.Errorf("should not crawl external domain, got: %s", u)
		}
	}
}

func TestCrawl_SkipsNonHTMLContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/image.png" {
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte("fake png"))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><a href="/image.png">Image</a></body></html>`)
	}))
	defer srv.Close()

	c := NewCrawler(10)
	result, err := c.Crawl(context.Background(), CrawlOptions{
		StartURL: srv.URL,
		MaxDepth: 2,
		MaxPages: 50,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	// Should only have the root page — /image.png is filtered by shouldSkipURL
	if len(result.URLs) != 1 {
		t.Errorf("expected 1 URL, got %d: %v", len(result.URLs), result.URLs)
	}
}

func TestCrawl_NoDuplicates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>
			<a href="/">Home</a>
			<a href="/">Home Again</a>
			<a href="/about">About</a>
			<a href="/about#section">About Section</a>
		</body></html>`)
	}))
	defer srv.Close()

	c := NewCrawler(10)
	result, err := c.Crawl(context.Background(), CrawlOptions{
		StartURL: srv.URL,
		MaxDepth: 2,
		MaxPages: 50,
	})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	// Should deduplicate by normalized URL
	seen := make(map[string]bool)
	for _, u := range result.URLs {
		norm := normalizeURL(u)
		if seen[norm] {
			t.Errorf("duplicate URL: %s", u)
		}
		seen[norm] = true
	}
}

func TestExtractLinks(t *testing.T) {
	html := `<html><body>
		<a href="/page1">Page 1</a>
		<a href="https://example.com/page2">Page 2</a>
		<a href="#anchor">Anchor</a>
		<a href="javascript:void(0)">JS</a>
		<a href="mailto:test@example.com">Email</a>
		<a href="/valid">Valid</a>
	</body></html>`

	links, err := extractLinks(strings.NewReader(html))
	if err != nil {
		t.Fatalf("extractLinks: %v", err)
	}
	// Should get /page1, https://example.com/page2, /valid
	// Should NOT get #anchor, javascript:, mailto:
	if len(links) != 3 {
		t.Errorf("expected 3 links, got %d: %v", len(links), links)
	}
}

func TestShouldSkipURL(t *testing.T) {
	tests := []struct {
		path string
		skip bool
	}{
		{"/page", false},
		{"/about", false},
		{"/image.png", true},
		{"/style.css", true},
		{"/app.js", true},
		{"/doc.pdf", true},
		{"/font.woff2", true},
		{"/data.json", true},
	}

	for _, tt := range tests {
		u, _ := parseTestURL("https://example.com" + tt.path)
		got := shouldSkipURL(u)
		if got != tt.skip {
			t.Errorf("shouldSkipURL(%q) = %v, want %v", tt.path, got, tt.skip)
		}
	}
}

func parseTestURL(raw string) (*url.URL, error) {
	return url.Parse(raw)
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://example.com/page#section", "https://example.com/page"},
		{"https://example.com/page/", "https://example.com/page"},
		{"https://example.com/", "https://example.com"},
	}

	for _, tt := range tests {
		got := normalizeURL(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
