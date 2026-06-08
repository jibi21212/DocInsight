package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func startAnthropicServer(t *testing.T, sseBody string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("x-api-key") == "" {
			http.Error(w, "missing api key", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAnthropic_StreamText(t *testing.T) {
	sse := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")

	srv := startAnthropicServer(t, sse)

	c := &AnthropicClient{BaseURL: srv.URL}
	stream, err := c.StreamChat(context.Background(), "k", "claude-test", "sys", []Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}

	var texts []string
	var sawDone bool
	for ev := range stream {
		switch ev.Type {
		case StreamText:
			texts = append(texts, ev.Text)
		case StreamDone:
			sawDone = true
		case StreamError:
			t.Fatalf("unexpected error: %s", ev.Error)
		}
	}
	if len(texts) != 1 || texts[0] != "Hello" {
		t.Errorf("text events = %v, want [Hello]", texts)
	}
	if !sawDone {
		t.Error("expected StreamDone")
	}
}

func TestAnthropic_ToolCall(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"message_start"}`,
		``,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"search_documents"}}`,
		``,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"q"}}`,
		``,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"uery\":"}}`,
		``,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"x\"}"}}`,
		``,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")

	srv := startAnthropicServer(t, sse)

	c := &AnthropicClient{BaseURL: srv.URL}
	stream, err := c.StreamChat(context.Background(), "k", "claude-test", "sys", []Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}

	var toolCalls []ToolCall
	var sawDone bool
	for ev := range stream {
		switch ev.Type {
		case StreamToolCall:
			toolCalls = append(toolCalls, *ev.ToolCall)
		case StreamDone:
			sawDone = true
		case StreamError:
			t.Fatalf("unexpected error: %s", ev.Error)
		}
	}
	if !sawDone {
		t.Error("expected StreamDone")
	}
	if len(toolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(toolCalls))
	}
	tc := toolCalls[0]
	if tc.ID != "toolu_1" || tc.Name != "search_documents" {
		t.Errorf("tool call meta = %+v", tc)
	}
	if string(tc.Args) != `{"query":"x"}` {
		t.Errorf("tool call args = %q, want %q", string(tc.Args), `{"query":"x"}`)
	}
}
