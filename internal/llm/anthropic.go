package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AnthropicClient streams chats from the Anthropic Messages API.
type AnthropicClient struct {
	BaseURL    string // defaults to https://api.anthropic.com when empty
	HTTPClient *http.Client
}

const anthropicVersion = "2023-06-01"

func (c *AnthropicClient) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return "https://api.anthropic.com"
}

func (c *AnthropicClient) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

// anthropicMessage is the on-wire shape for Anthropic /v1/messages.
type anthropicMessage struct {
	Role    string                    `json:"role"`
	Content []anthropicMessageContent `json:"content"`
}

type anthropicMessageContent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []anthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	MaxTokens int                `json:"max_tokens"`
	Stream    bool               `json:"stream"`
}

// convertMessagesToAnthropic maps the generic message list to Anthropic format.
// role=tool becomes a user message with a tool_result block.
// role=assistant carrying tool_calls produces an assistant message with tool_use blocks.
func convertMessagesToAnthropic(messages []Message) []anthropicMessage {
	out := make([]anthropicMessage, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "tool":
			out = append(out, anthropicMessage{
				Role: "user",
				Content: []anthropicMessageContent{
					{
						Type:      "tool_result",
						ToolUseID: m.ToolCallID,
						Content:   m.Content,
					},
				},
			})
		case "assistant":
			content := []anthropicMessageContent{}
			if m.Content != "" {
				content = append(content, anthropicMessageContent{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				input := tc.Args
				if len(input) == 0 {
					input = json.RawMessage(`{}`)
				}
				content = append(content, anthropicMessageContent{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: input,
				})
			}
			if len(content) == 0 {
				content = append(content, anthropicMessageContent{Type: "text", Text: ""})
			}
			out = append(out, anthropicMessage{Role: "assistant", Content: content})
		default: // user, system handled separately at top level
			if m.Role == "system" {
				continue
			}
			out = append(out, anthropicMessage{
				Role:    "user",
				Content: []anthropicMessageContent{{Type: "text", Text: m.Content}},
			})
		}
	}
	return out
}

// StreamChat implements the Client interface.
func (c *AnthropicClient) StreamChat(ctx context.Context, apiKey, modelName, system string, messages []Message, tools []Tool) (<-chan StreamEvent, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic: api key required")
	}

	anthTools := make([]anthropicTool, len(tools))
	for i, t := range tools {
		anthTools[i] = anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}

	body := anthropicRequest{
		Model:     modelName,
		Messages:  convertMessagesToAnthropic(messages),
		System:    system,
		Tools:     anthTools,
		MaxTokens: 4096,
		Stream:    true,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL()+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic request: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic returned %d: %s", resp.StatusCode, string(errBody))
	}

	out := make(chan StreamEvent, 16)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		parseAnthropicSSE(resp.Body, out)
	}()
	return out, nil
}

// parseAnthropicSSE reads the SSE stream body and emits StreamEvents on out.
// Each SSE record is "data: {json}" with blank line separators. Anthropic emits
// event types as separate "event: ..." lines, but we identify events by the
// `type` field in the JSON payload (the canonical source).
func parseAnthropicSSE(body io.Reader, out chan<- StreamEvent) {
	type currentBlock struct {
		index    int
		kind     string // "text" or "tool_use"
		toolID   string
		toolName string
		jsonBuf  bytes.Buffer
	}

	blocks := make(map[int]*currentBlock)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
			continue
		}
		typeStr, _ := parsed["type"].(string)

		switch typeStr {
		case "content_block_start":
			idx := intFromAny(parsed["index"])
			cb, _ := parsed["content_block"].(map[string]interface{})
			kind, _ := cb["type"].(string)
			block := &currentBlock{index: idx, kind: kind}
			if kind == "tool_use" {
				block.toolID, _ = cb["id"].(string)
				block.toolName, _ = cb["name"].(string)
			}
			blocks[idx] = block

		case "content_block_delta":
			idx := intFromAny(parsed["index"])
			delta, _ := parsed["delta"].(map[string]interface{})
			deltaType, _ := delta["type"].(string)
			switch deltaType {
			case "text_delta":
				txt, _ := delta["text"].(string)
				if txt != "" {
					out <- StreamEvent{Type: StreamText, Text: txt}
				}
			case "input_json_delta":
				partial, _ := delta["partial_json"].(string)
				if b, ok := blocks[idx]; ok {
					b.jsonBuf.WriteString(partial)
				}
			}

		case "content_block_stop":
			idx := intFromAny(parsed["index"])
			if b, ok := blocks[idx]; ok {
				if b.kind == "tool_use" {
					args := b.jsonBuf.Bytes()
					if len(args) == 0 {
						args = []byte("{}")
					}
					// Make a copy so caller doesn't share the scratch buffer.
					argsCopy := make([]byte, len(args))
					copy(argsCopy, args)
					out <- StreamEvent{
						Type: StreamToolCall,
						ToolCall: &ToolCall{
							ID:   b.toolID,
							Name: b.toolName,
							Args: argsCopy,
						},
					}
				}
				delete(blocks, idx)
			}

		case "message_stop":
			out <- StreamEvent{Type: StreamDone}
			return

		case "error":
			errObj, _ := parsed["error"].(map[string]interface{})
			msg, _ := errObj["message"].(string)
			if msg == "" {
				msg = payload
			}
			out <- StreamEvent{Type: StreamError, Error: msg}
			return
		}
	}
	if err := scanner.Err(); err != nil {
		out <- StreamEvent{Type: StreamError, Error: err.Error()}
	}
}

func intFromAny(v interface{}) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	}
	return 0
}
