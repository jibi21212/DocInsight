package handler

import (
	"strings"
)

// englishStopwords are filtered out of query tokens when building a snippet.
// Keeping the list intentionally small — these are very common words that
// would produce useless matches if used to center a snippet window.
var englishStopwords = map[string]struct{}{
	"the":  {},
	"and":  {},
	"or":   {},
	"of":   {},
	"to":   {},
	"in":   {},
	"is":   {},
	"a":    {},
	"an":   {},
	"on":   {},
	"at":   {},
	"by":   {},
	"for":  {},
	"with": {},
	"as":   {},
	"it":   {},
	"be":   {},
	"are":  {},
	"was":  {},
	"were": {},
	"this": {},
	"that": {},
}

// tokenizeQuery splits a query into normalized tokens: lowercase, length >= 2,
// and not in the English stopword list.
func tokenizeQuery(query string) []string {
	fields := strings.Fields(query)
	tokens := make([]string, 0, len(fields))
	seen := make(map[string]struct{})
	for _, f := range fields {
		t := strings.ToLower(strings.TrimFunc(f, func(r rune) bool {
			return !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9')
		}))
		if len(t) < 2 {
			continue
		}
		if _, stop := englishStopwords[t]; stop {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		tokens = append(tokens, t)
	}
	return tokens
}

// findEarliestMatch returns the earliest index in lowerContent where any of
// the supplied tokens appears, or -1 if no token matches.
func findEarliestMatch(lowerContent string, tokens []string) int {
	earliest := -1
	for _, tok := range tokens {
		idx := strings.Index(lowerContent, tok)
		if idx < 0 {
			continue
		}
		if earliest < 0 || idx < earliest {
			earliest = idx
		}
	}
	return earliest
}

// ExtractSnippet returns a windowed excerpt of content centered on the
// earliest match of any non-stopword token from query, plus the token slice
// used to build it (callers may share it across results in the same response
// for use as highlight tokens on the client).
//
// If no token matches, the function returns the first windowSize characters
// of content. Edges that don't reach content boundaries are decorated with a
// leading or trailing horizontal ellipsis "…".
func ExtractSnippet(content, query string, windowSize int) (string, []string) {
	tokens := tokenizeQuery(query)
	if windowSize <= 0 {
		return "", tokens
	}

	if len(content) <= windowSize {
		// Short content: return whole thing, no ellipsis.
		offset := findEarliestMatch(strings.ToLower(content), tokens)
		_ = offset
		return content, tokens
	}

	lower := strings.ToLower(content)
	matchOffset := findEarliestMatch(lower, tokens)

	if matchOffset < 0 {
		// Fallback: leading slice with trailing ellipsis (content > windowSize here).
		return content[:windowSize] + "\u2026", tokens
	}

	// Center the window: start 50 chars before the match.
	const lead = 50
	start := matchOffset - lead
	if start < 0 {
		start = 0
	}
	end := start + windowSize
	if end > len(content) {
		end = len(content)
		// Pull start back if there's room so the window stays full-sized.
		if end-windowSize > 0 {
			start = end - windowSize
		} else {
			start = 0
		}
	}

	snippet := content[start:end]
	if start > 0 {
		snippet = "\u2026" + snippet
	}
	if end < len(content) {
		snippet = snippet + "\u2026"
	}
	return snippet, tokens
}
