package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/docinsight/backend/internal/agent"
	"github.com/docinsight/backend/internal/config"
	"github.com/docinsight/backend/internal/embedder"
	"github.com/docinsight/backend/internal/events"
	"github.com/docinsight/backend/internal/llm"
	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// AgentRunner is the minimal interface that handler.SendMessage uses to
// dispatch a chat turn. Tests can supply a mock implementation.
type AgentRunner interface {
	Run(ctx context.Context, in agent.RunInput) (*model.AgentMessage, error)
}

// AgentHandler exposes BYO-LLM chat session endpoints.
type AgentHandler struct {
	store    store.Store
	embedder embedder.Embedder
	broker   *events.Broker
	cfg      *config.Config

	// runnerFactory is used to construct the per-request AgentRunner. Tests
	// can inject a stub by replacing this field. The llm.Client argument may
	// be nil, in which case the runner picks a real client from the session
	// provider.
	runnerFactory func(client llm.Client) AgentRunner
}

// NewAgentHandler returns an AgentHandler wired up with the real agent.Agent.
func NewAgentHandler(s store.Store, emb embedder.Embedder, b *events.Broker, cfg *config.Config) *AgentHandler {
	h := &AgentHandler{store: s, embedder: emb, broker: b, cfg: cfg}
	h.runnerFactory = func(client llm.Client) AgentRunner {
		return &agent.Agent{
			Store:    s,
			Embedder: emb,
			Broker:   b,
			LLM:      client,
		}
	}
	return h
}

// SetRunnerFactory swaps the AgentRunner factory. Test-only helper.
func (h *AgentHandler) SetRunnerFactory(f func(client llm.Client) AgentRunner) {
	h.runnerFactory = f
}

// ListSessions returns the caller's chat sessions, newest first.
func (h *AgentHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	uid := userIDFromContext(r.Context())
	sessions, err := h.store.ListAgentSessions(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch sessions")
		return
	}
	if sessions == nil {
		sessions = []model.AgentSession{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"sessions": sessions})
}

type createSessionRequest struct {
	FolderID *string `json:"folder_id,omitempty"`
	Provider string  `json:"provider"`
	Model    string  `json:"model"`
	Title    string  `json:"title,omitempty"`
}

// CreateSession creates a new chat session for the caller.
func (h *AgentHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Provider == "" || req.Model == "" {
		writeError(w, http.StatusBadRequest, "provider and model are required")
		return
	}
	switch llm.Provider(req.Provider) {
	case llm.ProviderAnthropic, llm.ProviderOpenAI:
	default:
		writeError(w, http.StatusBadRequest, "Unsupported provider")
		return
	}

	uid := userIDFromContext(r.Context())
	if uid == nil {
		// Auth is disabled — allocate a synthetic user-id so the session has
		// an owner column populated. The schema requires a non-null user_id.
		writeError(w, http.StatusUnauthorized, "Authentication required for agent sessions")
		return
	}

	var folderID *uuid.UUID
	if req.FolderID != nil && *req.FolderID != "" {
		parsed, err := uuid.Parse(*req.FolderID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid folder_id")
			return
		}
		folder, err := h.store.GetFolder(r.Context(), parsed, uid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to verify folder")
			return
		}
		if folder == nil {
			writeError(w, http.StatusBadRequest, "Folder not found")
			return
		}
		folderID = &parsed
	}

	session := &model.AgentSession{
		ID:        uuid.New(),
		UserID:    *uid,
		FolderID:  folderID,
		Title:     req.Title,
		Provider:  req.Provider,
		Model:     req.Model,
		CreatedAt: time.Now().UTC(),
	}
	if err := h.store.CreateAgentSession(r.Context(), session); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create session")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"session": session})
}

// ListMessages returns the chat history of a session owned by the caller.
func (h *AgentHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid session ID")
		return
	}
	uid := userIDFromContext(r.Context())
	sess, err := h.store.GetAgentSession(r.Context(), id, uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch session")
		return
	}
	if sess == nil {
		writeError(w, http.StatusNotFound, "Session not found")
		return
	}
	msgs, err := h.store.ListAgentMessages(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch messages")
		return
	}
	if msgs == nil {
		msgs = []model.AgentMessage{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"session":  sess,
		"messages": msgs,
	})
}

// DeleteSession removes a session and its messages.
func (h *AgentHandler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid session ID")
		return
	}
	uid := userIDFromContext(r.Context())
	if err := h.store.DeleteAgentSession(r.Context(), id, uid); err != nil {
		writeError(w, http.StatusNotFound, "Session not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Session deleted"})
}

type sendMessageRequest struct {
	Content string `json:"content"`
}

// SendMessage validates the request and kicks off an async agent.Agent.Run.
// The streaming response (deltas, tool calls, completion) is delivered via
// the SSE broker; the HTTP response acknowledges acceptance with 202.
func (h *AgentHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Header.Get("X-LLM-API-Key")
	if apiKey == "" {
		writeError(w, http.StatusBadRequest, "X-LLM-API-Key header is required")
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid session ID")
		return
	}

	var req sendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	uid := userIDFromContext(r.Context())
	sess, err := h.store.GetAgentSession(r.Context(), id, uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch session")
		return
	}
	if sess == nil {
		writeError(w, http.StatusNotFound, "Session not found")
		return
	}

	history, err := h.store.ListAgentMessages(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch history")
		return
	}

	messageID := uuid.New()

	go func(session *model.AgentSession, content, apiKey string, history []model.AgentMessage) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		runner := h.runnerFactory(nil)
		_, err := runner.Run(ctx, agent.RunInput{
			Session:     session,
			UserMessage: content,
			APIKey:      apiKey,
			History:     history,
		})
		if err != nil {
			slog.Error("agent run failed", "session_id", session.ID.String(), "error", err)
		}
	}(sess, req.Content, apiKey, history)

	writeJSON(w, http.StatusAccepted, map[string]string{
		"message_id": messageID.String(),
		"status":     "accepted",
	})
}
