package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func startOpenAIServer(t *testing.T, sseBody string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestOpenAI_StreamText(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Hi"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":" there"}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	srv := startOpenAIServer(t, sse)
	c := &OpenAIClient{BaseURL: srv.URL}
	stream, err := c.StreamChat(context.Background(), "k", "gpt-test", "sys", []Message{{Role: "user", Content: "hi"}}, nil)
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
	if !sawDone {
		t.Error("expected StreamDone")
	}
	if len(texts) != 2 || texts[0] != "Hi" || texts[1] != " there" {
		t.Errorf("text events = %v", texts)
	}
}

func TestOpenAI_ToolCall(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"search_documents","arguments":"{\"qu"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ery\":\"x\"}"}}]}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	srv := startOpenAIServer(t, sse)
	c := &OpenAIClient{BaseURL: srv.URL}
	stream, err := c.StreamChat(context.Background(), "k", "gpt-test", "sys", []Message{{Role: "user", Content: "hi"}}, nil)
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
	if tc.ID != "call_1" || tc.Name != "search_documents" {
		t.Errorf("tool call meta = %+v", tc)
	}
	if string(tc.Args) != `{"query":"x"}` {
		t.Errorf("tool call args = %q, want %q", string(tc.Args), `{"query":"x"}`)
	}
}
