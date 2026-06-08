package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/docinsight/backend/internal/agent"
	"github.com/docinsight/backend/internal/llm"
	"github.com/docinsight/backend/internal/model"
	"github.com/google/uuid"
)

// agentRunTimeout caps a single agent chat turn. Mirrors the old HTTP handler.
const agentRunTimeout = 120 * time.Second

// ListAgentSessions returns the local user's chat sessions, newest first.
func (a *App) ListAgentSessions() ([]model.AgentSession, error) {
	sessions, err := a.store.ListAgentSessions(a.ctx, a.userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sessions: %w", err)
	}
	if sessions == nil {
		sessions = []model.AgentSession{}
	}
	return sessions, nil
}

// CreateAgentSession creates a new BYO-LLM chat session for the local user.
// folderID is optional; pass "" for none.
func (a *App) CreateAgentSession(provider string, modelName string, title string, folderID string) (*model.AgentSession, error) {
	if provider == "" || modelName == "" {
		return nil, fmt.Errorf("provider and model are required")
	}
	switch llm.Provider(provider) {
	case llm.ProviderAnthropic, llm.ProviderOpenAI:
	default:
		return nil, fmt.Errorf("unsupported provider: %q", provider)
	}

	if a.userID == nil {
		// The schema requires a non-null user_id; the session must have an owner.
		return nil, fmt.Errorf("authentication required for agent sessions")
	}

	var folderPtr *uuid.UUID
	if folderID != "" {
		parsed, err := uuid.Parse(folderID)
		if err != nil {
			return nil, fmt.Errorf("invalid folder ID: %w", err)
		}
		folder, err := a.store.GetFolder(a.ctx, parsed, a.userID)
		if err != nil {
			return nil, fmt.Errorf("failed to verify folder: %w", err)
		}
		if folder == nil {
			return nil, fmt.Errorf("folder not found")
		}
		folderPtr = &parsed
	}

	session := &model.AgentSession{
		ID:        uuid.New(),
		UserID:    *a.userID,
		FolderID:  folderPtr,
		Title:     title,
		Provider:  provider,
		Model:     modelName,
		CreatedAt: time.Now().UTC(),
	}
	if err := a.store.CreateAgentSession(a.ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	return session, nil
}

// DeleteAgentSession removes a session (and its messages) owned by the local user.
func (a *App) DeleteAgentSession(id string) error {
	sessionID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}
	if err := a.store.DeleteAgentSession(a.ctx, sessionID, a.userID); err != nil {
		// Treat any failure (including wrong-user) as not-found; don't leak existence.
		return fmt.Errorf("session not found")
	}
	return nil
}

// ListAgentMessages returns the chat history of a session owned by the local user.
func (a *App) ListAgentMessages(sessionID string) ([]model.AgentMessage, error) {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return nil, fmt.Errorf("invalid session ID: %w", err)
	}
	sess, err := a.store.GetAgentSession(a.ctx, id, a.userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch session: %w", err)
	}
	if sess == nil {
		return nil, fmt.Errorf("session not found")
	}
	msgs, err := a.store.ListAgentMessages(a.ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch messages: %w", err)
	}
	if msgs == nil {
		msgs = []model.AgentMessage{}
	}
	return msgs, nil
}

// SendAgentMessage validates the request and kicks off an async agent run. The
// streaming response (deltas, tool calls, completion) is delivered via the event
// broker (forwarded to the frontend); this method returns once the run is
// launched, mirroring the old handler's 202 Accepted.
func (a *App) SendAgentMessage(sessionID string, content string, llmAPIKey string) error {
	if llmAPIKey == "" {
		return fmt.Errorf("LLM API key is required")
	}

	id, err := uuid.Parse(sessionID)
	if err != nil {
		return fmt.Errorf("invalid session ID: %w", err)
	}

	if content == "" {
		return fmt.Errorf("content is required")
	}

	sess, err := a.store.GetAgentSession(a.ctx, id, a.userID)
	if err != nil {
		return fmt.Errorf("failed to fetch session: %w", err)
	}
	if sess == nil {
		return fmt.Errorf("session not found")
	}

	history, err := a.store.ListAgentMessages(a.ctx, id)
	if err != nil {
		return fmt.Errorf("failed to fetch history: %w", err)
	}

	go func(session *model.AgentSession, content, apiKey string, history []model.AgentMessage) {
		ctx, cancel := context.WithTimeout(context.Background(), agentRunTimeout)
		defer cancel()

		runner := &agent.Agent{
			Store:    a.store,
			Embedder: a.emb,
			Broker:   a.broker,
			// LLM left nil so the agent picks a real client from session.Provider.
		}
		_, err := runner.Run(ctx, agent.RunInput{
			Session:     session,
			UserMessage: content,
			APIKey:      apiKey,
			History:     history,
		})
		if err != nil {
			slog.Error("agent run failed", "session_id", session.ID.String(), "error", err)
		}
	}(sess, content, llmAPIKey, history)

	return nil
}
