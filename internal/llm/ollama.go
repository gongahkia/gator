package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ollamaClient struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

func NewOllamaClient(baseURL, model string) Client {
	return &ollamaClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		model:      model,
		httpClient: http.DefaultClient,
	}
}

func (c *ollamaClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body, err := ollamaRequest(req, c.model)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = httpResp.Body.Close() }()
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}
	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		return nil, &statusError{StatusCode: httpResp.StatusCode, Body: string(respBody)}
	}
	var out ollamaResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, err
	}
	if out.Message.Content == "" {
		return nil, fmt.Errorf("ollama response has empty message content")
	}
	return &ChatResponse{
		Content: out.Message.Content,
		Usage: Usage{
			InputTokens:  out.PromptEvalCount,
			OutputTokens: out.EvalCount,
		},
	}, nil
}

func ollamaRequest(req ChatRequest, defaultModel string) ([]byte, error) {
	model := req.Model
	if model == "" {
		model = defaultModel
	}
	body := ollamaRequestBody{
		Model:    model,
		Messages: req.Messages,
		Stream:   false,
		Options: map[string]any{
			"temperature": req.Temperature,
		},
	}
	if req.JSONSchema != nil {
		var format any
		if err := json.Unmarshal(req.JSONSchema, &format); err != nil {
			return nil, err
		}
		body.Format = format
	}
	return json.Marshal(body)
}

type ollamaRequestBody struct {
	Model    string         `json:"model"`
	Messages []ChatMessage  `json:"messages"`
	Stream   bool           `json:"stream"`
	Format   any            `json:"format,omitempty"`
	Options  map[string]any `json:"options"`
}

type ollamaResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	PromptEvalCount int `json:"prompt_eval_count"`
	EvalCount       int `json:"eval_count"`
}
