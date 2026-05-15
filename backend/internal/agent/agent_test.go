package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/docinsight/backend/internal/events"
	"github.com/docinsight/backend/internal/llm"
	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/store"
	"github.com/google/uuid"
)

// mockEmbedder returns the same fixed embedding for any input.
type mockEmbedder struct {
	vec []float32
}

func (m *mockEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = m.vec
	}
	return out, nil
}

func (m *mockEmbedder) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	return m.vec, nil
}

// scriptedLLM replays canned event sequences in order, one batch per StreamChat call.
type scriptedLLM struct {
	scripts [][]llm.StreamEvent
	calls   int
}

func (s *scriptedLLM) StreamChat(ctx context.Context, apiKey, modelName, system string, messages []llm.Message, tools []llm.Tool) (<-chan llm.StreamEvent, error) {
	if s.calls >= len(s.scripts) {
		return nil, fmt.Errorf("no more scripted responses")
	}
	events := s.scripts[s.calls]
	s.calls++
	ch := make(chan llm.StreamEvent, len(events))
	go func() {
		defer close(ch)
		for _, ev := range events {
			ch <- ev
		}
	}()
	return ch, nil
}

func newTestStore(t *testing.T) store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "agent_test.db")
	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seedUserAndDocs creates a user and N documents (with one chunk + embedding each).
// chunkIDs is returned in order of creation.
func seedUserAndDocs(t *testing.T, s store.Store, userID uuid.UUID, count int, embedding []float32, folderID *uuid.UUID) (docIDs, chunkIDs []uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	// Create user
	user := &model.User{ID: userID, Email: fmt.Sprintf("u-%s@example.com", userID.String()[:8]), APIKey: "di_" + userID.String(), Name: "user"}
	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	for i := 0; i < count; i++ {
		docID := uuid.New()
		doc := &model.Document{
			ID:       docID,
			Name:     fmt.Sprintf("doc-%d.pdf", i),
			FilePath: fmt.Sprintf("/tmp/doc-%d.pdf", i),
			FileSize: 100,
			Status:   model.StatusCompleted,
			UserID:   &userID,
			FolderID: folderID,
		}
		if err := s.InsertDocument(ctx, doc, &userID); err != nil {
			t.Fatalf("InsertDocument: %v", err)
		}
		chunkID := uuid.New()
		chunk := model.Chunk{ID: chunkID, DocumentID: docID, Content: fmt.Sprintf("content of doc %d about topic", i), PageNumber: 1, ChunkIndex: 0}
		if _, err := s.InsertChunks(ctx, []model.Chunk{chunk}); err != nil {
			t.Fatalf("InsertChunks: %v", err)
		}
		if err := s.InsertEmbeddings(ctx, []uuid.UUID{chunkID}, [][]float32{embedding}); err != nil {
			t.Fatalf("InsertEmbeddings: %v", err)
		}
		docIDs = append(docIDs, docID)
		chunkIDs = append(chunkIDs, chunkID)
	}
	return
}

func newSession(userID uuid.UUID, folderID *uuid.UUID) *model.AgentSession {
	return &model.AgentSession{
		ID:       uuid.New(),
		UserID:   userID,
		FolderID: folderID,
		Provider: "anthropic",
		Model:    "claude-test",
	}
}

