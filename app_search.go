package main

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/docinsight/backend/internal/model"
	"github.com/google/uuid"
)

// SearchResponse is the result of a Search call exposed to the frontend.
// It mirrors the relevant fields of the old HTTP handler's response; the
// frontend already has the query and result count locally, so only the
// hits and timing are returned here.
type SearchResponse struct {
	Results []model.SearchResult `json:"results"`
	TookMs  int64                `json:"took_ms"`
}

// searchSnippetWindow is the number of characters in a result snippet window.
const searchSnippetWindow = 240

// Search runs a semantic, keyword, or hybrid search over the user's indexed
// chunks. searchMode is one of "hybrid" (default), "semantic", or "keyword".
// A topK <= 0 or threshold <= 0 falls back to the configured defaults. An
// empty folderID searches across all folders; otherwise it must be a valid
// UUID and scopes the search to that folder.
func (a *App) Search(query string, topK int, threshold float64, searchMode string, folderID string) (*SearchResponse, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	if topK <= 0 {
		topK = a.cfg.SearchTopK
	}
	if threshold <= 0 {
		threshold = a.cfg.SimilarityThreshold
	}

	mode := strings.ToLower(strings.TrimSpace(searchMode))
	if mode == "" {
		mode = "hybrid"
	}

	var folderUUID *uuid.UUID
	if folderID != "" {
		f, err := uuid.Parse(folderID)
		if err != nil {
			return nil, fmt.Errorf("invalid folder ID: %w", err)
		}
		folderUUID = &f
	}

	start := time.Now()

	var results []model.SearchResult
	var err error

	switch mode {
	case "keyword":
		results, err = a.store.KeywordSearch(a.ctx, query, topK, nil, a.userID, folderUUID)
	case "semantic":
		queryEmb, embErr := a.emb.EmbedSingle(a.ctx, query)
		if embErr != nil {
			slog.Error("search: failed to generate query embedding", "error", embErr)
			return nil, fmt.Errorf("failed to generate query embedding: %w", embErr)
		}
		results, err = a.store.MatchEmbeddings(a.ctx, queryEmb, threshold, topK, nil, a.userID, folderUUID)
		for i := range results {
			results[i].MatchType = "semantic"
		}
	default: // "hybrid"
		queryEmb, embErr := a.emb.EmbedSingle(a.ctx, query)
		if embErr != nil {
			slog.Error("search: failed to generate query embedding", "error", embErr)
			return nil, fmt.Errorf("failed to generate query embedding: %w", embErr)
		}
		results, err = a.store.HybridSearch(a.ctx, queryEmb, query, threshold, topK, nil, a.userID, folderUUID)
	}

	if err != nil {
		slog.Error("search failed", "mode", mode, "error", err)
		return nil, fmt.Errorf("search failed: %w", err)
	}

	if results == nil {
		results = []model.SearchResult{}
	}

	// Populate snippet + highlight tokens for each result. Tokens are the
	// same across the response, so they could be shared, but each result
	// carries its own copy for the client.
	for i := range results {
		snippet, tokens := extractSnippet(results[i].Content, query, searchSnippetWindow)
		results[i].Snippet = snippet
		results[i].HighlightTokens = tokens
	}

	tookMs := time.Since(start).Milliseconds()

	return &SearchResponse{
		Results: results,
		TookMs:  tookMs,
	}, nil
}

// searchStopwords are filtered out of query tokens when building a snippet.
// Kept intentionally small — these are very common words that would produce
// useless matches if used to center a snippet window.
var searchStopwords = map[string]struct{}{
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

// tokenizeSearchQuery splits a query into normalized tokens: lowercase,
// length >= 2, and not in the English stopword list.
func tokenizeSearchQuery(query string) []string {
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
		if _, stop := searchStopwords[t]; stop {
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

// findEarliestSearchMatch returns the earliest index in lowerContent where any
// of the supplied tokens appears, or -1 if no token matches.
func findEarliestSearchMatch(lowerContent string, tokens []string) int {
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

// extractSnippet returns a windowed excerpt of content centered on the earliest
// match of any non-stopword token from query, plus the token slice used to
// build it (usable as highlight tokens on the client).
//
// If no token matches, the function returns the first windowSize characters of
// content. Edges that don't reach content boundaries are decorated with a
// leading or trailing horizontal ellipsis "…".
func extractSnippet(content, query string, windowSize int) (string, []string) {
	tokens := tokenizeSearchQuery(query)
	if windowSize <= 0 {
		return "", tokens
	}

	if len(content) <= windowSize {
		// Short content: return whole thing, no ellipsis.
		return content, tokens
	}

	lower := strings.ToLower(content)
	matchOffset := findEarliestSearchMatch(lower, tokens)

	if matchOffset < 0 {
		// Fallback: leading slice with trailing ellipsis (content > windowSize here).
		return content[:windowSize] + "…", tokens
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
		snippet = "…" + snippet
	}
	if end < len(content) {
		snippet = snippet + "…"
	}
	return snippet, tokens
}
