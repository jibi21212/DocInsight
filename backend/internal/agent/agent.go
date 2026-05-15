package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/docinsight/backend/internal/embedder"
	"github.com/docinsight/backend/internal/events"
	"github.com/docinsight/backend/internal/llm"
	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/store"
	"github.com/google/uuid"
)

// SystemPrompt is the grounding prompt sent to every chat session.
const SystemPrompt = `You are a research assistant grounded in the user's document library.

Rules:
1. ALWAYS call search_documents at least once before answering any factual question.
2. When you cite information from a search result, wrap it in <cite chunk="CHUNK_ID"/> markers using the exact chunk_id from the tool result.
3. Be concise. If the search returns no relevant results, say so plainly.`

// MaxIterations is the maximum number of LLM round-trips per request.
const MaxIterations = 5

// SnippetMaxLen caps the size of each citation snippet (chars).
const SnippetMaxLen = 240

// SearchToolName is the canonical search tool name advertised to the LLM.
const SearchToolName = "search_documents"

// searchToolSchema describes the search_documents tool input.
var searchToolSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {"type": "string", "description": "The search query."},
    "top_k": {"type": "integer", "description": "Number of results to return.", "default": 5}
  },
  "required": ["query"]
}`)

// Agent orchestrates a single chat turn against an LLM with tool use.
type Agent struct {
	Store    store.Store
	Embedder embedder.Embedder
	Broker   *events.Broker
	LLM      llm.Client // override for tests; otherwise picked from session.Provider
}

// RunInput is everything required to run a single chat turn.
type RunInput struct {
	Session     *model.AgentSession
	UserMessage string
	APIKey      string
	History     []model.AgentMessage // existing messages, oldest first
}

var citePattern = regexp.MustCompile(`<cite\s+chunk="([0-9a-fA-F-]+)"\s*/?>`)

// Run executes one user-message → assistant-message round trip. It streams
// agent.delta events via the broker, runs tool calls as needed, persists the
// user/assistant messages on success, and returns the final assistant message.
func (a *Agent) Run(ctx context.Context, in RunInput) (*model.AgentMessage, error) {
	if a.Store == nil {
		return nil, fmt.Errorf("agent: store required")
	}
	if a.Embedder == nil {
		return nil, fmt.Errorf("agent: embedder required")
	}
	if in.Session == nil {
		return nil, fmt.Errorf("agent: session required")
	}

	sessionIDStr := in.Session.ID.String()

	publish := func(eventType string, data map[string]any) {
		if a.Broker == nil {
			return
		}
		if data == nil {
			data = map[string]any{}
		}
		data["session_id"] = sessionIDStr
		a.Broker.Publish(events.Event{Type: eventType, Data: data})
	}

	// Persist user message immediately.
	userMsg := &model.AgentMessage{
		ID:        uuid.New(),
		SessionID: in.Session.ID,
		Role:      "user",
		Content:   in.UserMessage,
		CreatedAt: time.Now().UTC(),
	}
	if err := a.Store.InsertAgentMessage(ctx, userMsg); err != nil {
		publish("agent.error", map[string]any{"error": err.Error()})
		return nil, fmt.Errorf("insert user message: %w", err)
	}

	// Pick client.
	client := a.LLM
	if client == nil {
		c, err := llm.NewClient(llm.Provider(in.Session.Provider))
		if err != nil {
			publish("agent.error", map[string]any{"error": err.Error()})
			return nil, err
		}
		client = c
	}

	tools := []llm.Tool{{
		Name:        SearchToolName,
		Description: "Search the user's document library and return the most relevant chunks.",
		InputSchema: searchToolSchema,
	}}

	// Build LLM message list from history + user message.
	llmMessages := make([]llm.Message, 0, len(in.History)+1)
	for _, h := range in.History {
		// "tool" role history needs the matching ToolCallID; we stored none, so
		// only feed back user/assistant roles for prior turns.
		if h.Role != "user" && h.Role != "assistant" {
			continue
		}
		llmMessages = append(llmMessages, llm.Message{Role: h.Role, Content: h.Content})
	}
	llmMessages = append(llmMessages, llm.Message{Role: "user", Content: in.UserMessage})

	// Citations accumulated across all tool calls in this run, keyed by chunk_id.
	citationMap := make(map[uuid.UUID]model.Citation)

	var finalText string

	for iter := 0; iter < MaxIterations; iter++ {
		stream, err := client.StreamChat(ctx, in.APIKey, in.Session.Model, SystemPrompt, llmMessages, tools)
		if err != nil {
			publish("agent.error", map[string]any{"error": err.Error()})
			return nil, fmt.Errorf("stream chat: %w", err)
		}

		var iterText string
		var iterToolCalls []llm.ToolCall

		for ev := range stream {
			switch ev.Type {
			case llm.StreamText:
				iterText += ev.Text
				publish("agent.delta", map[string]any{"text": ev.Text})
			case llm.StreamToolCall:
				if ev.ToolCall != nil {
					iterToolCalls = append(iterToolCalls, *ev.ToolCall)
				}
			case llm.StreamError:
				publish("agent.error", map[string]any{"error": ev.Error})
				return nil, fmt.Errorf("llm stream error: %s", ev.Error)
			case llm.StreamDone:
				// fall through after loop
			}
		}

		if len(iterToolCalls) == 0 {
			finalText = iterText
			break
		}

		// Append the assistant message with tool_calls.
		llmMessages = append(llmMessages, llm.Message{
			Role:      "assistant",
			Content:   iterText,
			ToolCalls: iterToolCalls,
		})

		// Execute each tool call.
		for _, tc := range iterToolCalls {
			if tc.Name != SearchToolName {
				toolResult := fmt.Sprintf(`{"error":"unknown tool: %s"}`, tc.Name)
				llmMessages = append(llmMessages, llm.Message{
					Role:       "tool",
					Content:    toolResult,
					ToolCallID: tc.ID,
				})
				continue
			}

			var args struct {
				Query string `json:"query"`
				TopK  int    `json:"top_k"`
			}
			if err := json.Unmarshal(tc.Args, &args); err != nil {
				toolResult := fmt.Sprintf(`{"error":"invalid args: %s"}`, err.Error())
				llmMessages = append(llmMessages, llm.Message{
					Role: "tool", Content: toolResult, ToolCallID: tc.ID,
				})
				continue
			}
			if args.TopK <= 0 {
				args.TopK = 5
			}

			publish("agent.tool_call", map[string]any{
				"name": tc.Name,
				"args": json.RawMessage(tc.Args),
			})

			results, citations, toolErr := a.runSearch(ctx, in.Session, args.Query, args.TopK)
			if toolErr != nil {
				toolResult := fmt.Sprintf(`{"error":"%s"}`, toolErr.Error())
				llmMessages = append(llmMessages, llm.Message{
					Role: "tool", Content: toolResult, ToolCallID: tc.ID,
				})
				continue
			}

			// Stash citations.
			for _, c := range citations {
				citationMap[c.ChunkID] = c
			}

			publish("agent.tool_result", map[string]any{
				"citations": citations,
			})

			payload, _ := json.Marshal(results)
			llmMessages = append(llmMessages, llm.Message{
				Role:       "tool",
				Content:    string(payload),
				ToolCallID: tc.ID,
			})
		}
		// Loop continues for next iteration.
	}

	// Extract chunk ids referenced in the final text.
	usedCitations := extractCitations(finalText, citationMap)

	assistantMsg := &model.AgentMessage{
		ID:        uuid.New(),
		SessionID: in.Session.ID,
		Role:      "assistant",
		Content:   finalText,
		Citations: usedCitations,
		CreatedAt: time.Now().UTC(),
	}
	if err := a.Store.InsertAgentMessage(ctx, assistantMsg); err != nil {
		publish("agent.error", map[string]any{"error": err.Error()})
		return nil, fmt.Errorf("insert assistant message: %w", err)
	}

	publish("agent.complete", map[string]any{
		"message_id": assistantMsg.ID.String(),
		"citations":  usedCitations,
	})

	return assistantMsg, nil
}

// searchResultItem is the JSON shape the LLM sees for each search hit.
type searchResultItem struct {
	ChunkID      string  `json:"chunk_id"`
	DocumentID   string  `json:"document_id"`
	DocumentName string  `json:"document_name"`
	Snippet      string  `json:"snippet"`
	PageNumber   int     `json:"page_number"`
	Score        float64 `json:"score"`
}

func (a *Agent) runSearch(ctx context.Context, session *model.AgentSession, query string, topK int) ([]searchResultItem, []model.Citation, error) {
	embs, err := a.Embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, nil, fmt.Errorf("embed: %w", err)
	}
	if len(embs) == 0 {
		return nil, nil, fmt.Errorf("empty embedding")
	}

	results, err := a.Store.HybridSearch(ctx, embs[0], query, 0.0, topK, nil, &session.UserID, session.FolderID)
	if err != nil {
		return nil, nil, fmt.Errorf("search: %w", err)
	}

	items := make([]searchResultItem, 0, len(results))
	citations := make([]model.Citation, 0, len(results))
	for _, r := range results {
		snippet := r.Content
		if len(snippet) > SnippetMaxLen {
			snippet = snippet[:SnippetMaxLen]
		}
		items = append(items, searchResultItem{
			ChunkID:      r.ChunkID.String(),
			DocumentID:   r.DocumentID.String(),
			DocumentName: r.DocumentName,
			Snippet:      snippet,
			PageNumber:   r.PageNumber,
			Score:        r.Similarity,
		})
		citations = append(citations, model.Citation{
			ChunkID:      r.ChunkID,
			DocumentID:   r.DocumentID,
			DocumentName: r.DocumentName,
			Snippet:      snippet,
			PageNumber:   r.PageNumber,
			Score:        r.Similarity,
		})
	}
	return items, citations, nil
}

// extractCitations returns citations whose chunk_id appears in <cite chunk=".../> markers.
func extractCitations(text string, available map[uuid.UUID]model.Citation) []model.Citation {
	matches := citePattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[uuid.UUID]bool)
	var out []model.Citation
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		id, err := uuid.Parse(m[1])
		if err != nil {
			continue
		}
		if seen[id] {
			continue
		}
		if c, ok := available[id]; ok {
			out = append(out, c)
			seen[id] = true
		}
	}
	return out
}
