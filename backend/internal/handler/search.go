package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/docinsight/backend/internal/config"
	"github.com/docinsight/backend/internal/embedder"
	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/store"
)

type SearchHandler struct {
	store    store.Store
	embedder embedder.Embedder
	cfg      *config.Config
}

func NewSearchHandler(s store.Store, emb embedder.Embedder, cfg *config.Config) *SearchHandler {
	return &SearchHandler{store: s, embedder: emb, cfg: cfg}
}

func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	var req model.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	query := strings.TrimSpace(req.Query)
	if query == "" {
		writeError(w, http.StatusBadRequest, "Query is required")
		return
	}

	topK := h.cfg.SearchTopK
	if req.TopK != nil {
		topK = *req.TopK
	}
	threshold := h.cfg.SimilarityThreshold
	if req.Threshold != nil {
		threshold = *req.Threshold
	}

	mode := strings.ToLower(strings.TrimSpace(req.SearchMode))
	if mode == "" {
		mode = "hybrid"
	}

	start := time.Now()

	var results []model.SearchResult
	var err error

	switch mode {
	case "keyword":
		results, err = h.store.KeywordSearch(r.Context(), query, topK, req.DocumentIDs)
	case "semantic":
		queryEmb, embErr := h.embedder.EmbedSingle(r.Context(), query)
		if embErr != nil {
			writeError(w, http.StatusInternalServerError, "Failed to generate query embedding")
			return
		}
		results, err = h.store.MatchEmbeddings(r.Context(), queryEmb, threshold, topK, req.DocumentIDs)
		for i := range results {
			results[i].MatchType = "semantic"
		}
	default: // "hybrid"
		queryEmb, embErr := h.embedder.EmbedSingle(r.Context(), query)
		if embErr != nil {
			writeError(w, http.StatusInternalServerError, "Failed to generate query embedding")
			return
		}
		results, err = h.store.HybridSearch(r.Context(), queryEmb, query, threshold, topK, req.DocumentIDs)
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "Search failed")
		return
	}

	if results == nil {
		results = []model.SearchResult{}
	}

	tookMs := time.Since(start).Milliseconds()

	resp := model.SearchResponse{
		Results: results,
		Query:   query,
		Total:   len(results),
		TookMs:  tookMs,
	}

	writeJSON(w, http.StatusOK, resp)
}
