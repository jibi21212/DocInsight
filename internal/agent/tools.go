package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/docinsight/backend/internal/embedder"
	"github.com/docinsight/backend/internal/llm"
	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/store"
	"github.com/google/uuid"
)

// Tool names advertised to the LLM.
const (
	ToolSearchDocuments   = SearchToolName // "search_documents"
	ToolGetDocument       = "get_document"
	ToolSummarizeDocument = "summarize_document"
	ToolListDocuments     = "list_documents"
	ToolGetChunkContext   = "get_chunk_context"
)

// getDocumentMaxContent caps the size of content returned by get_document.
const getDocumentMaxContent = 8000

// listDocumentsDefaultLimit is the default page size for list_documents.
const listDocumentsDefaultLimit = 20

// listDocumentsMaxLimit is the silent ceiling for list_documents.limit.
const listDocumentsMaxLimit = 100

// chunkContextDefaultWindow is the default ± window for get_chunk_context.
const chunkContextDefaultWindow = 3

// chunkContextMaxWindow is the silent ceiling for get_chunk_context.window.
const chunkContextMaxWindow = 10

// Tool input schemas.
var (
	getDocumentSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "document_id": {"type": "string", "description": "UUID of the document to read."}
  },
  "required": ["document_id"]
}`)

	summarizeDocumentSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "document_id": {"type": "string", "description": "UUID of the document to summarize."},
    "length": {"type": "string", "enum": ["short", "medium", "long"], "description": "Summary length. Default: medium."}
  },
  "required": ["document_id"]
}`)

	listDocumentsSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "folder_id": {"type": "string", "description": "Optional UUID to scope the listing to a folder."},
    "status": {"type": "string", "description": "Optional status filter (pending|processing|completed|failed)."},
    "limit": {"type": "integer", "description": "Max documents to return. Default 20, max 100."}
  }
}`)

	getChunkContextSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "chunk_id": {"type": "string", "description": "UUID of the focal chunk."},
    "window": {"type": "integer", "description": "Number of chunks before and after to include. Default 3, max 10."}
  },
  "required": ["chunk_id"]
}`)
)

// ToolDispatcher owns tool registration and execution for an Agent. It is
// constructed once at Agent build time and is safe to reuse across requests
// because it carries no per-request state of its own.
type ToolDispatcher struct {
	store    store.Store
	llm      llm.Client
	embedder embedder.Embedder
}

// NewToolDispatcher returns a ToolDispatcher backed by the supplied dependencies.
func NewToolDispatcher(s store.Store, l llm.Client, e embedder.Embedder) *ToolDispatcher {
	return &ToolDispatcher{store: s, llm: l, embedder: e}
}

// Specs returns the tool definitions to advertise to the LLM.
func (d *ToolDispatcher) Specs() []llm.Tool {
	return []llm.Tool{
		{
			Name:        ToolSearchDocuments,
			Description: "Search the user's document library and return the most relevant chunks.",
			InputSchema: searchToolSchema,
		},
		{
			Name:        ToolGetDocument,
			Description: "Read the full text of a document by its UUID. Content is capped at 8000 characters; the 'truncated' flag is true when the document was longer.",
			InputSchema: getDocumentSchema,
		},
		{
			Name:        ToolSummarizeDocument,
			Description: "Produce a summary of a document. 'length' controls verbosity: short (one sentence), medium (3-5 sentences), long (two paragraphs).",
			InputSchema: summarizeDocumentSchema,
		},
		{
			Name:        ToolListDocuments,
			Description: "List documents in the user's library, optionally filtered by folder or status.",
			InputSchema: listDocumentsSchema,
		},
		{
			Name:        ToolGetChunkContext,
			Description: "Return the chunks around a focal chunk_id (the focal chunk plus ±window neighbours) so the agent can read surrounding context.",
			InputSchema: getChunkContextSchema,
		},
	}
}

// Dispatch executes a single tool call. It returns the JSON payload that
// should be passed back to the LLM as the tool result, any citations to
// surface in the SSE stream, a short human-readable display label for the UI,
// and a fatal error (rare — most tool failures are encoded as a JSON {"error":...}
// payload so the LLM can recover gracefully).
func (d *ToolDispatcher) Dispatch(ctx context.Context, session *model.AgentSession, tc llm.ToolCall, apiKey string) (resultJSON string, citations []model.Citation, displayLabel string, err error) {
	switch tc.Name {
	case ToolSearchDocuments:
		return d.dispatchSearch(ctx, session, tc)
	case ToolGetDocument:
		return d.dispatchGetDocument(ctx, session, tc)
	case ToolSummarizeDocument:
		return d.dispatchSummarizeDocument(ctx, session, tc, apiKey)
	case ToolListDocuments:
		return d.dispatchListDocuments(ctx, session, tc)
	case ToolGetChunkContext:
		return d.dispatchGetChunkContext(ctx, session, tc)
	default:
		return fmt.Sprintf(`{"error":"unknown tool: %s"}`, jsonEscape(tc.Name)), nil, "", nil
	}
}

