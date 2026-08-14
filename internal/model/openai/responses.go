// Package openai implements Gator's native adapter for the OpenAI Responses
// API. It owns only protocol conversion; the agent loop remains in internal/agent.
package openai

import (
	"bytes"
	"context"
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
	APIKey  string
	Model   string
	BaseURL string
	Client  *http.Client
}

// Complete implements agent.Model.
func (r Responses) Complete(ctx context.Context, turn agent.TurnRequest) (agent.Turn, error) {
	if strings.TrimSpace(r.APIKey) == "" {
		return agent.Turn{}, errors.New("OPENAI_API_KEY is required")
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
	request.Header.Set("Authorization", "Bearer "+r.APIKey)
	request.Header.Set("Content-Type", "application/json")
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
	Content   string `json:"content,omitempty"`
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
			if message.Content != "" {
				input = append(input, inputItem{Role: string(message.Role), Content: message.Content})
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
