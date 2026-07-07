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

const anthropicVersion = "2023-06-01"

type anthropicClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewAnthropicClient(baseURL, apiKey, model string, timeout ...time.Duration) Client {
	return newAnthropicClient(baseURL, apiKey, model, defaultHTTPClient(timeoutOrDefault(timeout, defaultBrainCallTimeout)))
}

func newAnthropicClient(baseURL, apiKey, model string, httpClient *http.Client) Client {
	return &anthropicClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		httpClient: httpClient,
	}
}

func (c *anthropicClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body, toolMode, err := anthropicRequest(req, c.model, false)
	if err != nil {
		return nil, err
	}
	resp, err := c.post(ctx, body, anthropicInputText(req, body), toolMode)
	if se, ok := err.(*statusError); ok && req.JSONSchema != nil && se.StatusCode >= http.StatusBadRequest && se.StatusCode < http.StatusInternalServerError {
		fallback, _, buildErr := anthropicRequest(req, c.model, true)
		if buildErr != nil {
			return nil, buildErr
		}
		return c.post(ctx, fallback, anthropicInputText(req, fallback), false)
	}
	return resp, err
}

func (c *anthropicClient) post(ctx context.Context, body []byte, inputText string, toolMode bool) (*ChatResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	if c.apiKey != "" {
		httpReq.Header.Set("x-api-key", c.apiKey)
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
	var out anthropicResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, err
	}
	content, err := anthropicContent(out, toolMode)
	if err != nil {
		return nil, err
	}
	usage := Usage{
		InputTokens:  out.Usage.InputTokens,
		OutputTokens: out.Usage.OutputTokens,
	}
	return &ChatResponse{Content: content, Usage: usageWithEstimate(usage, inputText, content)}, nil
}

func anthropicRequest(req ChatRequest, defaultModel string, promptFallback bool) ([]byte, bool, error) {
	model := req.Model
	if model == "" {
		model = defaultModel
	}
	body := anthropicRequestBody{
		Model:       model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
	body.System, body.Messages = anthropicMessages(req.Messages)
	if req.JSONSchema != nil {
		if promptFallback {
			body.System = appendSystem(body.System, "Return only JSON matching this schema:\n"+string(req.JSONSchema))
		} else {
			var schema any
			if err := json.Unmarshal(req.JSONSchema, &schema); err != nil {
				return nil, false, err
			}
			body.Tools = []anthropicTool{{
				Name:        "response",
				Description: "Return the requested JSON object.",
				InputSchema: schema,
			}}
			body.ToolChoice = map[string]string{"type": "tool", "name": "response"}
		}
	}
	raw, err := json.Marshal(body)
	return raw, req.JSONSchema != nil && !promptFallback, err
}

func anthropicMessages(messages []ChatMessage) (string, []anthropicMessage) {
	var system []string
	out := make([]anthropicMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "system" {
			system = append(system, msg.Content)
			continue
		}
		out = append(out, anthropicMessage(msg))
	}
	return strings.Join(system, "\n\n"), out
}

func anthropicContent(resp anthropicResponse, toolMode bool) (string, error) {
	if toolMode {
		for _, block := range resp.Content {
			if block.Type == "tool_use" && block.Name == "response" && len(block.Input) > 0 {
				return string(block.Input), nil
			}
		}
		return "", fmt.Errorf("anthropic response has no response tool_use")
	}
	var b strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" {
			b.WriteString(block.Text)
		}
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("anthropic response has no text content")
	}
	return b.String(), nil
}

func anthropicInputText(req ChatRequest, body []byte) string {
	if len(body) == 0 {
		return messageText(req.Messages)
	}
	return messageText(req.Messages) + string(body)
}

func appendSystem(current, extra string) string {
	if current == "" {
		return extra
	}
	return current + "\n\n" + extra
}

type anthropicRequestBody struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens,omitempty"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Temperature float64            `json:"temperature"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
	ToolChoice  map[string]string  `json:"tool_choice,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
}

type anthropicResponse struct {
	Content []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text,omitempty"`
		Name  string          `json:"name,omitempty"`
		Input json.RawMessage `json:"input,omitempty"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}
