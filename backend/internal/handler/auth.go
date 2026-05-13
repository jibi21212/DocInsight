package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/store"
	"github.com/google/uuid"
)

type contextKey string

const userContextKey contextKey = "user"

// UserFromContext retrieves the authenticated user from the request context.
func UserFromContext(ctx context.Context) *model.User {
	if u, ok := ctx.Value(userContextKey).(*model.User); ok {
		return u
	}
	return nil
}

// AuthMiddleware validates Bearer tokens against the users table.
// When auth is disabled, all requests pass through without a user.
func AuthMiddleware(s store.Store, enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !enabled {
				next.ServeHTTP(w, r)
				return
			}

			// Allow SSE and health endpoints without auth
			if r.URL.Path == "/api/events" || r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			// Allow registration without auth
			if r.URL.Path == "/api/auth/register" && r.Method == http.MethodPost {
				next.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeError(w, http.StatusUnauthorized, "Authorization header required")
				return
			}

			if !strings.HasPrefix(authHeader, "Bearer ") {
				writeError(w, http.StatusUnauthorized, "Invalid authorization format (use Bearer token)")
				return
			}

			apiKey := strings.TrimPrefix(authHeader, "Bearer ")
			user, err := s.GetUserByAPIKey(r.Context(), apiKey)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "Auth lookup failed")
				return
			}
			if user == nil {
				writeError(w, http.StatusUnauthorized, "Invalid API key")
				return
			}

			ctx := context.WithValue(r.Context(), userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthHandler handles user registration and profile retrieval.
type AuthHandler struct {
	store store.Store
}

func NewAuthHandler(s store.Store) *AuthHandler {
	return &AuthHandler{store: s}
}

type registerRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	email := strings.TrimSpace(req.Email)
	if email == "" || !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "Valid email is required")
		return
	}

	// Check if email already exists
	existing, err := h.store.GetUserByEmail(r.Context(), email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, "Email already registered")
		return
	}

	// Generate API key
	apiKey, err := generateAPIKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate API key")
		return
	}

	user := &model.User{
		ID:     uuid.New(),
		Email:  email,
		APIKey: apiKey,
		Name:   strings.TrimSpace(req.Name),
	}

	if err := h.store.CreateUser(r.Context(), user); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create user")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"user":    user,
		"message": "Registration successful. Save your API key — it won't be shown again.",
	})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	// Don't send back the API key
	safeUser := *user
	safeUser.APIKey = ""

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": safeUser,
	})
}

func generateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "di_" + hex.EncodeToString(bytes), nil
}
