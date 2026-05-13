package chunker

import (
	"strings"
	"testing"

	"github.com/docinsight/backend/internal/pdf"
	"github.com/google/uuid"
)

func TestSplitIntoSentences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple sentences",
			input:    "Hello world. How are you? I am fine!",
			expected: []string{"Hello world.", "How are you?", "I am fine!"},
		},
		{
			name:     "single sentence",
			input:    "Just one sentence.",
			expected: []string{"Just one sentence."},
		},
		{
			name:     "no punctuation",
			input:    "No punctuation here",
			expected: []string{"No punctuation here"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: nil,
		},
		{
			name:     "multiple spaces between sentences",
			input:    "First sentence.   Second sentence.",
			expected: []string{"First sentence.", "Second sentence."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitIntoSentences(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d sentences, got %d: %v", len(tt.expected), len(result), result)
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("sentence[%d] = %q, want %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestSplitIntoParagraphs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "two paragraphs",
			input:    "First paragraph.\n\nSecond paragraph.",
			expected: 2,
		},
		{
			name:     "single paragraph",
			input:    "Just one paragraph with no breaks.",
			expected: 1,
		},
		{
			name:     "three paragraphs with extra whitespace",
			input:    "First.\n\n\n  \nSecond.\n\nThird.",
			expected: 3,
		},
		{
			name:     "empty string",
			input:    "",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitIntoParagraphs(tt.input)
			if len(result) != tt.expected {
				t.Fatalf("expected %d paragraphs, got %d: %v", tt.expected, len(result), result)
			}
		})
	}
}

func TestCreateChunks_BasicChunking(t *testing.T) {
	pages := []pdf.Page{
		{Number: 1, Text: "This is the first page with some content."},
		{Number: 2, Text: "This is the second page with more content."},
	}

	chunks := CreateChunks(pages, 1000, 200)

	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}

	// With small text and large chunk size, should be 1 chunk
	if len(chunks) != 1 {
		t.Logf("got %d chunks (expected 1 for small text)", len(chunks))
	}

	// Verify chunk index starts at 0
	if chunks[0].ChunkIndex != 0 {
		t.Errorf("first chunk index = %d, want 0", chunks[0].ChunkIndex)
	}

	// Verify metadata
	if chunks[0].Metadata.StartPage != 1 {
		t.Errorf("start page = %d, want 1", chunks[0].Metadata.StartPage)
	}
	if chunks[0].Metadata.CharCount == 0 {
		t.Error("char count should be > 0")
	}
	if chunks[0].Metadata.WordCount == 0 {
		t.Error("word count should be > 0")
	}
}

func TestCreateChunks_SplitsLargeText(t *testing.T) {
	// Create text that exceeds chunk size
	longText := strings.Repeat("This is a sentence. ", 100) // ~2000 chars

	pages := []pdf.Page{
		{Number: 1, Text: longText},
	}

	chunks := CreateChunks(pages, 200, 50)

	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks for large text, got %d", len(chunks))
	}

	// Verify sequential chunk indices
	for i, c := range chunks {
		if c.ChunkIndex != i {
			t.Errorf("chunk[%d].ChunkIndex = %d, want %d", i, c.ChunkIndex, i)
		}
	}
}

func TestCreateChunks_EmptyPages(t *testing.T) {
	result := CreateChunks(nil, 1000, 200)
	if result != nil {
		t.Errorf("expected nil for empty pages, got %v", result)
	}

	result = CreateChunks([]pdf.Page{}, 1000, 200)
	if result != nil {
		t.Errorf("expected nil for empty slice, got %v", result)
	}
}

func TestCreateChunks_MultiPageTracking(t *testing.T) {
	// Create content across multiple pages, each paragraph small enough to fit in one chunk
	pages := []pdf.Page{
		{Number: 1, Text: "Page one content paragraph one.\n\nPage one content paragraph two."},
		{Number: 2, Text: "Page two content paragraph one.\n\nPage two content paragraph two."},
		{Number: 3, Text: "Page three content."},
	}

	chunks := CreateChunks(pages, 1000, 200)

	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}

	// First chunk should reference page 1
	if chunks[0].Metadata.StartPage != 1 {
		t.Errorf("first chunk start page = %d, want 1", chunks[0].Metadata.StartPage)
	}
}

func TestToModelChunks(t *testing.T) {
	docID := uuid.New()
	textChunks := []TextChunk{
		{Content: "chunk 1", PageNumber: 1, ChunkIndex: 0, Metadata: ChunkMetadata{CharCount: 7, WordCount: 2}},
		{Content: "chunk 2", PageNumber: 2, ChunkIndex: 1, Metadata: ChunkMetadata{CharCount: 7, WordCount: 2}},
	}

	// Need to import model type alias
	modelChunks := ToModelChunks(textChunks, docID)

	if len(modelChunks) != 2 {
		t.Fatalf("expected 2 model chunks, got %d", len(modelChunks))
	}

	for i, mc := range modelChunks {
		if mc.DocumentID != docID {
			t.Errorf("chunk[%d].DocumentID = %v, want %v", i, mc.DocumentID, docID)
		}
		if mc.Content != textChunks[i].Content {
			t.Errorf("chunk[%d].Content = %q, want %q", i, mc.Content, textChunks[i].Content)
		}
		if mc.ID == uuid.Nil {
			t.Errorf("chunk[%d].ID should not be nil", i)
		}
	}
}

// ChunkMetadata alias for test readability
type ChunkMetadata = struct {
	CharCount int `json:"char_count"`
	WordCount int `json:"word_count"`
	StartPage int `json:"start_page"`
	EndPage   int `json:"end_page"`
}

func TestGetOverlapParts(t *testing.T) {
	parts := []string{"hello", "world", "foo", "bar"}

	result := getOverlapParts(parts, 5)
	if len(result) == 0 {
		t.Fatal("expected at least one overlap part")
	}

	// Should include "bar" (3 chars) and "foo" (3 chars, total 6 >= 5)
	if result[len(result)-1] != "bar" {
		t.Errorf("last overlap part = %q, want %q", result[len(result)-1], "bar")
	}
}

func TestJoinLength(t *testing.T) {
	parts := []string{"hello", "world"}
	length := joinLength(parts, ", ")
	// "hello" (5) + ", " (2) + "world" (5) = 12
	expected := 12
	if length != expected {
		t.Errorf("joinLength = %d, want %d", length, expected)
	}

	if joinLength(nil, ", ") != 0 {
		t.Error("joinLength of nil should be 0")
	}
}
