package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type openAIClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

type statusError struct {
	StatusCode int
	Body       string
}

func (e *statusError) Error() string {
	return fmt.Sprintf("llm http status %d: %s", e.StatusCode, e.Body)
}

func NewOpenAIClient(baseURL, apiKey, model string, timeout ...time.Duration) Client {
	return newOpenAIClient(baseURL, apiKey, model, defaultHTTPClient(timeoutOrDefault(timeout, defaultBrainCallTimeout)))
}

func newOpenAIClient(baseURL, apiKey, model string, httpClient *http.Client) Client {
	return &openAIClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		httpClient: httpClient,
	}
}

func (c *openAIClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body, err := openAIRequest(req, c.model, false)
	if err != nil {
		return nil, err
	}
	resp, err := c.post(ctx, body, messageText(req.Messages))
	if se, ok := err.(*statusError); ok && req.JSONSchema != nil && se.StatusCode >= http.StatusBadRequest && se.StatusCode < http.StatusInternalServerError {
		fallback, buildErr := openAIRequest(req, c.model, true)
		if buildErr != nil {
			return nil, buildErr
		}
		return c.post(ctx, fallback, messageText(req.Messages))
	}
	return resp, err
}

func (c *openAIClient) post(ctx context.Context, body []byte, inputText string) (*ChatResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
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
	var out openAIResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, err
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("openai response has no choices")
	}
	content := out.Choices[0].Message.Content
	usage := Usage{
		InputTokens:  out.Usage.PromptTokens,
		OutputTokens: out.Usage.CompletionTokens,
	}
	return &ChatResponse{Content: content, Usage: usageWithEstimate(usage, inputText, content)}, nil
}

func openAIRequest(req ChatRequest, defaultModel string, jsonObjectFallback bool) ([]byte, error) {
	model := req.Model
	if model == "" {
		model = defaultModel
	}
	messages := append([]ChatMessage(nil), req.Messages...)
	body := openAIRequestBody{
		Model:       model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      false,
	}
	if req.JSONSchema != nil {
		if jsonObjectFallback {
			body.ResponseFormat = map[string]any{"type": "json_object"}
			body.Messages = append(body.Messages, ChatMessage{
				Role:    "system",
				Content: "Return only JSON matching this schema:\n" + string(req.JSONSchema),
			})
		} else {
			var schema any
			if err := json.Unmarshal(req.JSONSchema, &schema); err != nil {
				return nil, err
			}
			body.ResponseFormat = map[string]any{
				"type": "json_schema",
				"json_schema": map[string]any{
					"name":   "response",
					"schema": schema,
					"strict": true,
				},
			}
		}
	}
	return json.Marshal(body)
}

type openAIRequestBody struct {
	Model          string        `json:"model"`
	Messages       []ChatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
	Stream         bool          `json:"stream"`
	ResponseFormat any           `json:"response_format,omitempty"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}
