// Package radius implements Gator's adapter for a pi-messages-compatible
// gateway such as Radius. Gator still owns the coding-agent and tool loop.
package radius

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/agent"
)

const (
	defaultGateway  = "https://radius.pi.dev"
	defaultMaxToken = 8192
	maxResponseSize = 2 * 1024 * 1024
)

// Messages sends a normalized Gator turn to a pi-messages gateway.
type Messages struct {
	APIKey  string
	Model   string
	BaseURL string
	Gateway string
	Client  *http.Client
}

// Complete implements agent.Model.
func (m Messages) Complete(ctx context.Context, turn agent.TurnRequest) (agent.Turn, error) {
	return m.complete(ctx, turn, nil)
}

// CompleteStream implements agent.StreamingModel over the gateway SSE stream.
func (m Messages) CompleteStream(ctx context.Context, turn agent.TurnRequest, onDelta func(string)) (agent.Turn, error) {
	return m.complete(ctx, turn, onDelta)
}

func (m Messages) complete(ctx context.Context, turn agent.TurnRequest, onDelta func(string)) (agent.Turn, error) {
	if strings.TrimSpace(m.APIKey) == "" {
		return agent.Turn{}, errors.New("RADIUS_API_KEY is required")
	}
	if strings.TrimSpace(m.Model) == "" {
		return agent.Turn{}, errors.New("Radius model is required")
	}
	endpoint, err := m.endpoint(ctx)
	if err != nil {
		return agent.Turn{}, err
	}
	payload, err := m.payload(turn)
	if err != nil {
		return agent.Turn{}, err
	}
	contents, err := json.Marshal(payload)
	if err != nil {
		return agent.Turn{}, fmt.Errorf("encode Radius request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/messages", bytes.NewReader(contents))
	if err != nil {
		return agent.Turn{}, fmt.Errorf("create Radius request: %w", err)
	}
	request.Header.Set("authorization", "Bearer "+m.APIKey)
	request.Header.Set("accept", "text/event-stream")
	request.Header.Set("content-type", "application/json")
	response, err := m.client().Do(request)
	if err != nil {
		return agent.Turn{}, fmt.Errorf("request Radius response: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseSize))
		if readErr != nil {
			return agent.Turn{}, fmt.Errorf("read Radius response: %w", readErr)
		}
		return agent.Turn{}, agent.HTTPStatusError(response.StatusCode, fmt.Errorf("Radius request returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body))))
	}
	return decodeSSE(response.Body, onDelta)
}

func (m Messages) endpoint(ctx context.Context) (string, error) {
	if baseURL := strings.TrimRight(strings.TrimSpace(m.BaseURL), "/"); baseURL != "" {
		if _, err := absoluteURL(baseURL); err != nil {
			return "", fmt.Errorf("invalid Radius base URL: %w", err)
		}
		return baseURL, nil
	}
	gateway := strings.TrimRight(strings.TrimSpace(m.Gateway), "/")
	if gateway == "" {
		gateway = defaultGateway
	}
	if _, err := absoluteURL(gateway); err != nil {
		return "", fmt.Errorf("invalid Radius gateway URL: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, gateway+"/v1/config", nil)
	if err != nil {
		return "", fmt.Errorf("create Radius config request: %w", err)
	}
	request.Header.Set("accept", "application/json")
	request.Header.Set("authorization", "Bearer "+m.APIKey)
	response, err := m.client().Do(request)
	if err != nil {
		return "", fmt.Errorf("request Radius config: %w", err)
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil {
		return "", fmt.Errorf("read Radius config: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("Radius config returned HTTP %d", response.StatusCode)
	}
	var config struct {
		BaseURL string `json:"baseUrl"`
	}
	if err := json.Unmarshal(contents, &config); err != nil {
		return "", errors.New("Radius config returned invalid JSON")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if _, err := absoluteURL(baseURL); err != nil {
		return "", errors.New("Radius config returned an invalid base URL")
	}
	return baseURL, nil
}

func (m Messages) payload(turn agent.TurnRequest) (request, error) {
	messages, err := encodeMessages(turn.Messages, m.Model)
	if err != nil {
		return request{}, err
	}
	tools := make([]tool, 0, len(turn.Tools))
	for _, definition := range turn.Tools {
		if strings.TrimSpace(definition.Name) == "" || !json.Valid(definition.Parameters) {
			return request{}, fmt.Errorf("tool %q has invalid JSON Schema", definition.Name)
		}
		var parameters map[string]any
		if err := json.Unmarshal(definition.Parameters, &parameters); err != nil {
			return request{}, fmt.Errorf("decode schema for tool %q: %w", definition.Name, err)
		}
		tools = append(tools, tool{Name: definition.Name, Description: definition.Description, Parameters: parameters})
	}
	return request{Model: m.Model, Context: contextPayload{SystemPrompt: turn.System, Messages: messages, Tools: tools}, Options: requestOptions{MaxTokens: defaultMaxToken}}, nil
}

type request struct {
	Model   string         `json:"model"`
	Context contextPayload `json:"context"`
	Options requestOptions `json:"options"`
}

type contextPayload struct {
	SystemPrompt string            `json:"systemPrompt,omitempty"`
	Messages     []json.RawMessage `json:"messages"`
	Tools        []tool            `json:"tools,omitempty"`
}

type requestOptions struct {
	MaxTokens int `json:"maxTokens"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

func encodeMessages(source []agent.Message, model string) ([]json.RawMessage, error) {
	messages := make([]json.RawMessage, 0, len(source))
	for _, item := range source {
		payload, err := encodeMessage(item, model)
		if err != nil {
			return nil, err
		}
		contents, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode Radius message: %w", err)
		}
		messages = append(messages, contents)
	}
	return messages, nil
}

func encodeMessage(item agent.Message, model string) (any, error) {
	timestamp := time.Now().UnixMilli()
	switch item.Role {
	case agent.RoleUser:
		content := make([]any, 0, len(item.Images)+len(item.Attachments)+1)
		if item.Content != "" {
			content = append(content, map[string]any{"type": "text", "text": item.Content})
		}
		for _, image := range item.Images {
			content = append(content, map[string]any{"type": "image", "data": base64.StdEncoding.EncodeToString(image.Data), "mimeType": image.MediaType})
		}
		for _, attachment := range item.Attachments {
			if attachment.MediaType != "text/plain" {
				return nil, fmt.Errorf("Radius attachment %q has unsupported media type %q", attachment.Name, attachment.MediaType)
			}
			content = append(content, map[string]any{"type": "text", "text": agent.AttachmentText(attachment)})
		}
		if len(content) == 0 {
			return nil, errors.New("agent history contains an empty user message")
		}
		if len(content) == 1 && len(item.Images) == 0 && len(item.Attachments) == 0 {
			return map[string]any{"role": "user", "content": item.Content, "timestamp": timestamp}, nil
		}
		return map[string]any{"role": "user", "content": content, "timestamp": timestamp}, nil
	case agent.RoleAgent:
		content := make([]any, 0, len(item.ToolCalls)+1)
		if item.Content != "" {
			content = append(content, map[string]any{"type": "text", "text": item.Content})
		}
		for _, call := range item.ToolCalls {
			if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" || !json.Valid(call.Arguments) {
				return nil, errors.New("agent history contains an invalid tool call")
			}
			var arguments map[string]any
			if err := json.Unmarshal(call.Arguments, &arguments); err != nil {
				return nil, fmt.Errorf("decode arguments for tool %q: %w", call.Name, err)
			}
			content = append(content, map[string]any{"type": "toolCall", "id": call.ID, "name": call.Name, "arguments": arguments})
		}
		if len(content) == 0 {
			return nil, errors.New("agent history contains an empty agent message")
		}
		return map[string]any{
			"role":       "assistant",
			"content":    content,
			"api":        "pi-messages",
			"provider":   "radius",
			"model":      model,
			"usage":      map[string]any{"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0, "totalTokens": 0, "cost": map[string]any{"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0, "total": 0}},
			"stopReason": "stop",
			"timestamp":  timestamp,
		}, nil
	case agent.RoleTool:
		if strings.TrimSpace(item.ToolCallID) == "" || strings.TrimSpace(item.ToolName) == "" {
			return nil, errors.New("agent history contains a tool result without a call id or name")
		}
		return map[string]any{"role": "toolResult", "toolCallId": item.ToolCallID, "toolName": item.ToolName, "content": []any{map[string]any{"type": "text", "text": item.Content}}, "isError": false, "timestamp": timestamp}, nil
	default:
		return nil, fmt.Errorf("agent history contains unsupported role %q", item.Role)
	}
}

func decodeSSE(reader io.Reader, onDelta func(string)) (agent.Turn, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxResponseSize)
	turn := agent.Turn{}
	var dataLines []string
	done := false
	flush := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		data := strings.Join(dataLines, "\n")
		dataLines = nil
		var event struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
			Tool  struct {
				ID        string         `json:"id"`
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"toolCall"`
			ErrorMessage string `json:"errorMessage"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return fmt.Errorf("decode Radius stream event: %w", err)
		}
		switch event.Type {
		case "text_delta":
			turn.Text += event.Delta
			if onDelta != nil && event.Delta != "" {
				onDelta(event.Delta)
			}
		case "toolcall_end":
			if strings.TrimSpace(event.Tool.ID) == "" || strings.TrimSpace(event.Tool.Name) == "" {
				return errors.New("Radius stream returned an invalid tool call")
			}
			arguments, err := json.Marshal(event.Tool.Arguments)
			if err != nil {
				return fmt.Errorf("encode Radius tool arguments: %w", err)
			}
			turn.ToolCalls = append(turn.ToolCalls, agent.ToolCall{ID: event.Tool.ID, ProviderID: event.Tool.ID, Name: event.Tool.Name, Arguments: arguments})
		case "done":
			done = true
		case "error":
			if event.ErrorMessage != "" {
				return errors.New("Radius stream failed: " + event.ErrorMessage)
			}
			return errors.New("Radius stream failed")
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return agent.Turn{}, err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return agent.Turn{}, fmt.Errorf("read Radius event stream: %w", err)
	}
	if err := flush(); err != nil {
		return agent.Turn{}, err
	}
	if !done {
		return agent.Turn{}, errors.New("Radius stream ended without a terminal event")
	}
	if turn.Text == "" && len(turn.ToolCalls) == 0 {
		return agent.Turn{}, errors.New("Radius response contained no output text or tool calls")
	}
	return turn, nil
}

func (m Messages) client() *http.Client {
	if m.Client != nil {
		return m.Client
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

func absoluteURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("must be an absolute URL")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, errors.New("must use http or https")
	}
	return parsed, nil
}
