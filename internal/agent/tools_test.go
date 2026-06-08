package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/docinsight/backend/internal/llm"
	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/store"
	"github.com/google/uuid"
)

// capturingLLM records calls to StreamChat so summarize_document tests can
// inspect the system prompt, messages, and apiKey the dispatcher forwards.
type capturingLLM struct {
	calls []capturedCall
	// streamText is the text streamed back from every call.
	streamText string
}

type capturedCall struct {
	apiKey   string
	model    string
	system   string
	messages []llm.Message
	tools    []llm.Tool
}

func (c *capturingLLM) StreamChat(ctx context.Context, apiKey, modelName, system string, messages []llm.Message, tools []llm.Tool) (<-chan llm.StreamEvent, error) {
	c.calls = append(c.calls, capturedCall{
		apiKey:   apiKey,
		model:    modelName,
		system:   system,
		messages: append([]llm.Message(nil), messages...),
		tools:    append([]llm.Tool(nil), tools...),
	})
	ch := make(chan llm.StreamEvent, 2)
	ch <- llm.StreamEvent{Type: llm.StreamText, Text: c.streamText}
	ch <- llm.StreamEvent{Type: llm.StreamDone}
	close(ch)
	return ch, nil
}

// seedDocWithChunks inserts one document owned by ownerID with `n` chunks,
// each containing the supplied content. Returns docID and chunk IDs in order.
func seedDocWithChunks(t *testing.T, s store.Store, ownerID uuid.UUID, name string, contents []string, folderID *uuid.UUID) (uuid.UUID, []uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	docID := uuid.New()
	doc := &model.Document{
		ID:       docID,
		Name:     name,
		FilePath: "/tmp/" + name,
		FileSize: 100,
		Status:   model.StatusCompleted,
		UserID:   &ownerID,
		FolderID: folderID,
	}
	if err := s.InsertDocument(ctx, doc, &ownerID); err != nil {
		t.Fatalf("InsertDocument: %v", err)
	}
	ids := make([]uuid.UUID, 0, len(contents))
	chunks := make([]model.Chunk, 0, len(contents))
	for i, body := range contents {
		cid := uuid.New()
		chunks = append(chunks, model.Chunk{
			ID:         cid,
			DocumentID: docID,
			Content:    body,
			PageNumber: 1,
			ChunkIndex: i,
		})
		ids = append(ids, cid)
	}
	if _, err := s.InsertChunks(ctx, chunks); err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}
	return docID, ids
}

// createUser registers a user in the store with a synthetic e-mail/API key.
func createUser(t *testing.T, s store.Store, userID uuid.UUID) {
	t.Helper()
	u := &model.User{ID: userID, Email: fmt.Sprintf("u-%s@example.com", userID.String()[:8]), APIKey: "di_" + userID.String(), Name: "user"}
	if err := s.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
}

func newDispatcher(s store.Store, l llm.Client) *ToolDispatcher {
	return NewToolDispatcher(s, l, &mockEmbedder{vec: []float32{1, 0, 0, 0}})
}

// ---------- get_document ----------

