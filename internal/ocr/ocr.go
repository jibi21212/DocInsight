package ocr

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	pdfpkg "github.com/docinsight/backend/internal/pdf"
)

// Processor handles OCR extraction from scanned PDFs using Tesseract.
type Processor struct {
	tesseractPath string
}

// NewProcessor creates an OCR processor. Pass "" to use "tesseract" from PATH.
func NewProcessor(tesseractPath string) *Processor {
	if tesseractPath == "" {
		tesseractPath = "tesseract"
	}
	return &Processor{tesseractPath: tesseractPath}
}

// Available checks whether the tesseract binary is reachable.
func (p *Processor) Available() bool {
	cmd := exec.Command(p.tesseractPath, "--version")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

// ExtractFromPDF runs Tesseract on raw PDF bytes via stdin and returns the
// extracted text as a single-page result. Tesseract's built-in PDF handler
// treats the file as page images, so this works for scanned documents.
func (p *Processor) ExtractFromPDF(ctx context.Context, pdfData []byte) (*pdfpkg.ExtractResult, error) {
	// tesseract stdin stdout -l eng pdf  -- reads from stdin as PDF, outputs text to stdout
	// We use "stdin" as input and "stdout" as output with the default page segmentation.
	cmd := exec.CommandContext(ctx, p.tesseractPath, "stdin", "stdout", "-l", "eng", "--psm", "3")
	cmd.Stdin = bytes.NewReader(pdfData)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tesseract failed: %w (stderr: %s)", err, stderr.String())
	}

	text := strings.TrimSpace(stdout.String())
	if text == "" {
		return nil, fmt.Errorf("tesseract produced no text output")
	}

	slog.Info("OCR extraction complete", "chars", len(text))

	return &pdfpkg.ExtractResult{
		Pages: []pdfpkg.Page{
			{Number: 1, Text: text},
		},
		PageCount: 1,
	}, nil
}

// IsTextSparse returns true when the extracted text content is suspiciously
// low relative to the file size, suggesting a scanned/image-based PDF.
// minRatio is the minimum chars-per-byte ratio (e.g., 0.1 = at least 1 char
// per 10 bytes of file data).
func IsTextSparse(pages []pdfpkg.Page, fileSize int64, minRatio float64) bool {
	if fileSize == 0 {
		return false
	}
	var totalChars int
	for _, p := range pages {
		totalChars += len(strings.TrimSpace(p.Text))
	}
	ratio := float64(totalChars) / float64(fileSize)
	return ratio < minRatio
}
