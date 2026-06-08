package pdf

import (
	"bytes"
	"fmt"
	"strings"

	gopdf "github.com/ledongthuc/pdf"
)

type LedongthucExtractor struct{}

func NewLedongthucExtractor() *LedongthucExtractor {
	return &LedongthucExtractor{}
}

func (e *LedongthucExtractor) Extract(data []byte) (*ExtractResult, error) {
	reader := bytes.NewReader(data)
	pdfReader, err := gopdf.NewReader(reader, int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open pdf: %w", err)
	}

	pageCount := pdfReader.NumPage()
	pages := make([]Page, 0, pageCount)

	for i := 1; i <= pageCount; i++ {
		page := pdfReader.Page(i)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			// Skip pages that fail to parse
			continue
		}

		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}

		pages = append(pages, Page{
			Number: i,
			Text:   trimmed,
		})
	}

	return &ExtractResult{
		Pages:     pages,
		PageCount: pageCount,
	}, nil
}
