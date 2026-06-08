package handler

import (
	"strings"
	"testing"
)

func TestExtractSnippet_MatchInMiddle(t *testing.T) {
	content := strings.Repeat("alpha beta gamma ", 30) + "photosynthesis is amazing " + strings.Repeat("delta epsilon ", 30)
	snippet, tokens := ExtractSnippet(content, "photosynthesis", 240)

	if len(snippet) == 0 {
		t.Fatal("expected non-empty snippet")
	}
	if !strings.Contains(strings.ToLower(snippet), "photosynthesis") {
		t.Errorf("snippet should contain match, got %q", snippet)
	}
	if !strings.HasPrefix(snippet, "\u2026") {
		t.Errorf("snippet should start with ellipsis since match is in middle, got %q", snippet)
	}
	if !strings.HasSuffix(snippet, "\u2026") {
		t.Errorf("snippet should end with ellipsis, got %q", snippet)
	}
	if len(tokens) != 1 || tokens[0] != "photosynthesis" {
		t.Errorf("tokens = %v, want [photosynthesis]", tokens)
	}
}

func TestExtractSnippet_MatchAtStart(t *testing.T) {
	content := "photosynthesis is the process by which plants convert sunlight " + strings.Repeat("filler text words here ", 30)
	snippet, _ := ExtractSnippet(content, "photosynthesis", 240)

	if !strings.HasPrefix(strings.ToLower(snippet), "photosynthesis") {
		t.Errorf("snippet should start with the match (no leading ellipsis), got %q", snippet)
	}
	if strings.HasPrefix(snippet, "\u2026") {
		t.Errorf("snippet should not have leading ellipsis when match is at start, got %q", snippet)
	}
	if !strings.HasSuffix(snippet, "\u2026") {
		t.Errorf("snippet should have trailing ellipsis, got %q", snippet)
	}
}

func TestExtractSnippet_NoMatch(t *testing.T) {
	content := strings.Repeat("nothing relevant here whatsoever ", 30)
	snippet, tokens := ExtractSnippet(content, "photosynthesis", 240)

	if len(snippet) == 0 {
		t.Fatal("expected non-empty snippet")
	}
	// First 240 chars + trailing ellipsis
	if !strings.HasSuffix(snippet, "\u2026") {
		t.Errorf("no-match fallback should end with ellipsis, got %q", snippet)
	}
	if strings.HasPrefix(snippet, "\u2026") {
		t.Errorf("no-match fallback should not start with ellipsis, got %q", snippet)
	}
	// Excluding the ellipsis rune, the leading slice should be exactly 240 bytes
	trimmed := strings.TrimSuffix(snippet, "\u2026")
	if len(trimmed) != 240 {
		t.Errorf("snippet body length = %d, want 240", len(trimmed))
	}
	if len(tokens) != 1 || tokens[0] != "photosynthesis" {
		t.Errorf("tokens = %v, want [photosynthesis]", tokens)
	}
}

func TestExtractSnippet_MultipleTokens(t *testing.T) {
	// "mitochondria" appears earlier than "atp", so it should anchor the snippet.
	prefix := strings.Repeat("xx ", 40)
	content := prefix + "the mitochondria contain many enzymes " + strings.Repeat("middle ", 30) + "and produce atp molecules" + strings.Repeat(" tail", 30)
	snippet, tokens := ExtractSnippet(content, "atp mitochondria", 240)

	if !strings.Contains(strings.ToLower(snippet), "mitochondria") {
		t.Errorf("snippet should contain the earliest matched token, got %q", snippet)
	}
	if len(tokens) != 2 {
		t.Errorf("tokens = %v, want 2 tokens", tokens)
	}
}

func TestExtractSnippet_CaseInsensitive(t *testing.T) {
	content := strings.Repeat("aaa ", 30) + "Photosynthesis works wonders " + strings.Repeat("bbb ", 30)
	snippet, _ := ExtractSnippet(content, "photosynthesis", 240)

	if !strings.Contains(snippet, "Photosynthesis") {
		t.Errorf("snippet should contain the original-cased match, got %q", snippet)
	}
}

func TestExtractSnippet_StopwordsFiltered(t *testing.T) {
	tokens := tokenizeQuery("the and of photosynthesis")
	if len(tokens) != 1 || tokens[0] != "photosynthesis" {
		t.Errorf("tokens = %v, want [photosynthesis] (stopwords filtered)", tokens)
	}

	// Single-char tokens filtered too
	tokens2 := tokenizeQuery("a b photosynthesis")
	if len(tokens2) != 1 || tokens2[0] != "photosynthesis" {
		t.Errorf("tokens2 = %v, want [photosynthesis] (1-char filtered)", tokens2)
	}
}

func TestExtractSnippet_ShortContent(t *testing.T) {
	content := "Short content with photosynthesis here."
	snippet, tokens := ExtractSnippet(content, "photosynthesis", 240)

	if snippet != content {
		t.Errorf("short content should be returned verbatim, got %q want %q", snippet, content)
	}
	if strings.HasPrefix(snippet, "\u2026") || strings.HasSuffix(snippet, "\u2026") {
		t.Errorf("short content should have no ellipsis, got %q", snippet)
	}
	if len(tokens) != 1 || tokens[0] != "photosynthesis" {
		t.Errorf("tokens = %v, want [photosynthesis]", tokens)
	}
}
