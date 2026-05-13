package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type HTTPEmbedder struct {
	baseURL    string
	httpClient *http.Client
}

type embedRequest struct {
	Texts []string `json:"texts"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

func NewHTTPEmbedder(baseURL string) *HTTPEmbedder {
	return &HTTPEmbedder{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (e *HTTPEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody, err := json.Marshal(embedRequest{Texts: texts})
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embed", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embed request returned status %d: %s", resp.StatusCode, string(body))
	}

	var result embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embed response: %w", err)
	}

	return result.Embeddings, nil
}

func (e *HTTPEmbedder) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	results, err := e.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}
	return results[0], nil
}

func (e *HTTPEmbedder) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("embedding sidecar unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("embedding sidecar unhealthy: status %d", resp.StatusCode)
	}
	return nil
}
