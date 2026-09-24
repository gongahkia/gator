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
		return agent.Turn{}, agent.HTTPStatusError(response.StatusCode, describeAPIError(response.StatusCode, contents))
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
		return agent.Turn{}, agent.HTTPStatusError(response.StatusCode, describeAPIError(response.StatusCode, contents))
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
	tools := make([]responseTool, 0, len(turn.Tools)+1)
	for _, definition := range turn.Tools {
		if definition.Name == "computer_action" && turn.Computer != nil {
			continue // CUA is a provider-native computer tool, not a function schema.
		}
		if !json.Valid(definition.Parameters) {
			return responseRequest{}, fmt.Errorf("tool %q has invalid JSON Schema", definition.Name)
		}
		tools = append(tools, responseTool{
			Type:        "function",
			Name:        definition.Name,
			Description: definition.Description,
			Parameters:  definition.Parameters,
			Strict:      false,
		})
	}
	store := false
	previousResponseID := ""
	if turn.Computer != nil {
		if !turn.Computer.RetainState {
			return responseRequest{}, errors.New("OpenAI computer use requires explicit provider-state retention consent")
		}
		tools = append(tools, responseTool{Type: "computer"})
		previousResponseID, input, err = computerContinuationInput(turn.Messages)
		if err != nil {
			return responseRequest{}, err
		}
		store = true
	}
	return responseRequest{
		Model:              modelOrDefault(r.Model),
		Instructions:       turn.System,
		Input:              input,
		Tools:              tools,
		Store:              store,
		PreviousResponseID: previousResponseID,
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
	Model              string         `json:"model"`
	Instructions       string         `json:"instructions,omitempty"`
	Input              []inputItem    `json:"input"`
	Tools              []responseTool `json:"tools,omitempty"`
	Store              bool           `json:"store"`
	PreviousResponseID string         `json:"previous_response_id,omitempty"`
	Stream             bool           `json:"stream,omitempty"`
}

type responseTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      bool            `json:"strict,omitempty"`
}

type inputItem struct {
	Type      string            `json:"type,omitempty"`
	Role      string            `json:"role,omitempty"`
	Content   any               `json:"content,omitempty"`
	ID        string            `json:"id,omitempty"`
	CallID    string            `json:"call_id,omitempty"`
	Name      string            `json:"name,omitempty"`
	Arguments string            `json:"arguments,omitempty"`
	Output    any               `json:"output,omitempty"`
	Actions   []json.RawMessage `json:"actions,omitempty"`
}

func encodeInput(messages []agent.Message) ([]inputItem, error) {
	calls := make(map[string]agent.ToolCall)
	for _, message := range messages {
		if message.Role != agent.RoleAgent {
			continue
		}
		for _, call := range message.ToolCalls {
			calls[call.ID] = call
		}
	}
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
					if call.Kind == agent.ToolCallComputer {
						var computer struct {
							Actions []json.RawMessage `json:"actions"`
						}
						if err := json.Unmarshal(call.Arguments, &computer); err != nil || len(computer.Actions) == 0 {
							return nil, errors.New("agent history contains an invalid computer action batch")
						}
						input = append(input, inputItem{Type: "computer_call", ID: call.ProviderID, CallID: call.ID, Actions: append([]json.RawMessage(nil), computer.Actions...)})
					} else {
						input = append(input, inputItem{Type: "function_call", ID: call.ProviderID, CallID: call.ID, Name: call.Name, Arguments: string(call.Arguments)})
					}
				}
			}
		case agent.RoleTool:
			if strings.TrimSpace(message.ToolCallID) == "" {
				return nil, errors.New("agent history contains a tool result without a call id")
			}
			call, known := calls[message.ToolCallID]
			if known && call.Kind == agent.ToolCallComputer {
				if len(message.Images) != 1 || message.Images[0].MediaType != "image/png" || len(message.Images[0].Data) == 0 {
					return nil, errors.New("computer call output requires exactly one transient PNG screenshot")
				}
				input = append(input, inputItem{Type: "computer_call_output", CallID: message.ToolCallID, Output: computerScreenshotOutput(message.Images[0])})
			} else {
				input = append(input, inputItem{Type: "function_call_output", CallID: message.ToolCallID, Output: message.Content})
			}
		default:
			return nil, fmt.Errorf("agent history contains unsupported role %q", message.Role)
		}
	}
	return input, nil
}

