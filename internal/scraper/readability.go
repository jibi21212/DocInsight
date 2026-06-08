package scraper

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	readability "github.com/go-shiori/go-readability"

	"github.com/docinsight/backend/internal/pdf"
)

type ReadabilityScraper struct {
	client    *http.Client
	userAgent string
}

func NewReadabilityScraper(timeoutSec int, userAgent string) *ReadabilityScraper {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	if userAgent == "" {
		userAgent = "DocInsight/1.0"
	}
	return &ReadabilityScraper{
		client: &http.Client{
			Timeout: time.Duration(timeoutSec) * time.Second,
		},
		userAgent: userAgent,
	}
}

func (s *ReadabilityScraper) Scrape(rawURL string) (*ScrapeResult, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme %q: only http and https are supported", parsed.Scheme)
	}

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, rawURL)
	}

	htmlData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	pages, title, err := extractReadableContent(htmlData, parsed)
	if err != nil {
		return nil, err
	}

	return &ScrapeResult{
		Title:   title,
		Pages:   pages,
		RawHTML: htmlData,
		URL:     rawURL,
	}, nil
}

func (s *ReadabilityScraper) ExtractFromHTML(htmlData []byte, sourceURL string) (*pdf.ExtractResult, error) {
	parsed, _ := url.Parse(sourceURL)
	if parsed == nil {
		parsed = &url.URL{}
	}

	pages, _, err := extractReadableContent(htmlData, parsed)
	if err != nil {
		return nil, err
	}

	return &pdf.ExtractResult{
		Pages:     pages,
		PageCount: len(pages),
	}, nil
}

func extractReadableContent(htmlData []byte, parsedURL *url.URL) ([]pdf.Page, string, error) {
	article, err := readability.FromReader(bytes.NewReader(htmlData), parsedURL)
	if err != nil {
		return nil, "", fmt.Errorf("readability extraction failed: %w", err)
	}

	text := strings.TrimSpace(article.TextContent)
	if text == "" {
		return nil, article.Title, fmt.Errorf("no readable text content found at %s", parsedURL.String())
	}

	// Split into sections by double-newlines for multi-page representation.
	// Each section becomes a "page" for the chunker.
	sections := splitIntoSections(text)
	pages := make([]pdf.Page, len(sections))
	for i, section := range sections {
		pages[i] = pdf.Page{
			Number: i + 1,
			Text:   section,
		}
	}

	return pages, article.Title, nil
}

// splitIntoSections splits text by double-newlines into meaningful sections.
// If the text has no clear section breaks, returns a single section.
func splitIntoSections(text string) []string {
	parts := strings.Split(text, "\n\n")
	var sections []string
	var current strings.Builder

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		// Accumulate small paragraphs into sections of reasonable size
		if current.Len() > 0 && current.Len()+len(trimmed) > 2000 {
			sections = append(sections, current.String())
			current.Reset()
		}
		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(trimmed)
	}
	if current.Len() > 0 {
		sections = append(sections, current.String())
	}

	if len(sections) == 0 {
		return []string{text}
	}
	return sections
}