func TestGetDocument_HappyPath(t *testing.T) {
	s := newTestStore(t)
	userID := uuid.New()
	createUser(t, s, userID)
	docID, _ := seedDocWithChunks(t, s, userID, "report.pdf", []string{"alpha", "bravo", "charlie"}, nil)

	d := newDispatcher(s, nil)
	tc := llm.ToolCall{ID: "1", Name: ToolGetDocument, Args: json.RawMessage(fmt.Sprintf(`{"document_id":%q}`, docID.String()))}

	result, _, label, err := d.Dispatch(context.Background(), &model.AgentSession{UserID: userID}, tc, "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	var out struct {
		Title     string `json:"title"`
		Status    string `json:"status"`
		Content   string `json:"content"`
		Truncated bool   `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, result)
	}
	if out.Title != "report.pdf" {
		t.Errorf("title = %q, want report.pdf", out.Title)
	}
	if out.Status != string(model.StatusCompleted) {
		t.Errorf("status = %q, want %q", out.Status, model.StatusCompleted)
	}
	for _, want := range []string{"alpha", "bravo", "charlie"} {
		if !strings.Contains(out.Content, want) {
			t.Errorf("content missing %q: %s", want, out.Content)
		}
	}
	if out.Truncated {
		t.Errorf("truncated unexpectedly")
	}
	if !strings.Contains(label, "report.pdf") {
		t.Errorf("display label = %q, want to contain report.pdf", label)
	}
}

func TestGetDocument_Truncation(t *testing.T) {
	s := newTestStore(t)
	userID := uuid.New()
	createUser(t, s, userID)
	big := strings.Repeat("a", getDocumentMaxContent+500)
	docID, _ := seedDocWithChunks(t, s, userID, "big.pdf", []string{big}, nil)

	d := newDispatcher(s, nil)
	tc := llm.ToolCall{ID: "1", Name: ToolGetDocument, Args: json.RawMessage(fmt.Sprintf(`{"document_id":%q}`, docID.String()))}
	result, _, _, err := d.Dispatch(context.Background(), &model.AgentSession{UserID: userID}, tc, "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	var out struct {
		Content   string `json:"content"`
		Truncated bool   `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.Truncated {
		t.Errorf("truncated = false, want true")
	}
	// ASCII is one byte per rune, so the content must be cut to EXACTLY the cap.
	// A "<= cap" assertion would also pass a regression that over-truncates.
	if got := utf8.RuneCountInString(out.Content); got != getDocumentMaxContent {
		t.Errorf("content rune count = %d, want exactly %d", got, getDocumentMaxContent)
	}
	if got := len(out.Content); got != getDocumentMaxContent {
		t.Errorf("content byte length = %d, want exactly %d", got, getDocumentMaxContent)
	}
}

// TestGetDocument_TruncationMultibyte guards the documented UTF-8 rule: a naive
// byte slice at the cap would split a multibyte rune and emit invalid UTF-8.
func TestGetDocument_TruncationMultibyte(t *testing.T) {
	s := newTestStore(t)
	userID := uuid.New()
	createUser(t, s, userID)
	// "世" is 3 bytes / 1 rune. getDocumentMaxContent is not a multiple of 3, so a
	// raw byte slice at the cap necessarily lands mid-rune (the bug we fixed);
	// rune-aware capping keeps exactly cap-many whole runes. (A 2-byte rune like
	// "é" would divide evenly into an even cap and miss the bug entirely.)
	const wide = "世"
	big := strings.Repeat(wide, getDocumentMaxContent+500)
	docID, _ := seedDocWithChunks(t, s, userID, "wide.pdf", []string{big}, nil)

	d := newDispatcher(s, nil)
	tc := llm.ToolCall{ID: "1", Name: ToolGetDocument, Args: json.RawMessage(fmt.Sprintf(`{"document_id":%q}`, docID.String()))}
	result, _, _, err := d.Dispatch(context.Background(), &model.AgentSession{UserID: userID}, tc, "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	var out struct {
		Content   string `json:"content"`
		Truncated bool   `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.Truncated {
		t.Errorf("truncated = false, want true")
	}
	// Strongest signal: a byte slice would yield ~cap/3 runes ending in U+FFFD
	// (json coerces the split trailing bytes); rune capping yields exactly
	// cap-many identical whole runes.
	if out.Content != strings.Repeat(wide, getDocumentMaxContent) {
		t.Errorf("content not truncated on a rune boundary: got %d runes, want %d identical %q runes",
			utf8.RuneCountInString(out.Content), getDocumentMaxContent, wide)
	}
	if !utf8.ValidString(out.Content) {
		t.Errorf("content is not valid UTF-8")
	}
}

func TestGetDocument_NotFound(t *testing.T) {
	s := newTestStore(t)
	userID := uuid.New()
	createUser(t, s, userID)

	d := newDispatcher(s, nil)
	tc := llm.ToolCall{ID: "1", Name: ToolGetDocument, Args: json.RawMessage(fmt.Sprintf(`{"document_id":%q}`, uuid.New().String()))}

	result, _, _, err := d.Dispatch(context.Background(), &model.AgentSession{UserID: userID}, tc, "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !strings.Contains(result, `"error"`) {
		t.Errorf("expected error in result, got %s", result)
	}
}

func TestGetDocument_WrongUser(t *testing.T) {
	s := newTestStore(t)
	owner := uuid.New()
	intruder := uuid.New()
	createUser(t, s, owner)
	createUser(t, s, intruder)
	docID, _ := seedDocWithChunks(t, s, owner, "secret.pdf", []string{"top secret"}, nil)

	d := newDispatcher(s, nil)
	tc := llm.ToolCall{ID: "1", Name: ToolGetDocument, Args: json.RawMessage(fmt.Sprintf(`{"document_id":%q}`, docID.String()))}

	result, _, _, err := d.Dispatch(context.Background(), &model.AgentSession{UserID: intruder}, tc, "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !strings.Contains(result, `"error"`) {
		t.Fatalf("expected error for wrong-user, got %s", result)
	}
	if strings.Contains(result, "top secret") {
		t.Errorf("content leaked to wrong user: %s", result)
	}
}

func TestGetDocument_MissingArg(t *testing.T) {
	s := newTestStore(t)
	userID := uuid.New()
	createUser(t, s, userID)

	d := newDispatcher(s, nil)
	tc := llm.ToolCall{ID: "1", Name: ToolGetDocument, Args: json.RawMessage(`{}`)}

	result, _, _, err := d.Dispatch(context.Background(), &model.AgentSession{UserID: userID}, tc, "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !strings.Contains(result, "missing required arg") {
		t.Errorf("expected missing-arg error, got %s", result)
	}
}

// ---------- summarize_document ----------

func TestSummarizeDocument_HappyPath(t *testing.T) {
	s := newTestStore(t)
	userID := uuid.New()
	createUser(t, s, userID)
	docID, _ := seedDocWithChunks(t, s, userID, "paper.pdf", []string{"some body text"}, nil)

	llmMock := &capturingLLM{streamText: "This is a summary."}
	d := newDispatcher(s, llmMock)

	tc := llm.ToolCall{ID: "1", Name: ToolSummarizeDocument, Args: json.RawMessage(fmt.Sprintf(`{"document_id":%q}`, docID.String()))}
	result, _, label, err := d.Dispatch(context.Background(), &model.AgentSession{UserID: userID, Model: "claude-test"}, tc, "user-api-key")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	var out struct {
		Summary          string `json:"summary"`
		SourceDocumentID string `json:"source_document_id"`
		SourceTitle      string `json:"source_title"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, result)
	}
	if out.Summary != "This is a summary." {
		t.Errorf("summary = %q, want %q", out.Summary, "This is a summary.")
	}
	if out.SourceDocumentID != docID.String() {
		t.Errorf("source_document_id mismatch: %s != %s", out.SourceDocumentID, docID.String())
	}
	if out.SourceTitle != "paper.pdf" {
		t.Errorf("source_title = %q", out.SourceTitle)
	}
	if len(llmMock.calls) != 1 {
		t.Fatalf("expected 1 nested LLM call, got %d", len(llmMock.calls))
	}
	if llmMock.calls[0].apiKey != "user-api-key" {
		t.Errorf("apiKey = %q, want %q", llmMock.calls[0].apiKey, "user-api-key")
	}
	if !strings.Contains(label, "paper.pdf") {
		t.Errorf("label = %q, want to contain paper.pdf", label)
	}
}