// dispatchSearch executes a search_documents tool call. Mirrors the previous
// inline implementation in agent.go.
func (d *ToolDispatcher) dispatchSearch(ctx context.Context, session *model.AgentSession, tc llm.ToolCall) (string, []model.Citation, string, error) {
	var args struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := json.Unmarshal(tc.Args, &args); err != nil {
		return fmt.Sprintf(`{"error":"invalid args: %s"}`, jsonEscape(err.Error())), nil, "", nil
	}
	if strings.TrimSpace(args.Query) == "" {
		return `{"error":"missing required arg: query"}`, nil, "", nil
	}
	if args.TopK <= 0 {
		args.TopK = 5
	}

	label := fmt.Sprintf("Searching for %q", args.Query)

	embs, err := d.embedder.Embed(ctx, []string{args.Query})
	if err != nil {
		return fmt.Sprintf(`{"error":"embed: %s"}`, jsonEscape(err.Error())), nil, label, nil
	}
	if len(embs) == 0 {
		return `{"error":"empty embedding"}`, nil, label, nil
	}
	results, err := d.store.HybridSearch(ctx, embs[0], args.Query, 0.0, args.TopK, nil, &session.UserID, session.FolderID)
	if err != nil {
		return fmt.Sprintf(`{"error":"search: %s"}`, jsonEscape(err.Error())), nil, label, nil
	}

	items := make([]searchResultItem, 0, len(results))
	cites := make([]model.Citation, 0, len(results))
	for _, r := range results {
		snippet, _ := capRunes(r.Content, SnippetMaxLen)
		items = append(items, searchResultItem{
			ChunkID:      r.ChunkID.String(),
			DocumentID:   r.DocumentID.String(),
			DocumentName: r.DocumentName,
			Snippet:      snippet,
			PageNumber:   r.PageNumber,
			Score:        r.Similarity,
		})
		cites = append(cites, model.Citation{
			ChunkID:      r.ChunkID,
			DocumentID:   r.DocumentID,
			DocumentName: r.DocumentName,
			Snippet:      snippet,
			PageNumber:   r.PageNumber,
			Score:        r.Similarity,
		})
	}
	payload, _ := json.Marshal(items)
	return string(payload), cites, label, nil
}

// dispatchGetDocument returns the full body of a document (capped).
func (d *ToolDispatcher) dispatchGetDocument(ctx context.Context, session *model.AgentSession, tc llm.ToolCall) (string, []model.Citation, string, error) {
	var args struct {
		DocumentID string `json:"document_id"`
	}
	if err := json.Unmarshal(tc.Args, &args); err != nil {
		return fmt.Sprintf(`{"error":"invalid args: %s"}`, jsonEscape(err.Error())), nil, "", nil
	}
	if args.DocumentID == "" {
		return `{"error":"missing required arg: document_id"}`, nil, "", nil
	}
	docID, err := uuid.Parse(args.DocumentID)
	if err != nil {
		return fmt.Sprintf(`{"error":"invalid document_id: %s"}`, jsonEscape(err.Error())), nil, "", nil
	}

	uid := session.UserID
	doc, err := d.store.GetDocument(ctx, docID, &uid)
	if err != nil {
		return fmt.Sprintf(`{"error":"get document: %s"}`, jsonEscape(err.Error())), nil, "", nil
	}
	if doc == nil {
		// Same response whether the document is missing or owned by a different
		// user — never leak existence information across users.
		return `{"error":"document not found"}`, nil, "", nil
	}

	chunks, err := d.store.GetChunksByDocumentID(ctx, docID)
	if err != nil {
		return fmt.Sprintf(`{"error":"get chunks: %s"}`, jsonEscape(err.Error())), nil, "", nil
	}

	var b strings.Builder
	for i, c := range chunks {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(c.Content)
	}
	content, truncated := capRunes(b.String(), getDocumentMaxContent)

	out := map[string]any{
		"title":     doc.Name,
		"status":    string(doc.Status),
		"content":   content,
		"truncated": truncated,
	}
	payload, _ := json.Marshal(out)
	label := fmt.Sprintf("Reading %s", doc.Name)
	return string(payload), nil, label, nil
}

// summaryLengthInstruction maps a length keyword to a natural-language target
// length hint embedded in the system prompt.
func summaryLengthInstruction(length string) string {
	switch length {
	case "short":
		return "1 sentence"
	case "long":
		return "2 paragraphs"
	default:
		return "3-5 sentences"
	}
}

