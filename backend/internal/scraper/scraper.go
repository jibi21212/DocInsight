package scraper

import (
	"github.com/docinsight/backend/internal/pdf"
)

type Scraper interface {
	// Scrape fetches a URL and extracts readable text content.
	Scrape(url string) (*ScrapeResult, error)

	// ExtractFromHTML parses saved HTML without making a network request.
	// Used by the Processor when re-processing web documents.
	ExtractFromHTML(htmlData []byte, sourceURL string) (*pdf.ExtractResult, error)
}

type ScrapeResult struct {
	Title   string
	Pages   []pdf.Page
	RawHTML []byte
	URL     string
}