func TestSummarizeDocument_LengthVariants(t *testing.T) {
	s := newTestStore(t)
	userID := uuid.New()
	createUser(t, s, userID)
	docID, _ := seedDocWithChunks(t, s, userID, "paper.pdf", []string{"body"}, nil)

	cases := []struct {
		length     string
		wantSubstr string
	}{
		{"short", "1 sentence"},
		{"medium", "3-5 sentences"},
		{"long", "2 paragraphs"},
	}

	for _, tc := range cases {
		t.Run(tc.length, func(t *testing.T) {
			llmMock := &capturingLLM{streamText: "ok"}
			d := newDispatcher(s, llmMock)
			call := llm.ToolCall{ID: "1", Name: ToolSummarizeDocument, Args: json.RawMessage(fmt.Sprintf(`{"document_id":%q,"length":%q}`, docID.String(), tc.length))}
			_, _, _, err := d.Dispatch(context.Background(), &model.AgentSession{UserID: userID, Model: "claude-test"}, call, "k")
			if err != nil {
				t.Fatalf("Dispatch: %v", err)
			}
			if len(llmMock.calls) != 1 {
				t.Fatalf("expected 1 call, got %d", len(llmMock.calls))
			}
			if !strings.Contains(llmMock.calls[0].system, tc.wantSubstr) {
				t.Errorf("system prompt missing %q: %s", tc.wantSubstr, llmMock.calls[0].system)
			}
		})
	}
}

// Default length should fall back to medium when unspecified.
func TestSummarizeDocument_DefaultLength(t *testing.T) {
	s := newTestStore(t)
	userID := uuid.New()
	createUser(t, s, userID)
	docID, _ := seedDocWithChunks(t, s, userID, "paper.pdf", []string{"body"}, nil)

	llmMock := &capturingLLM{streamText: "ok"}
	d := newDispatcher(s, llmMock)
	tc := llm.ToolCall{ID: "1", Name: ToolSummarizeDocument, Args: json.RawMessage(fmt.Sprintf(`{"document_id":%q}`, docID.String()))}
	if _, _, _, err := d.Dispatch(context.Background(), &model.AgentSession{UserID: userID, Model: "claude-test"}, tc, "k"); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(llmMock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(llmMock.calls))
	}
	if !strings.Contains(llmMock.calls[0].system, "3-5 sentences") {
		t.Errorf("default length should pick medium (3-5 sentences); system = %q", llmMock.calls[0].system)
	}
}