// dispatchSummarizeDocument issues a nested LLM call to summarize a document.
// No citations are emitted: DocInsight's Citation schema requires a chunk_id,
// and document-level citations are out of scope for this phase.
func (d *ToolDispatcher) dispatchSummarizeDocument(ctx context.Context, session *model.AgentSession, tc llm.ToolCall, apiKey string) (string, []model.Citation, string, error) {
	var args struct {
		DocumentID string `json:"document_id"`
		Length     string `json:"length"`
	}
	if err := json.Unmarshal(tc.Args, &args); err != nil {
		return fmt.Sprintf(`{"error":"invalid args: %s"}`, jsonEscape(err.Error())), nil, "", nil
	}
	if args.DocumentID == "" {
		return `{"error":"missing required arg: document_id"}`, nil, "", nil
	}
	docID, err := uuid.Parse(args.DocumentID)
	if err != nil {
		return fmt.Sprintf(`{"error":"invalid document_id: %s"}`, jsonEscape(err.Error())), nil, "", nil
	}
	length := strings.ToLower(strings.TrimSpace(args.Length))
	if length != "short" && length != "long" {
		length = "medium"
	}

	uid := session.UserID
	doc, err := d.store.GetDocument(ctx, docID, &uid)
	if err != nil {
		return fmt.Sprintf(`{"error":"get document: %s"}`, jsonEscape(err.Error())), nil, "", nil
	}
	if doc == nil {
		return `{"error":"document not found"}`, nil, "", nil
	}

	chunks, err := d.store.GetChunksByDocumentID(ctx, docID)
	if err != nil {
		return fmt.Sprintf(`{"error":"get chunks: %s"}`, jsonEscape(err.Error())), nil, "", nil
	}

	var bodyBuf strings.Builder
	for i, c := range chunks {
		if i > 0 {
			bodyBuf.WriteString("\n\n")
		}
		bodyBuf.WriteString(c.Content)
	}
	body, _ := capRunes(bodyBuf.String(), getDocumentMaxContent)

	if d.llm == nil {
		return `{"error":"llm client unavailable"}`, nil, "", nil
	}

	system := fmt.Sprintf("Summarize the document the user provides in %s. Be faithful to the source; do not invent details.", summaryLengthInstruction(length))
	msgs := []llm.Message{{Role: "user", Content: body}}

	stream, err := d.llm.StreamChat(ctx, apiKey, session.Model, system, msgs, nil)
	if err != nil {
		return fmt.Sprintf(`{"error":"summarize: %s"}`, jsonEscape(err.Error())), nil, "", nil
	}
	var summary strings.Builder
	for ev := range stream {
		switch ev.Type {
		case llm.StreamText:
			summary.WriteString(ev.Text)
		case llm.StreamError:
			return fmt.Sprintf(`{"error":"summarize stream: %s"}`, jsonEscape(ev.Error)), nil, "", nil
		}
	}

	out := map[string]any{
		"summary":            summary.String(),
		"source_document_id": doc.ID.String(),
		"source_title":       doc.Name,
	}
	payload, _ := json.Marshal(out)
	label := fmt.Sprintf("Summarizing %s", doc.Name)
	return string(payload), nil, label, nil
}

// listDocsItem is the slim view of a document returned by list_documents.
type listDocsItem struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// dispatchListDocuments returns the user's documents, optionally filtered.
func (d *ToolDispatcher) dispatchListDocuments(ctx context.Context, session *model.AgentSession, tc llm.ToolCall) (string, []model.Citation, string, error) {
	var args struct {
		FolderID string `json:"folder_id"`
		Status   string `json:"status"`
		Limit    int    `json:"limit"`
	}
	if len(tc.Args) > 0 {
		if err := json.Unmarshal(tc.Args, &args); err != nil {
			return fmt.Sprintf(`{"error":"invalid args: %s"}`, jsonEscape(err.Error())), nil, "", nil
		}
	}

	var folderID *uuid.UUID
	if args.FolderID != "" {
		parsed, err := uuid.Parse(args.FolderID)
		if err != nil {
			return fmt.Sprintf(`{"error":"invalid folder_id: %s"}`, jsonEscape(err.Error())), nil, "", nil
		}
		folderID = &parsed
	}

	var statusPtr *string
	if args.Status != "" {
		s := args.Status
		statusPtr = &s
	}

	limit := args.Limit
	if limit <= 0 {
		limit = listDocumentsDefaultLimit
	}
	if limit > listDocumentsMaxLimit {
		limit = listDocumentsMaxLimit
	}

	uid := session.UserID
	docs, total, err := d.store.ListDocuments(ctx, 1, limit, statusPtr, &uid, folderID)
	if err != nil {
		return fmt.Sprintf(`{"error":"list documents: %s"}`, jsonEscape(err.Error())), nil, "", nil
	}

	items := make([]listDocsItem, 0, len(docs))
	for _, doc := range docs {
		items = append(items, listDocsItem{
			ID:        doc.ID.String(),
			Title:     doc.Name,
			Status:    string(doc.Status),
			CreatedAt: doc.CreatedAt,
		})
	}
	out := map[string]any{
		"documents": items,
		"total":     total,
	}
	payload, _ := json.Marshal(out)
	return string(payload), nil, "Listing documents", nil
}

