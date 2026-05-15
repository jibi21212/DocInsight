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

// OpenAIClient streams chats from OpenAI's chat completions API.
type OpenAIClient struct {
	BaseURL    string // defaults to https://api.openai.com when empty
	HTTPClient *http.Client
}

func (c *OpenAIClient) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return "https://api.openai.com"
}

func (c *OpenAIClient) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

type openaiToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openaiTool struct {
	Type     string             `json:"type"`
	Function openaiToolFunction `json:"function"`
}

type openaiToolCallReq struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openaiMessage struct {
	Role       string              `json:"role"`
	Content    string              `json:"content,omitempty"`
	ToolCalls  []openaiToolCallReq `json:"tool_calls,omitempty"`
	ToolCallID string              `json:"tool_call_id,omitempty"`
	Name       string              `json:"name,omitempty"`
}

type openaiRequest struct {
	Model    string          `json:"model"`
	Messages []openaiMessage `json:"messages"`
	Tools    []openaiTool    `json:"tools,omitempty"`
	Stream   bool            `json:"stream"`
}

func convertMessagesToOpenAI(system string, messages []Message) []openaiMessage {
	out := make([]openaiMessage, 0, len(messages)+1)
	if system != "" {
		out = append(out, openaiMessage{Role: "system", Content: system})
	}
	for _, m := range messages {
		switch m.Role {
		case "tool":
			out = append(out, openaiMessage{
				Role:       "tool",
				Content:    m.Content,
				ToolCallID: m.ToolCallID,
			})
		case "assistant":
			om := openaiMessage{Role: "assistant", Content: m.Content}
			for _, tc := range m.ToolCalls {
				args := string(tc.Args)
				if args == "" {
					args = "{}"
				}
				call := openaiToolCallReq{ID: tc.ID, Type: "function"}
				call.Function.Name = tc.Name
				call.Function.Arguments = args
				om.ToolCalls = append(om.ToolCalls, call)
			}
			out = append(out, om)
		case "system":
			// Already handled above (or override).
			out = append(out, openaiMessage{Role: "system", Content: m.Content})
		default:
			out = append(out, openaiMessage{Role: "user", Content: m.Content})
		}
	}
	return out
}

// StreamChat implements the Client interface.
func (c *OpenAIClient) StreamChat(ctx context.Context, apiKey, modelName, system string, messages []Message, tools []Tool) (<-chan StreamEvent, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("openai: api key required")
	}

	oaTools := make([]openaiTool, len(tools))
	for i, t := range tools {
		oaTools[i] = openaiTool{
			Type: "function",
			Function: openaiToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		}
	}

	body := openaiRequest{
		Model:    modelName,
		Messages: convertMessagesToOpenAI(system, messages),
		Tools:    oaTools,
		Stream:   true,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL()+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai returned %d: %s", resp.StatusCode, string(errBody))
	}

	out := make(chan StreamEvent, 16)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		parseOpenAISSE(resp.Body, out)
	}()
	return out, nil
}

// parseOpenAISSE handles the OpenAI streaming format. Each SSE record is
// "data: {json}" with a final "data: [DONE]". Tool calls arrive as deltas
// indexed by `index` and must be accumulated.
func parseOpenAISSE(body io.Reader, out chan<- StreamEvent) {
	type partialTool struct {
		id   string
		name string
		args bytes.Buffer
	}
	tools := make(map[int]*partialTool)
	toolOrder := []int{}

	flushTools := func() {
		for _, idx := range toolOrder {
			pt, ok := tools[idx]
			if !ok {
				continue
			}
			args := pt.args.Bytes()
			if len(args) == 0 {
				args = []byte("{}")
			}
			argsCopy := make([]byte, len(args))
			copy(argsCopy, args)
			out <- StreamEvent{
				Type: StreamToolCall,
				ToolCall: &ToolCall{
					ID:   pt.id,
					Name: pt.name,
					Args: argsCopy,
				},
			}
		}
	}

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
		if payload == "[DONE]" {
			flushTools()
			out <- StreamEvent{Type: StreamDone}
			return
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		if delta.Content != "" {
			out <- StreamEvent{Type: StreamText, Text: delta.Content}
		}
		for _, tc := range delta.ToolCalls {
			pt, ok := tools[tc.Index]
			if !ok {
				pt = &partialTool{}
				tools[tc.Index] = pt
				toolOrder = append(toolOrder, tc.Index)
			}
			if tc.ID != "" {
				pt.id = tc.ID
			}
			if tc.Function.Name != "" {
				pt.name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				pt.args.WriteString(tc.Function.Arguments)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		out <- StreamEvent{Type: StreamError, Error: err.Error()}
		return
	}
	flushTools()
	out <- StreamEvent{Type: StreamDone}
}
