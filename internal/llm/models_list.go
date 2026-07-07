package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type ModelInfo struct {
	ID string `json:"id"`
}

func ListOllamaModels(ctx context.Context, baseURL string) ([]ModelInfo, error) {
	return EndpointHealthChecker{}.ListOllamaModels(ctx, baseURL)
}

func (c EndpointHealthChecker) ListOllamaModels(ctx context.Context, baseURL string) ([]ModelInfo, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return c.getModelInfos(ctx, http.MethodGet, baseURL+"/api/tags", nil, "models")
}

func PullOllamaModel(ctx context.Context, baseURL, model string) error {
	return EndpointHealthChecker{}.PullOllamaModel(ctx, baseURL, model)
}

func (c EndpointHealthChecker) PullOllamaModel(ctx context.Context, baseURL, model string) error {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	body, err := json.Marshal(map[string]any{
		"model":  model,
		"stream": false,
	})
	if err != nil {
		return err
	}
	_, err = c.probeHTTP(ctx, http.MethodPost, baseURL+"/api/pull", bytes.NewReader(body), map[string]string{"Content-Type": "application/json"})
	return err
}

func SmokeCheckOllamaSchema(ctx context.Context, baseURL, model string) error {
	return EndpointHealthChecker{}.SmokeCheckOllamaSchema(ctx, baseURL, model)
}

func (c EndpointHealthChecker) SmokeCheckOllamaSchema(ctx context.Context, baseURL, model string) error {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	body, err := ollamaRequest(ChatRequest{
		Model: model,
		Messages: []ChatMessage{{
			Role:    "user",
			Content: `Return exactly {"ok":true} as JSON.`,
		}},
		Temperature: 0,
		JSONSchema:  json.RawMessage(`{"type":"object","additionalProperties":false,"required":["ok"],"properties":{"ok":{"type":"boolean"}}}`),
	}, model)
	if err != nil {
		return err
	}
	respBody, err := c.probeHTTP(ctx, http.MethodPost, baseURL+"/api/chat", bytes.NewReader(body), map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return err
	}
	var out ollamaResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out.Message.Content), &payload); err != nil {
		return fmt.Errorf("schema smoke returned invalid JSON: %w", err)
	}
	ok, hasOK := payload["ok"].(bool)
	if !hasOK || !ok || len(payload) != 1 {
		return fmt.Errorf("schema smoke response mismatch: %s", out.Message.Content)
	}
	return nil
}

func (c EndpointHealthChecker) getModelIDs(ctx context.Context, method, url string, headers map[string]string, field string) (map[string]bool, error) {
	models, err := c.getModelInfos(ctx, method, url, headers, field)
	if err != nil {
		return nil, err
	}
	return modelSet(models), nil
}

func (c EndpointHealthChecker) getModelInfos(ctx context.Context, method, url string, headers map[string]string, field string) ([]ModelInfo, error) {
	body, err := c.probeHTTP(ctx, method, url, nil, headers)
	if err != nil {
		return nil, err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	var items []map[string]any
	if err := json.Unmarshal(doc[field], &items); err != nil {
		return nil, fmt.Errorf("models response missing %q array: %w", field, err)
	}
	var models []ModelInfo
	for _, item := range items {
		if id, ok := item["id"].(string); ok && id != "" {
			models = append(models, ModelInfo{ID: id})
		}
		if name, ok := item["name"].(string); ok && name != "" {
			models = append(models, ModelInfo{ID: name})
		}
		if model, ok := item["model"].(string); ok && model != "" {
			models = append(models, ModelInfo{ID: model})
		}
	}
	return models, nil
}

func modelSet(models []ModelInfo) map[string]bool {
	out := map[string]bool{}
	for _, model := range models {
		if model.ID != "" {
			out[model.ID] = true
		}
	}
	return out
}