// chunkContextItem is one chunk returned by get_chunk_context.
type chunkContextItem struct {
	ChunkID    string `json:"chunk_id"`
	ChunkIndex int    `json:"chunk_index"`
	Content    string `json:"content"`
}

// dispatchGetChunkContext returns the focal chunk and ±window neighbours.
func (d *ToolDispatcher) dispatchGetChunkContext(ctx context.Context, session *model.AgentSession, tc llm.ToolCall) (string, []model.Citation, string, error) {
	var args struct {
		ChunkID string `json:"chunk_id"`
		Window  int    `json:"window"`
	}
	if err := json.Unmarshal(tc.Args, &args); err != nil {
		return fmt.Sprintf(`{"error":"invalid args: %s"}`, jsonEscape(err.Error())), nil, "", nil
	}
	if args.ChunkID == "" {
		return `{"error":"missing required arg: chunk_id"}`, nil, "", nil
	}
	chunkID, err := uuid.Parse(args.ChunkID)
	if err != nil {
		return fmt.Sprintf(`{"error":"invalid chunk_id: %s"}`, jsonEscape(err.Error())), nil, "", nil
	}

	window := args.Window
	if window <= 0 {
		window = chunkContextDefaultWindow
	}
	if window > chunkContextMaxWindow {
		window = chunkContextMaxWindow
	}

	uid := session.UserID
	focal, err := d.store.GetChunkByID(ctx, chunkID, &uid)
	if err != nil {
		return fmt.Sprintf(`{"error":"get chunk: %s"}`, jsonEscape(err.Error())), nil, "", nil
	}
	if focal == nil {
		return `{"error":"chunk not found"}`, nil, "", nil
	}

	doc, err := d.store.GetDocument(ctx, focal.DocumentID, &uid)
	if err != nil {
		return fmt.Sprintf(`{"error":"get document: %s"}`, jsonEscape(err.Error())), nil, "", nil
	}
	if doc == nil {
		// Belt and braces — GetChunkByID already enforces ownership.
		return `{"error":"document not found"}`, nil, "", nil
	}

	all, err := d.store.GetChunksByDocumentID(ctx, focal.DocumentID)
	if err != nil {
		return fmt.Sprintf(`{"error":"get chunks: %s"}`, jsonEscape(err.Error())), nil, "", nil
	}

	lo := focal.ChunkIndex - window
	hi := focal.ChunkIndex + window
	items := make([]chunkContextItem, 0, 2*window+1)
	cites := make([]model.Citation, 0, 2*window+1)
	for _, c := range all {
		if c.ChunkIndex < lo || c.ChunkIndex > hi {
			continue
		}
		snippet, _ := capRunes(c.Content, SnippetMaxLen)
		items = append(items, chunkContextItem{
			ChunkID:    c.ID.String(),
			ChunkIndex: c.ChunkIndex,
			Content:    c.Content,
		})
		cites = append(cites, model.Citation{
			ChunkID:      c.ID,
			DocumentID:   doc.ID,
			DocumentName: doc.Name,
			Snippet:      snippet,
			PageNumber:   c.PageNumber,
		})
	}

	out := map[string]any{
		"document_id":    doc.ID.String(),
		"document_title": doc.Name,
		"chunks":         items,
	}
	payload, _ := json.Marshal(out)
	return string(payload), cites, "Expanding context", nil
}

// capRunes truncates s to at most maxRunes runes without splitting a multibyte
// rune, returning the (possibly shortened) string and whether it was cut. A raw
// byte slice at an arbitrary offset can land mid-rune and yield invalid UTF-8
// (json.Marshal then coerces it to U+FFFD); capping by rune count avoids that.
// See LESSONS_LEARNED.md — the UTF-8 window-boundary rule.
func capRunes(s string, maxRunes int) (string, bool) {
	// Byte length is an upper bound on rune count: if the bytes fit, the runes
	// fit too — fast path with no allocation.
	if len(s) <= maxRunes {
		return s, false
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s, false
	}
	return string(r[:maxRunes]), true
}

// jsonEscape escapes a string so it can be safely embedded in a JSON string
// literal built via fmt.Sprintf. It is intentionally narrow — only the
// characters that would otherwise break parsing.
func jsonEscape(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return s
	}
	// Strip surrounding quotes added by json.Marshal.
	if len(b) >= 2 {
		return string(b[1 : len(b)-1])
	}
	return s
}
