package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/docinsight/backend/internal/agent"
	"github.com/docinsight/backend/internal/events"
	"github.com/docinsight/backend/internal/llm"
	"github.com/docinsight/backend/internal/model"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// fakeRunner records the Run() input and returns a canned assistant message.
type fakeRunner struct {
	mu       sync.Mutex
	called   bool
	gotInput agent.RunInput
}

func (f *fakeRunner) Run(ctx context.Context, in agent.RunInput) (*model.AgentMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called = true
	f.gotInput = in
	return &model.AgentMessage{
		ID:        uuid.New(),
		SessionID: in.Session.ID,
		Role:      "assistant",
		Content:   "ok",
	}, nil
}

// withUserContext returns a request whose context contains the given user.
func withUserContext(t *testing.T, req *http.Request, user *model.User) *http.Request {
	t.Helper()
	ctx := context.WithValue(req.Context(), userContextKey, user)
	return req.WithContext(ctx)
}

func newAgentHandler(t *testing.T) (*AgentHandler, *model.User) {
	t.Helper()
	s := newTestStore(t)
	cfg := newTestConfig(t)
	broker := events.NewBroker()
	h := NewAgentHandler(s, &mockEmbedder{embedding: []float32{1, 0, 0, 0}}, broker, cfg)

	user := &model.User{ID: uuid.New(), Email: "agent@example.com", APIKey: "di_agent_test", Name: "agent user"}
	if err := s.CreateUser(context.Background(), user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return h, user
}

func TestCreateSession(t *testing.T) {
	h, user := newAgentHandler(t)

	body := bytes.NewBufferString(`{"provider":"anthropic","model":"claude-test","title":"Hi"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/sessions", body)
	req.Header.Set("Content-Type", "application/json")
	req = withUserContext(t, req, user)

	w := httptest.NewRecorder()
	h.CreateSession(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	sess, ok := resp["session"].(map[string]interface{})
	if !ok {
		t.Fatal("response missing session")
	}
	if sess["provider"] != "anthropic" {
		t.Errorf("provider = %v", sess["provider"])
	}
}

func TestCreateSession_BadProvider(t *testing.T) {
	h, user := newAgentHandler(t)
	body := bytes.NewBufferString(`{"provider":"nope","model":"x"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/sessions", body)
	req.Header.Set("Content-Type", "application/json")
	req = withUserContext(t, req, user)
	w := httptest.NewRecorder()
	h.CreateSession(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSendMessage_RequiresAPIKey(t *testing.T) {
	h, user := newAgentHandler(t)

	// Insert a session.
	sess := &model.AgentSession{ID: uuid.New(), UserID: user.ID, Provider: "anthropic", Model: "claude-test"}
	if err := h.store.CreateAgentSession(context.Background(), sess); err != nil {
		t.Fatalf("CreateAgentSession: %v", err)
	}

	body := bytes.NewBufferString(`{"content":"hi"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/sessions/"+sess.ID.String()+"/messages", body)
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sess.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = withUserContext(t, req, user)

	w := httptest.NewRecorder()
	h.SendMessage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSendMessage_Accepts(t *testing.T) {
	h, user := newAgentHandler(t)

	sess := &model.AgentSession{ID: uuid.New(), UserID: user.ID, Provider: "anthropic", Model: "claude-test"}
	if err := h.store.CreateAgentSession(context.Background(), sess); err != nil {
		t.Fatalf("CreateAgentSession: %v", err)
	}

	runner := &fakeRunner{}
	h.SetRunnerFactory(func(client llm.Client) AgentRunner { return runner })

	body := bytes.NewBufferString(`{"content":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/sessions/"+sess.ID.String()+"/messages", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-LLM-API-Key", "sk-test")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sess.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = withUserContext(t, req, user)

	w := httptest.NewRecorder()
	h.SendMessage(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	// Wait for the background goroutine.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runner.mu.Lock()
		ok := runner.called
		runner.mu.Unlock()
		if ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if !runner.called {
		t.Fatal("runner was not invoked")
	}
	if runner.gotInput.APIKey != "sk-test" {
		t.Errorf("api key = %q", runner.gotInput.APIKey)
	}
	if runner.gotInput.UserMessage != "hello" {
		t.Errorf("user message = %q", runner.gotInput.UserMessage)
	}
}

func TestListSessions_UserScoped(t *testing.T) {
	h, userA := newAgentHandler(t)

	userB := &model.User{ID: uuid.New(), Email: "b@example.com", APIKey: "di_b", Name: "B"}
	if err := h.store.CreateUser(context.Background(), userB); err != nil {
		t.Fatalf("CreateUser b: %v", err)
	}

	// Two sessions for A, one for B.
	for i := 0; i < 2; i++ {
		s := &model.AgentSession{ID: uuid.New(), UserID: userA.ID, Provider: "anthropic", Model: "claude-test"}
		if err := h.store.CreateAgentSession(context.Background(), s); err != nil {
			t.Fatal(err)
		}
	}
	bSess := &model.AgentSession{ID: uuid.New(), UserID: userB.ID, Provider: "openai", Model: "gpt-test"}
	if err := h.store.CreateAgentSession(context.Background(), bSess); err != nil {
		t.Fatal(err)
	}

	// User A should see only their two sessions.
	req := httptest.NewRequest(http.MethodGet, "/api/agent/sessions", nil)
	req = withUserContext(t, req, userA)
	w := httptest.NewRecorder()
	h.ListSessions(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	sessions, _ := resp["sessions"].([]interface{})
	if len(sessions) != 2 {
		t.Errorf("user A sessions = %d, want 2", len(sessions))
	}

	// User B should see only their one session.
	req2 := httptest.NewRequest(http.MethodGet, "/api/agent/sessions", nil)
	req2 = withUserContext(t, req2, userB)
	w2 := httptest.NewRecorder()
	h.ListSessions(w2, req2)
	json.NewDecoder(w2.Body).Decode(&resp)
	sessions, _ = resp["sessions"].([]interface{})
	if len(sessions) != 1 {
		t.Errorf("user B sessions = %d, want 1", len(sessions))
	}
}

func TestDeleteSession(t *testing.T) {
	h, user := newAgentHandler(t)
	sess := &model.AgentSession{ID: uuid.New(), UserID: user.ID, Provider: "anthropic", Model: "claude-test"}
	if err := h.store.CreateAgentSession(context.Background(), sess); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/agent/sessions/"+sess.ID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sess.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = withUserContext(t, req, user)

	w := httptest.NewRecorder()
	h.DeleteSession(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	got, _ := h.store.GetAgentSession(context.Background(), sess.ID, &user.ID)
	if got != nil {
		t.Error("session should be deleted")
	}
}