// ---------- list_documents ----------

func TestListDocuments_FolderScope(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := uuid.New()
	createUser(t, s, userID)

	folderA := uuid.New()
	folderB := uuid.New()
	if err := s.CreateFolder(ctx, &model.Folder{ID: folderA, UserID: &userID, Name: "A"}); err != nil {
		t.Fatalf("CreateFolder A: %v", err)
	}
	if err := s.CreateFolder(ctx, &model.Folder{ID: folderB, UserID: &userID, Name: "B"}); err != nil {
		t.Fatalf("CreateFolder B: %v", err)
	}
	seedDocWithChunks(t, s, userID, "in-a-1.pdf", []string{"x"}, &folderA)
	seedDocWithChunks(t, s, userID, "in-a-2.pdf", []string{"x"}, &folderA)
	seedDocWithChunks(t, s, userID, "in-b.pdf", []string{"x"}, &folderB)

	d := newDispatcher(s, nil)
	tc := llm.ToolCall{ID: "1", Name: ToolListDocuments, Args: json.RawMessage(fmt.Sprintf(`{"folder_id":%q}`, folderA.String()))}

	result, _, _, err := d.Dispatch(ctx, &model.AgentSession{UserID: userID}, tc, "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	var out struct {
		Documents []listDocsItem `json:"documents"`
		Total     int            `json:"total"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, result)
	}
	if out.Total != 2 {
		t.Errorf("total = %d, want 2", out.Total)
	}
	for _, doc := range out.Documents {
		if !strings.HasPrefix(doc.Title, "in-a-") {
			t.Errorf("unexpected doc in folder-A listing: %q", doc.Title)
		}
	}
}

func TestListDocuments_PaginationCap(t *testing.T) {
	s := newTestStore(t)
	userID := uuid.New()
	// Seed one more than the cap so the clamp is observable: without it, all
	// listDocumentsMaxLimit+1 docs would come back.
	seedUserAndDocs(t, s, userID, listDocumentsMaxLimit+1, []float32{1, 0, 0, 0}, nil)

	d := newDispatcher(s, nil)
	tc := llm.ToolCall{ID: "1", Name: ToolListDocuments, Args: json.RawMessage(`{"limit":500}`)}

	result, _, _, err := d.Dispatch(context.Background(), &model.AgentSession{UserID: userID}, tc, "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if strings.Contains(result, `"error"`) {
		t.Fatalf("limit clamping should not surface as an error: %s", result)
	}
	var out struct {
		Documents []listDocsItem `json:"documents"`
		Total     int            `json:"total"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, result)
	}
	// limit:500 must clamp to listDocumentsMaxLimit, not return every seeded doc.
	// Delete the clamp in dispatchListDocuments and this assertion fails.
	if len(out.Documents) != listDocumentsMaxLimit {
		t.Errorf("returned %d documents, want exactly %d (clamped)", len(out.Documents), listDocumentsMaxLimit)
	}
	// total reflects the full matching count, independent of the page cap.
	if out.Total != listDocumentsMaxLimit+1 {
		t.Errorf("total = %d, want %d", out.Total, listDocumentsMaxLimit+1)
	}
}

