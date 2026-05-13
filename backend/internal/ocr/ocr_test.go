package ocr

import (
	"testing"

	pdfpkg "github.com/docinsight/backend/internal/pdf"
)

func TestIsTextSparse_Empty(t *testing.T) {
	// No file size → not sparse (avoid division by zero edge case)
	if IsTextSparse(nil, 0, 0.02) {
		t.Error("expected false for zero file size")
	}
}

func TestIsTextSparse_SparseDocument(t *testing.T) {
	// 10 chars in a 10KB file → ratio = 0.001, well below 0.02
	pages := []pdfpkg.Page{{Number: 1, Text: "0123456789"}}
	if !IsTextSparse(pages, 10000, 0.02) {
		t.Error("expected sparse for 10 chars in 10KB file")
	}
}

func TestIsTextSparse_NormalDocument(t *testing.T) {
	// 500 chars in a 5KB file → ratio = 0.1, above 0.02
	pages := []pdfpkg.Page{{Number: 1, Text: string(make([]byte, 500))}}
	// string(make([]byte, 500)) is 500 null bytes — TrimSpace will strip them.
	// Use a real string instead.
	text := ""
	for i := 0; i < 500; i++ {
		text += "a"
	}
	pages = []pdfpkg.Page{{Number: 1, Text: text}}
	if IsTextSparse(pages, 5000, 0.02) {
		t.Error("expected not sparse for 500 chars in 5KB file")
	}
}

func TestIsTextSparse_MultiplePages(t *testing.T) {
	pages := []pdfpkg.Page{
		{Number: 1, Text: "short"},
		{Number: 2, Text: "also short"},
	}
	// 15 chars in 50KB → sparse
	if !IsTextSparse(pages, 50000, 0.02) {
		t.Error("expected sparse for 15 chars in 50KB file")
	}
}

func TestNewProcessor_DefaultPath(t *testing.T) {
	p := NewProcessor("")
	if p.tesseractPath != "tesseract" {
		t.Errorf("default path = %q, want 'tesseract'", p.tesseractPath)
	}
}

func TestNewProcessor_CustomPath(t *testing.T) {
	p := NewProcessor("/usr/local/bin/tesseract")
	if p.tesseractPath != "/usr/local/bin/tesseract" {
		t.Errorf("custom path = %q, want '/usr/local/bin/tesseract'", p.tesseractPath)
	}
}

func TestAvailable_NotInstalled(t *testing.T) {
	p := NewProcessor("nonexistent-tesseract-binary-xyz")
	if p.Available() {
		t.Error("expected Available() = false for nonexistent binary")
	}
}
