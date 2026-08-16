// Package openai implements Gator's native adapter for the OpenAI Responses
// API. It owns only protocol conversion; the agent loop remains in internal/agent.
package openai

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
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/agent"
)

const (
	defaultBaseURL = "https://api.openai.com/v1/responses"
	defaultModel   = "gpt-5.6"
)

// DefaultModel is the model used when the caller does not explicitly configure
// one. It is exposed so the CLI can accurately report its effective setting.
func DefaultModel() string {
	return defaultModel
}

// Responses is a stateless OpenAI Responses API adapter. The core sends the
// full normalized history on each call, allowing Gator to own persistence and
// avoid provider-side conversation storage.
type Responses struct {
	APIKey              string
	APIKeyEnv           string
	Model               string
	BaseURL             string
	AuthorizationHeader string
	AuthorizationPrefix string
	Headers             http.Header
	Client              *http.Client
}

// Complete implements agent.Model.
func (r Responses) Complete(ctx context.Context, turn agent.TurnRequest) (agent.Turn, error) {
	if strings.TrimSpace(r.APIKey) == "" {
		return agent.Turn{}, errors.New(r.apiKeyEnv() + " is required")
	}
	body, err := r.requestBody(turn)
	if err != nil {
		return agent.Turn{}, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return agent.Turn{}, fmt.Errorf("encode OpenAI request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL(), bytes.NewReader(payload))
	if err != nil {
		return agent.Turn{}, fmt.Errorf("create OpenAI request: %w", err)
	}
	r.setHeaders(request, false)
	response, err := r.client().Do(request)
	if err != nil {
		return agent.Turn{}, fmt.Errorf("request OpenAI response: %w", err)
	}
	defer response.Body.Close()

	contents, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	if err != nil {
		return agent.Turn{}, fmt.Errorf("read OpenAI response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return agent.Turn{}, describeAPIError(response.StatusCode, contents)
	}
	return decodeResponse(contents)
}

// CompleteStream implements agent.StreamingModel. It forwards output-text
// deltas as they arrive, then returns the completed typed response used by the
// core tool loop.
func (r Responses) CompleteStream(ctx context.Context, turn agent.TurnRequest, onDelta func(string)) (agent.Turn, error) {
	if strings.TrimSpace(r.APIKey) == "" {
		return agent.Turn{}, errors.New(r.apiKeyEnv() + " is required")
	}
	body, err := r.requestBody(turn)
	if err != nil {
		return agent.Turn{}, err
	}
	body.Stream = true
	payload, err := json.Marshal(body)
	if err != nil {
		return agent.Turn{}, fmt.Errorf("encode OpenAI stream request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL(), bytes.NewReader(payload))
	if err != nil {
		return agent.Turn{}, fmt.Errorf("create OpenAI stream request: %w", err)
	}
	r.setHeaders(request, true)
	response, err := r.client().Do(request)
	if err != nil {
		return agent.Turn{}, fmt.Errorf("request OpenAI stream: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		contents, readErr := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
		if readErr != nil {
			return agent.Turn{}, fmt.Errorf("read OpenAI stream error: %w", readErr)
		}
		return agent.Turn{}, describeAPIError(response.StatusCode, contents)
	}
	return decodeSSE(response.Body, onDelta)
}

func (r Responses) setHeaders(request *http.Request, stream bool) {
	authorizationHeader := r.AuthorizationHeader
	if authorizationHeader == "" {
		authorizationHeader = "Authorization"
	}
	authorizationPrefix := r.AuthorizationPrefix
	if authorizationHeader == "Authorization" && authorizationPrefix == "" {
		authorizationPrefix = "Bearer "
	}
	request.Header.Set(authorizationHeader, authorizationPrefix+r.APIKey)
	request.Header.Set("Content-Type", "application/json")
	if stream {
		request.Header.Set("Accept", "text/event-stream")
	}
	for name, values := range r.Headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
}

func (r Responses) apiKeyEnv() string {
	if strings.TrimSpace(r.APIKeyEnv) != "" {
		return r.APIKeyEnv
	}
	return "OPENAI_API_KEY"
}

func (r Responses) requestBody(turn agent.TurnRequest) (responseRequest, error) {
	input, err := encodeInput(turn.Messages)
	if err != nil {
		return responseRequest{}, err
	}
	tools := make([]functionTool, 0, len(turn.Tools))
	for _, definition := range turn.Tools {
		if !json.Valid(definition.Parameters) {
			return responseRequest{}, fmt.Errorf("tool %q has invalid JSON Schema", definition.Name)
		}
		tools = append(tools, functionTool{
			Type:        "function",
			Name:        definition.Name,
			Description: definition.Description,
			Parameters:  definition.Parameters,
			Strict:      false,
		})
	}
	return responseRequest{
		Model:        modelOrDefault(r.Model),
		Instructions: turn.System,
		Input:        input,
		Tools:        tools,
		Store:        false,
	}, nil
}

func (r Responses) baseURL() string {
	if r.BaseURL != "" {
		return r.BaseURL
	}
	return defaultBaseURL
}

func (r Responses) client() *http.Client {
	if r.Client != nil {
		return r.Client
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

func modelOrDefault(model string) string {
	if strings.TrimSpace(model) == "" {
		return defaultModel
	}
	return model
}

type responseRequest struct {
	Model        string         `json:"model"`
	Instructions string         `json:"instructions,omitempty"`
	Input        []inputItem    `json:"input"`
	Tools        []functionTool `json:"tools,omitempty"`
	Store        bool           `json:"store"`
	Stream       bool           `json:"stream,omitempty"`
}

type functionTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      bool            `json:"strict"`
}

type inputItem struct {
	Type      string `json:"type,omitempty"`
	Role      string `json:"role,omitempty"`
	Content   any    `json:"content,omitempty"`
	ID        string `json:"id,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`
}

func encodeInput(messages []agent.Message) ([]inputItem, error) {
	input := make([]inputItem, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case agent.RoleUser, agent.RoleAgent:
			role := "user"
			if message.Role == agent.RoleAgent {
				role = "assistant"
			}
			if message.Content != "" || len(message.Images) > 0 || len(message.Attachments) > 0 {
				content := any(message.Content)
				if len(message.Images) > 0 || len(message.Attachments) > 0 {
					parts := make([]responseInputPart, 0, len(message.Images)+len(message.Attachments)+1)
					if message.Content != "" {
						parts = append(parts, responseInputPart{Type: "input_text", Text: message.Content})
					}
					for _, image := range message.Images {
						parts = append(parts, responseInputPart{Type: "input_image", ImageURL: imageDataURL(image)})
					}
					for _, attachment := range message.Attachments {
						switch attachment.MediaType {
						case "application/pdf":
							parts = append(parts, responseInputPart{Type: "input_file", FileData: base64.StdEncoding.EncodeToString(attachment.Data), Filename: attachment.Name})
						case "text/plain":
							parts = append(parts, responseInputPart{Type: "input_text", Text: agent.AttachmentText(attachment)})
						default:
							return nil, fmt.Errorf("OpenAI attachment %q has unsupported media type %q", attachment.Name, attachment.MediaType)
						}
					}
					content = parts
				}
				input = append(input, inputItem{Role: role, Content: content})
			}
			if message.Role == agent.RoleAgent {
				for _, call := range message.ToolCalls {
					if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" || !json.Valid(call.Arguments) {
						return nil, errors.New("agent history contains an invalid tool call")
					}
					input = append(input, inputItem{
						Type:      "function_call",
						ID:        call.ProviderID,
						CallID:    call.ID,
						Name:      call.Name,
						Arguments: string(call.Arguments),
					})
				}
			}
		case agent.RoleTool:
			if strings.TrimSpace(message.ToolCallID) == "" {
				return nil, errors.New("agent history contains a tool result without a call id")
			}
			input = append(input, inputItem{Type: "function_call_output", CallID: message.ToolCallID, Output: message.Content})
		default:
			return nil, fmt.Errorf("agent history contains unsupported role %q", message.Role)
		}
	}
	return input, nil
}

type responseInputPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	FileData string `json:"file_data,omitempty"`
	Filename string `json:"filename,omitempty"`
}

func imageDataURL(image agent.Image) string {
	return "data:" + image.MediaType + ";base64," + base64.StdEncoding.EncodeToString(image.Data)
}

type responsePayload struct {
	Output []responseOutputItem `json:"output"`
}

type responseOutputItem struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func decodeResponse(contents []byte) (agent.Turn, error) {
	var payload responsePayload
	if err := json.Unmarshal(contents, &payload); err != nil {
		return agent.Turn{}, fmt.Errorf("decode OpenAI response: %w", err)
	}
	var text strings.Builder
	var calls []agent.ToolCall
	for _, item := range payload.Output {
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				if content.Type == "output_text" {
					text.WriteString(content.Text)
				}
			}
		case "function_call":
			if item.CallID == "" || item.Name == "" || !json.Valid([]byte(item.Arguments)) {
				return agent.Turn{}, errors.New("OpenAI response contains an invalid function call")
			}
			calls = append(calls, agent.ToolCall{
				ID:         item.CallID,
				ProviderID: item.ID,
				Name:       item.Name,
				Arguments:  json.RawMessage(item.Arguments),
			})
		}
	}
	if text.Len() == 0 && len(calls) == 0 {
		return agent.Turn{}, errors.New("OpenAI response contained no output text or function calls")
	}
	return agent.Turn{Text: text.String(), ToolCalls: calls}, nil
}

func decodeSSE(reader io.Reader, onDelta func(string)) (agent.Turn, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	var eventType string
	var dataLines []string
	var completed *agent.Turn
	flush := func() error {
		if len(dataLines) == 0 {
			eventType = ""
			return nil
		}
		data := strings.Join(dataLines, "\n")
		dataLines = nil
		if data == "[DONE]" {
			eventType = ""
			return nil
		}
		var event struct {
			Type     string          `json:"type"`
			Delta    string          `json:"delta"`
			Response json.RawMessage `json:"response"`
			Error    struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return fmt.Errorf("decode OpenAI stream event: %w", err)
		}
		if event.Type == "" {
			event.Type = eventType
		}
		switch event.Type {
		case "response.output_text.delta":
			if onDelta != nil && event.Delta != "" {
				onDelta(event.Delta)
			}
		case "response.completed":
			if len(event.Response) == 0 {
				return errors.New("OpenAI completed stream event omitted the response")
			}
			turn, err := decodeResponse(event.Response)
			if err != nil {
				return err
			}
			completed = &turn
		case "response.failed":
			if event.Error.Message != "" {
				return fmt.Errorf("OpenAI response failed: %s", event.Error.Message)
			}
			return errors.New("OpenAI response failed")
		}
		eventType = ""
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
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return agent.Turn{}, fmt.Errorf("read OpenAI event stream: %w", err)
	}
	if err := flush(); err != nil {
		return agent.Turn{}, err
	}
	if completed == nil {
		return agent.Turn{}, errors.New("OpenAI event stream ended without a completed response")
	}
	return *completed, nil
}

func describeAPIError(status int, contents []byte) error {
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(contents, &payload); err == nil && payload.Error.Message != "" {
		return fmt.Errorf("OpenAI Responses API returned HTTP %d: %s", status, payload.Error.Message)
	}
	return fmt.Errorf("OpenAI Responses API returned HTTP %d", status)
}