// TestListDocuments_DefaultLimit covers the limit<=0 → default-page-size branch.
func TestListDocuments_DefaultLimit(t *testing.T) {
	s := newTestStore(t)
	userID := uuid.New()
	seedUserAndDocs(t, s, userID, listDocumentsDefaultLimit+5, []float32{1, 0, 0, 0}, nil)

	d := newDispatcher(s, nil)
	// No limit arg → must fall back to listDocumentsDefaultLimit, not return all.
	tc := llm.ToolCall{ID: "1", Name: ToolListDocuments, Args: json.RawMessage(`{}`)}

	result, _, _, err := d.Dispatch(context.Background(), &model.AgentSession{UserID: userID}, tc, "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	var out struct {
		Documents []listDocsItem `json:"documents"`
		Total     int            `json:"total"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, result)
	}
	if len(out.Documents) != listDocumentsDefaultLimit {
		t.Errorf("returned %d documents, want exactly %d (default)", len(out.Documents), listDocumentsDefaultLimit)
	}
}

// ---------- get_chunk_context ----------

func TestGetChunkContext_WindowExpands(t *testing.T) {
	s := newTestStore(t)
	userID := uuid.New()
	createUser(t, s, userID)

	contents := make([]string, 10)
	for i := range contents {
		contents[i] = fmt.Sprintf("body-%d", i)
	}
	_, chunkIDs := seedDocWithChunks(t, s, userID, "long.pdf", contents, nil)

	d := newDispatcher(s, nil)
	tc := llm.ToolCall{ID: "1", Name: ToolGetChunkContext, Args: json.RawMessage(fmt.Sprintf(`{"chunk_id":%q,"window":2}`, chunkIDs[5].String()))}

	result, citations, _, err := d.Dispatch(context.Background(), &model.AgentSession{UserID: userID}, tc, "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	var out struct {
		Chunks []chunkContextItem `json:"chunks"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, result)
	}
	wantIndices := []int{3, 4, 5, 6, 7}
	if len(out.Chunks) != len(wantIndices) {
		t.Fatalf("chunks = %d, want %d", len(out.Chunks), len(wantIndices))
	}
	for i, want := range wantIndices {
		if out.Chunks[i].ChunkIndex != want {
			t.Errorf("chunks[%d].ChunkIndex = %d, want %d", i, out.Chunks[i].ChunkIndex, want)
		}
	}
	if len(citations) != len(wantIndices) {
		t.Errorf("citations = %d, want %d", len(citations), len(wantIndices))
	}
}

func TestGetChunkContext_BoundaryClamps(t *testing.T) {
	s := newTestStore(t)
	userID := uuid.New()
	createUser(t, s, userID)

	contents := []string{"a", "b", "c", "d"}
	_, chunkIDs := seedDocWithChunks(t, s, userID, "short.pdf", contents, nil)

	d := newDispatcher(s, nil)
	// Hit the first chunk with an oversize window — should not panic, no negative indices.
	tc := llm.ToolCall{ID: "1", Name: ToolGetChunkContext, Args: json.RawMessage(fmt.Sprintf(`{"chunk_id":%q,"window":50}`, chunkIDs[0].String()))}

	result, _, _, err := d.Dispatch(context.Background(), &model.AgentSession{UserID: userID}, tc, "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	var out struct {
		Chunks []chunkContextItem `json:"chunks"`
	}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Chunks) != len(contents) {
		t.Fatalf("chunks = %d, want %d", len(out.Chunks), len(contents))
	}
	if out.Chunks[0].ChunkIndex != 0 {
		t.Errorf("first chunk index = %d, want 0", out.Chunks[0].ChunkIndex)
	}
}

func TestGetChunkContext_WrongUser(t *testing.T) {
	s := newTestStore(t)
	owner := uuid.New()
	intruder := uuid.New()
	createUser(t, s, owner)
	createUser(t, s, intruder)
	_, chunkIDs := seedDocWithChunks(t, s, owner, "private.pdf", []string{"x", "y", "z"}, nil)

	d := newDispatcher(s, nil)
	tc := llm.ToolCall{ID: "1", Name: ToolGetChunkContext, Args: json.RawMessage(fmt.Sprintf(`{"chunk_id":%q}`, chunkIDs[0].String()))}

	result, _, _, err := d.Dispatch(context.Background(), &model.AgentSession{UserID: intruder}, tc, "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !strings.Contains(result, `"error"`) {
		t.Errorf("expected error for cross-user access, got %s", result)
	}
}

// ---------- unknown tool ----------

func TestUnknownTool_ReturnsError(t *testing.T) {
	s := newTestStore(t)
	userID := uuid.New()
	createUser(t, s, userID)
	d := newDispatcher(s, nil)

	tc := llm.ToolCall{ID: "1", Name: "no_such_tool", Args: json.RawMessage(`{}`)}
	result, _, _, err := d.Dispatch(context.Background(), &model.AgentSession{UserID: userID}, tc, "")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !strings.Contains(result, "unknown tool") {
		t.Errorf("expected unknown-tool error, got %s", result)
	}
}

// Specs sanity check — five tools, distinct names.
func TestDispatcher_SpecsContainAllTools(t *testing.T) {
	d := newDispatcher(newTestStore(t), nil)
	specs := d.Specs()
	want := map[string]bool{
		ToolSearchDocuments:   false,
		ToolGetDocument:       false,
		ToolSummarizeDocument: false,
		ToolListDocuments:     false,
		ToolGetChunkContext:   false,
	}
	for _, s := range specs {
		if _, ok := want[s.Name]; !ok {
			t.Errorf("unexpected tool advertised: %s", s.Name)
			continue
		}
		want[s.Name] = true
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("tool %s missing from Specs()", name)
		}
	}
}