func TestRun_NoToolCall(t *testing.T) {
	s := newTestStore(t)
	userID := uuid.New()
	seedUserAndDocs(t, s, userID, 1, []float32{1, 0, 0, 0}, nil)

	sess := newSession(userID, nil)
	if err := s.CreateAgentSession(context.Background(), sess); err != nil {
		t.Fatalf("CreateAgentSession: %v", err)
	}

	llmMock := &scriptedLLM{
		scripts: [][]llm.StreamEvent{
			{
				{Type: llm.StreamText, Text: "Hello, no search needed."},
				{Type: llm.StreamDone},
			},
		},
	}

	a := &Agent{
		Store:    s,
		Embedder: &mockEmbedder{vec: []float32{1, 0, 0, 0}},
		Broker:   events.NewBroker(),
		LLM:      llmMock,
	}

	msg, err := a.Run(context.Background(), RunInput{Session: sess, UserMessage: "hi", APIKey: "k"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if msg.Role != "assistant" {
		t.Errorf("role = %q, want assistant", msg.Role)
	}
	if msg.Content != "Hello, no search needed." {
		t.Errorf("content = %q", msg.Content)
	}
	if llmMock.calls != 1 {
		t.Errorf("llm calls = %d, want 1", llmMock.calls)
	}
}

func TestRun_WithToolCall(t *testing.T) {
	s := newTestStore(t)
	userID := uuid.New()
	_, chunkIDs := seedUserAndDocs(t, s, userID, 1, []float32{1, 0, 0, 0}, nil)

	sess := newSession(userID, nil)
	if err := s.CreateAgentSession(context.Background(), sess); err != nil {
		t.Fatalf("CreateAgentSession: %v", err)
	}

	llmMock := &scriptedLLM{
		scripts: [][]llm.StreamEvent{
			// First call: tool use
			{
				{Type: llm.StreamToolCall, ToolCall: &llm.ToolCall{
					ID: "t1", Name: SearchToolName, Args: json.RawMessage(`{"query":"topic","top_k":3}`),
				}},
				{Type: llm.StreamDone},
			},
			// Second call: final answer citing the chunk
			{
				{Type: llm.StreamText, Text: fmt.Sprintf(`Based on the docs <cite chunk="%s"/>`, chunkIDs[0].String())},
				{Type: llm.StreamDone},
			},
		},
	}

	a := &Agent{
		Store:    s,
		Embedder: &mockEmbedder{vec: []float32{1, 0, 0, 0}},
		Broker:   events.NewBroker(),
		LLM:      llmMock,
	}

	msg, err := a.Run(context.Background(), RunInput{Session: sess, UserMessage: "tell me about topic", APIKey: "k"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if llmMock.calls != 2 {
		t.Errorf("llm calls = %d, want 2", llmMock.calls)
	}
	if len(msg.Citations) != 1 {
		t.Fatalf("citations = %d, want 1", len(msg.Citations))
	}
	if msg.Citations[0].ChunkID != chunkIDs[0] {
		t.Errorf("citation chunk id mismatch")
	}
}

func TestRun_FolderScoping(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := uuid.New()

	// Create a folder; only docs in the folder should be searchable.
	user := &model.User{ID: userID, Email: "folder@example.com", APIKey: "di_folder", Name: "folder user"}
	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	folderID := uuid.New()
	if err := s.CreateFolder(ctx, &model.Folder{ID: folderID, UserID: &userID, Name: "scoped"}); err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}

	emb := []float32{1, 0, 0, 0}

	// Doc inside folder
	insideDocID := uuid.New()
	insideChunkID := uuid.New()
	if err := s.InsertDocument(ctx, &model.Document{ID: insideDocID, Name: "in.pdf", FilePath: "/tmp/in.pdf", FileSize: 1, Status: model.StatusCompleted, UserID: &userID, FolderID: &folderID}, &userID); err != nil {
		t.Fatalf("InsertDocument: %v", err)
	}
	if _, err := s.InsertChunks(ctx, []model.Chunk{{ID: insideChunkID, DocumentID: insideDocID, Content: "inside the folder", PageNumber: 1}}); err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}
	if err := s.InsertEmbeddings(ctx, []uuid.UUID{insideChunkID}, [][]float32{emb}); err != nil {
		t.Fatalf("InsertEmbeddings: %v", err)
	}

	// Doc outside folder
	outsideDocID := uuid.New()
	outsideChunkID := uuid.New()
	if err := s.InsertDocument(ctx, &model.Document{ID: outsideDocID, Name: "out.pdf", FilePath: "/tmp/out.pdf", FileSize: 1, Status: model.StatusCompleted, UserID: &userID}, &userID); err != nil {
		t.Fatalf("InsertDocument outside: %v", err)
	}
	if _, err := s.InsertChunks(ctx, []model.Chunk{{ID: outsideChunkID, DocumentID: outsideDocID, Content: "outside the folder", PageNumber: 1}}); err != nil {
		t.Fatalf("InsertChunks outside: %v", err)
	}
	if err := s.InsertEmbeddings(ctx, []uuid.UUID{outsideChunkID}, [][]float32{emb}); err != nil {
		t.Fatalf("InsertEmbeddings outside: %v", err)
	}

	sess := newSession(userID, &folderID)
	if err := s.CreateAgentSession(ctx, sess); err != nil {
		t.Fatalf("CreateAgentSession: %v", err)
	}

	// Capture the tool-result content sent to the LLM in the second invocation.
	llmMock := &scriptedLLM{
		scripts: [][]llm.StreamEvent{
			{
				{Type: llm.StreamToolCall, ToolCall: &llm.ToolCall{ID: "t1", Name: SearchToolName, Args: json.RawMessage(`{"query":"folder"}`)}},
				{Type: llm.StreamDone},
			},
			{
				{Type: llm.StreamText, Text: fmt.Sprintf(`Only inside <cite chunk="%s"/>`, insideChunkID.String())},
				{Type: llm.StreamDone},
			},
		},
	}

	a := &Agent{Store: s, Embedder: &mockEmbedder{vec: emb}, Broker: events.NewBroker(), LLM: llmMock}
	msg, err := a.Run(ctx, RunInput{Session: sess, UserMessage: "scoped?", APIKey: "k"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(msg.Citations) != 1 || msg.Citations[0].ChunkID != insideChunkID {
		t.Fatalf("expected only the in-folder chunk in citations, got %+v", msg.Citations)
	}
	// Defence in depth: explicitly assert the outside chunk wasn't returned even if cited.
	for _, c := range msg.Citations {
		if c.ChunkID == outsideChunkID {
			t.Errorf("outside-folder chunk leaked into citations")
		}
	}
}

func TestRun_CitationExtraction(t *testing.T) {
	s := newTestStore(t)
	userID := uuid.New()
	_, chunkIDs := seedUserAndDocs(t, s, userID, 2, []float32{1, 0, 0, 0}, nil)

	sess := newSession(userID, nil)
	if err := s.CreateAgentSession(context.Background(), sess); err != nil {
		t.Fatalf("CreateAgentSession: %v", err)
	}

	// One chunk cited twice (dedup), one cited once, one not cited.
	finalText := fmt.Sprintf(
		`First <cite chunk="%s"/> and again <cite chunk="%s"/> but also <cite chunk="%s"/>`,
		chunkIDs[0], chunkIDs[0], chunkIDs[1],
	)

	llmMock := &scriptedLLM{
		scripts: [][]llm.StreamEvent{
			{
				{Type: llm.StreamToolCall, ToolCall: &llm.ToolCall{ID: "t1", Name: SearchToolName, Args: json.RawMessage(`{"query":"x"}`)}},
				{Type: llm.StreamDone},
			},
			{
				{Type: llm.StreamText, Text: finalText},
				{Type: llm.StreamDone},
			},
		},
	}

	a := &Agent{Store: s, Embedder: &mockEmbedder{vec: []float32{1, 0, 0, 0}}, Broker: events.NewBroker(), LLM: llmMock}
	msg, err := a.Run(context.Background(), RunInput{Session: sess, UserMessage: "tell", APIKey: "k"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(msg.Citations) != 2 {
		t.Fatalf("citations = %d, want 2 (deduped)", len(msg.Citations))
	}
	// Order should follow first-mention.
	if msg.Citations[0].ChunkID != chunkIDs[0] || msg.Citations[1].ChunkID != chunkIDs[1] {
		t.Errorf("citation order = [%s %s], want [%s %s]", msg.Citations[0].ChunkID, msg.Citations[1].ChunkID, chunkIDs[0], chunkIDs[1])
	}
}

func TestAgent_MultiToolFlow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := uuid.New()

	// Seed a user and a multi-chunk document so search → context expansion has
	// real data to walk through.
	user := &model.User{ID: userID, Email: "multi@example.com", APIKey: "di_multi", Name: "multi user"}
	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	docID := uuid.New()
	if err := s.InsertDocument(ctx, &model.Document{
		ID: docID, Name: "research.pdf", FilePath: "/tmp/research.pdf", FileSize: 100,
		Status: model.StatusCompleted, UserID: &userID,
	}, &userID); err != nil {
		t.Fatalf("InsertDocument: %v", err)
	}
	emb := []float32{1, 0, 0, 0}
	chunks := make([]model.Chunk, 5)
	chunkIDs := make([]uuid.UUID, 5)
	for i := range chunks {
		chunkIDs[i] = uuid.New()
		chunks[i] = model.Chunk{
			ID:         chunkIDs[i],
			DocumentID: docID,
			Content:    fmt.Sprintf("body %d about widgets", i),
			PageNumber: 1,
			ChunkIndex: i,
		}
	}
	if _, err := s.InsertChunks(ctx, chunks); err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}
	embs := make([][]float32, 5)
	for i := range embs {
		embs[i] = emb
	}
	if err := s.InsertEmbeddings(ctx, chunkIDs, embs); err != nil {
		t.Fatalf("InsertEmbeddings: %v", err)
	}

	sess := newSession(userID, nil)
	if err := s.CreateAgentSession(ctx, sess); err != nil {
		t.Fatalf("CreateAgentSession: %v", err)
	}

	// Hit chunk index 2 — get_chunk_context (default window 3) will return
	// indices 0..4. We cite chunkIDs[2] in the final answer.
	hit := chunkIDs[2]

	llmMock := &scriptedLLM{
		scripts: [][]llm.StreamEvent{
			// 1) Main loop iter 1: ask for search_documents.
			{
				{Type: llm.StreamToolCall, ToolCall: &llm.ToolCall{
					ID: "t-search", Name: ToolSearchDocuments, Args: json.RawMessage(`{"query":"widgets"}`),
				}},
				{Type: llm.StreamDone},
			},
			// 2) Main loop iter 2: ask for get_chunk_context on the focal chunk.
			{
				{Type: llm.StreamToolCall, ToolCall: &llm.ToolCall{
					ID: "t-ctx", Name: ToolGetChunkContext, Args: json.RawMessage(fmt.Sprintf(`{"chunk_id":%q}`, hit.String())),
				}},
				{Type: llm.StreamDone},
			},
			// 3) Main loop iter 3: ask for summarize_document.
			{
				{Type: llm.StreamToolCall, ToolCall: &llm.ToolCall{
					ID: "t-sum", Name: ToolSummarizeDocument, Args: json.RawMessage(fmt.Sprintf(`{"document_id":%q,"length":"short"}`, docID.String())),
				}},
				{Type: llm.StreamDone},
			},
			// 4) Nested LLM call from inside summarize_document. Returns the summary text.
			{
				{Type: llm.StreamText, Text: "A succinct summary."},
				{Type: llm.StreamDone},
			},
			// 5) Main loop iter 4: final answer citing the focal chunk.
			{
				{Type: llm.StreamText, Text: fmt.Sprintf(`Done. <cite chunk="%s"/>`, hit.String())},
				{Type: llm.StreamDone},
			},
		},
	}

	broker := events.NewBroker()
	clientID := "test-client"
	sub := broker.Subscribe(clientID)
	defer broker.Unsubscribe(clientID)

	a := &Agent{
		Store:    s,
		Embedder: &mockEmbedder{vec: emb},
		Broker:   broker,
		LLM:      llmMock,
	}
	msg, err := a.Run(ctx, RunInput{Session: sess, UserMessage: "tell me about widgets", APIKey: "user-key"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if llmMock.calls != 5 {
		t.Errorf("llm calls = %d, want 5 (3 main iters + 1 nested summarize + final answer)", llmMock.calls)
	}
	if len(msg.Citations) != 1 {
		t.Fatalf("citations = %d, want 1", len(msg.Citations))
	}
	if msg.Citations[0].ChunkID != hit {
		t.Errorf("citation chunk_id mismatch: got %s want %s", msg.Citations[0].ChunkID, hit)
	}

	// Drain published events so we can assert tool_call / tool_result fired
	// in the expected order. The broker is buffered; reading until we have
	// observed each expected event keeps the test deterministic.
	expectedToolCallNames := []string{ToolSearchDocuments, ToolGetChunkContext, ToolSummarizeDocument}
	gotToolCalls := []string{}
	gotToolResults := []string{}
	gotComplete := false

readLoop:
	for {
		select {
		case ev, ok := <-sub:
			if !ok {
				break readLoop
			}
			data, _ := ev.Data.(map[string]any)
			switch ev.Type {
			case "agent.tool_call":
				if name, _ := data["name"].(string); name != "" {
					gotToolCalls = append(gotToolCalls, name)
				}
			case "agent.tool_result":
				if name, _ := data["name"].(string); name != "" {
					gotToolResults = append(gotToolResults, name)
				}
			case "agent.complete":
				gotComplete = true
			case "agent.error":
				t.Fatalf("unexpected agent.error event: %+v", data)
			}
			if gotComplete {
				break readLoop
			}
		default:
			break readLoop
		}
	}

	if len(gotToolCalls) != len(expectedToolCallNames) {
		t.Errorf("tool_call events = %v, want %v", gotToolCalls, expectedToolCallNames)
	} else {
		for i, want := range expectedToolCallNames {
			if gotToolCalls[i] != want {
				t.Errorf("tool_call[%d] = %s, want %s", i, gotToolCalls[i], want)
			}
		}
	}
	if len(gotToolResults) != len(expectedToolCallNames) {
		t.Errorf("tool_result events = %v, want %v", gotToolResults, expectedToolCallNames)
	}
	if !gotComplete {
		t.Errorf("did not observe agent.complete event")
	}
}

func TestRun_MaxIterations(t *testing.T) {
	s := newTestStore(t)
	userID := uuid.New()
	seedUserAndDocs(t, s, userID, 1, []float32{1, 0, 0, 0}, nil)

	sess := newSession(userID, nil)
	if err := s.CreateAgentSession(context.Background(), sess); err != nil {
		t.Fatalf("CreateAgentSession: %v", err)
	}

	// All iterations: tool-call only, never a final answer.
	iter := []llm.StreamEvent{
		{Type: llm.StreamToolCall, ToolCall: &llm.ToolCall{ID: "t", Name: SearchToolName, Args: json.RawMessage(`{"query":"x"}`)}},
		{Type: llm.StreamDone},
	}
	scripts := make([][]llm.StreamEvent, MaxIterations+2)
	for i := range scripts {
		scripts[i] = iter
	}
	llmMock := &scriptedLLM{scripts: scripts}

	a := &Agent{Store: s, Embedder: &mockEmbedder{vec: []float32{1, 0, 0, 0}}, Broker: events.NewBroker(), LLM: llmMock}
	_, err := a.Run(context.Background(), RunInput{Session: sess, UserMessage: "tell", APIKey: "k"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if llmMock.calls != MaxIterations {
		t.Errorf("llm calls = %d, want %d", llmMock.calls, MaxIterations)
	}
}