// computerContinuationInput returns only the observations generated after the
// most recent stored response. OpenAI retains the preceding response chain;
// replaying its computer_call items would duplicate actions against the local
// desktop. If this is the first turn, normal full-history encoding is used.
func computerContinuationInput(messages []agent.Message) (string, []inputItem, error) {
	responseIndex := -1
	responseID := ""
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.Role != agent.RoleAgent {
			continue
		}
		var state responseProviderState
		if len(message.ProviderData) == 0 || json.Unmarshal(message.ProviderData, &state) != nil || strings.TrimSpace(state.ResponseID) == "" {
			// An assistant item without a stored provider response cannot be
			// represented safely as an incremental continuation.
			input, err := encodeInput(messages)
			return "", input, err
		}
		responseIndex, responseID = index, state.ResponseID
		break
	}
	if responseIndex < 0 {
		input, err := encodeInput(messages)
		return "", input, err
	}

	calls := make(map[string]agent.ToolCall)
	for _, message := range messages {
		if message.Role != agent.RoleAgent {
			continue
		}
		for _, call := range message.ToolCalls {
			calls[call.ID] = call
		}
	}
	input := make([]inputItem, 0, len(messages)-responseIndex)
	for _, message := range messages[responseIndex+1:] {
		switch message.Role {
		case agent.RoleUser:
			items, err := encodeInput([]agent.Message{message})
			if err != nil {
				return "", nil, err
			}
			input = append(input, items...)
		case agent.RoleTool:
			if strings.TrimSpace(message.ToolCallID) == "" {
				return "", nil, errors.New("agent history contains a tool result without a call id")
			}
			call, known := calls[message.ToolCallID]
			if known && call.Kind == agent.ToolCallComputer {
				if len(message.Images) != 1 || message.Images[0].MediaType != "image/png" || len(message.Images[0].Data) == 0 {
					return "", nil, errors.New("computer call output requires exactly one transient PNG screenshot")
				}
				input = append(input, inputItem{Type: "computer_call_output", CallID: message.ToolCallID, Output: computerScreenshotOutput(message.Images[0])})
			} else {
				input = append(input, inputItem{Type: "function_call_output", CallID: message.ToolCallID, Output: message.Content})
			}
		case agent.RoleAgent:
			return "", nil, errors.New("agent history contains an unsupported assistant item after the latest OpenAI response")
		default:
			return "", nil, fmt.Errorf("agent history contains unsupported role %q", message.Role)
		}
	}
	return responseID, input, nil
}

func computerScreenshotOutput(image agent.Image) map[string]string {
	return map[string]string{"type": "computer_screenshot", "image_url": imageDataURL(image), "detail": "original"}
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
	ID    string `json:"id"`
	Usage *struct {
		Input  int64 `json:"input_tokens"`
		Output int64 `json:"output_tokens"`
	} `json:"usage"`
	Output []responseOutputItem `json:"output"`
}

type responseOutputItem struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	CallID    string            `json:"call_id"`
	Name      string            `json:"name"`
	Arguments string            `json:"arguments"`
	Actions   []json.RawMessage `json:"actions"`
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
		case "computer_call":
			if item.CallID == "" || len(item.Actions) == 0 {
				return agent.Turn{}, errors.New("OpenAI response contains an invalid computer call")
			}
			for _, action := range item.Actions {
				if !json.Valid(action) {
					return agent.Turn{}, errors.New("OpenAI response contains an invalid computer action")
				}
			}
			arguments, err := json.Marshal(struct {
				Actions []json.RawMessage `json:"actions"`
			}{Actions: item.Actions})
			if err != nil {
				return agent.Turn{}, fmt.Errorf("encode OpenAI computer action batch: %w", err)
			}
			calls = append(calls, agent.ToolCall{ID: item.CallID, ProviderID: item.ID, Kind: agent.ToolCallComputer, Name: "computer_action", Arguments: arguments})
		}
	}
	if text.Len() == 0 && len(calls) == 0 {
		return agent.Turn{}, errors.New("OpenAI response contained no output text or function calls")
	}
	turn := agent.Turn{Text: text.String(), ToolCalls: calls}
	if strings.TrimSpace(payload.ID) != "" {
		providerData, err := json.Marshal(responseProviderState{ResponseID: payload.ID})
		if err != nil {
			return agent.Turn{}, fmt.Errorf("encode OpenAI response state: %w", err)
		}
		turn.ProviderData = providerData
	}
	if payload.Usage != nil {
		turn.Usage = agent.Usage{Reported: true, InputTokens: payload.Usage.Input, OutputTokens: payload.Usage.Output}
	}
	return turn, nil
}

type responseProviderState struct {
	ResponseID string `json:"response_id"`
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
