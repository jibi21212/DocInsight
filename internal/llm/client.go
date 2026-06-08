package llm

import (
	"context"
	"encoding/json"
	"errors"
)

// Provider identifies an LLM provider.
type Provider string

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
)

// Message is a chat message in a conversation passed to an LLM.
type Message struct {
	Role       string     `json:"role"` // user|assistant|tool|system
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // for role=tool
}

// ToolCall is a single function/tool invocation requested by the model.
type ToolCall struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// Tool describes a function the LLM may call.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// StreamEventType is the kind of event emitted during a chat stream.
type StreamEventType string

const (
	StreamText     StreamEventType = "text"
	StreamToolCall StreamEventType = "tool_call"
	StreamDone     StreamEventType = "done"
	StreamError    StreamEventType = "error"
)

// StreamEvent is a single event during a streaming chat response.
type StreamEvent struct {
	Type     StreamEventType
	Text     string
	ToolCall *ToolCall
	Error    string
}

// Client is the LLM provider interface.
type Client interface {
	StreamChat(ctx context.Context, apiKey, model string, system string, messages []Message, tools []Tool) (<-chan StreamEvent, error)
}

// NewClient returns a default client for the requested provider.
func NewClient(p Provider) (Client, error) {
	switch p {
	case ProviderAnthropic:
		return &AnthropicClient{}, nil
	case ProviderOpenAI:
		return &OpenAIClient{}, nil
	default:
		return nil, errors.New("unknown provider")
	}
}
