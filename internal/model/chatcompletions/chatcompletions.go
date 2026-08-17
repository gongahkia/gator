// Package chatcompletions adapts OpenAI-compatible Chat Completions APIs to
// Gator's provider-independent turn contract.
package chatcompletions

import (
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

const maxResponseBytes = 2 * 1024 * 1024

// Config describes one OpenAI-compatible Chat Completions endpoint.
// ProviderName and APIKeyEnv are only used in clear, non-secret errors.
type Config struct {
	APIKey              string
	APIKeySource        func(context.Context) (string, error)
	APIKeyEnv           string
	BaseURL             string
	Model               string
	ProviderName        string
	AuthorizationHeader string
	AuthorizationPrefix string
	Headers             http.Header
	RequestHeaders      func(agent.TurnRequest) http.Header
	RequestSigner       func(context.Context, *http.Request, []byte) error
	Client              *http.Client
}

// Model is a stateless adapter for OpenAI-compatible Chat Completions APIs.
// The complete normalized conversation is sent for every turn so Gator owns
// resume state and providers do not need server-side conversation storage.
type Model struct {
	Config Config
}

// Complete implements agent.Model.
func (m Model) Complete(ctx context.Context, turn agent.TurnRequest) (agent.Turn, error) {
	apiKey, err := m.apiKey(ctx)
	if err != nil {
		return agent.Turn{}, err
	}
	if strings.TrimSpace(apiKey) == "" && m.Config.RequestSigner == nil {
		env := m.Config.APIKeyEnv
		if env == "" {
			env = "API key"
		}
		return agent.Turn{}, fmt.Errorf("%s is required", env)
	}
	if strings.TrimSpace(m.Config.BaseURL) == "" {
		return agent.Turn{}, errors.New("Chat Completions base URL is required")
	}
	if strings.TrimSpace(m.Config.Model) == "" {
		return agent.Turn{}, errors.New("Chat Completions model is required")
	}
	body, err := requestBody(turn, m.Config.Model)
	if err != nil {
		return agent.Turn{}, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return agent.Turn{}, fmt.Errorf("encode Chat Completions request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, m.Config.BaseURL, bytes.NewReader(payload))
	if err != nil {
		return agent.Turn{}, fmt.Errorf("create Chat Completions request: %w", err)
	}
	authHeader := m.Config.AuthorizationHeader
	if authHeader == "" {
		authHeader = "Authorization"
	}
	authPrefix := m.Config.AuthorizationPrefix
	if authHeader == "Authorization" && authPrefix == "" {
		authPrefix = "Bearer "
	}
	if strings.TrimSpace(apiKey) != "" {
		request.Header.Set(authHeader, authPrefix+apiKey)
	}
	request.Header.Set("Content-Type", "application/json")
	for name, values := range m.Config.Headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	if m.Config.RequestHeaders != nil {
		for name, values := range m.Config.RequestHeaders(turn) {
			for _, value := range values {
				request.Header.Add(name, value)
			}
		}
	}
	if m.Config.RequestSigner != nil {
		if err := m.Config.RequestSigner(ctx, request, payload); err != nil {
			return agent.Turn{}, fmt.Errorf("sign %s request: %w", m.providerName(), err)
		}
	}
	response, err := m.client().Do(request)
	if err != nil {
		return agent.Turn{}, fmt.Errorf("request %s response: %w", m.providerName(), err)
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return agent.Turn{}, fmt.Errorf("read %s response: %w", m.providerName(), err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return agent.Turn{}, describeAPIError(m.providerName(), response.StatusCode, contents)
	}
	return decodeResponse(contents, m.providerName())
}

func (m Model) apiKey(ctx context.Context) (string, error) {
	if m.Config.APIKeySource == nil {
		return m.Config.APIKey, nil
	}
	apiKey, err := m.Config.APIKeySource(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve %s credential: %w", m.providerName(), err)
	}
	return apiKey, nil
}

func (m Model) providerName() string {
	if m.Config.ProviderName != "" {
		return m.Config.ProviderName
	}
	return "Chat Completions API"
}

func (m Model) client() *http.Client {
	if m.Config.Client != nil {
		return m.Config.Client
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

type request struct {
	Model    string         `json:"model"`
	Messages []message      `json:"messages"`
	Tools    []functionTool `json:"tools,omitempty"`
}

type message struct {
	Role             string          `json:"role"`
	Content          any             `json:"content,omitempty"`
	ToolCalls        []toolCall      `json:"tool_calls,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
	Name             string          `json:"name,omitempty"`
	ReasoningContent json.RawMessage `json:"reasoning_content,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type functionTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

func requestBody(turn agent.TurnRequest, model string) (request, error) {
	messages, err := encodeMessages(turn.System, turn.Messages)
	if err != nil {
		return request{}, err
	}
	tools := make([]functionTool, 0, len(turn.Tools))
	for _, definition := range turn.Tools {
		if strings.TrimSpace(definition.Name) == "" || !json.Valid(definition.Parameters) {
			return request{}, fmt.Errorf("tool %q has invalid JSON Schema", definition.Name)
		}
		tool := functionTool{Type: "function"}
		tool.Function.Name = definition.Name
		tool.Function.Description = definition.Description
		tool.Function.Parameters = append(json.RawMessage(nil), definition.Parameters...)
		tools = append(tools, tool)
	}
	return request{Model: model, Messages: messages, Tools: tools}, nil
}

func encodeMessages(system string, source []agent.Message) ([]message, error) {
	messages := make([]message, 0, len(source)+1)
	if strings.TrimSpace(system) != "" {
		messages = append(messages, textMessage("system", system))
	}
	for _, item := range source {
		switch item.Role {
		case agent.RoleUser:
			message, err := userMessage(item)
			if err != nil {
				return nil, err
			}
			messages = append(messages, message)
		case agent.RoleAgent:
			message := message{Role: "assistant"}
			if item.Content != "" {
				message.Content = item.Content
			}
			for _, call := range item.ToolCalls {
				if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" || !json.Valid(call.Arguments) {
					return nil, errors.New("agent history contains an invalid tool call")
				}
				encoded := toolCall{ID: call.ID, Type: "function"}
				encoded.Function.Name = call.Name
				encoded.Function.Arguments = string(call.Arguments)
				message.ToolCalls = append(message.ToolCalls, encoded)
			}
			if message.Content == nil && len(message.ToolCalls) == 0 {
				return nil, errors.New("agent history contains an empty agent message")
			}
			if len(item.ProviderData) > 0 {
				var providerData struct {
					ReasoningContent json.RawMessage `json:"reasoning_content"`
				}
				if !json.Valid(item.ProviderData) || json.Unmarshal(item.ProviderData, &providerData) != nil {
					return nil, errors.New("agent history contains invalid Chat Completions provider state")
				}
				if len(providerData.ReasoningContent) > 0 && string(providerData.ReasoningContent) != "null" {
					message.ReasoningContent = append(json.RawMessage(nil), providerData.ReasoningContent...)
				}
			}
			messages = append(messages, message)
		case agent.RoleTool:
			if strings.TrimSpace(item.ToolCallID) == "" {
				return nil, errors.New("agent history contains a tool result without a call id")
			}
			messages = append(messages, message{Role: "tool", Content: item.Content, ToolCallID: item.ToolCallID, Name: item.ToolName})
		default:
			return nil, fmt.Errorf("agent history contains unsupported role %q", item.Role)
		}
	}
	return messages, nil
}

func textMessage(role, content string) message {
	return message{Role: role, Content: content}
}

func userMessage(item agent.Message) (message, error) {
	if len(item.Images) == 0 && len(item.Attachments) == 0 {
		return textMessage("user", item.Content), nil
	}
	content := make([]chatContentPart, 0, len(item.Images)+len(item.Attachments)+1)
	if item.Content != "" {
		content = append(content, chatContentPart{Type: "text", Text: item.Content})
	}
	for _, image := range item.Images {
		content = append(content, chatContentPart{Type: "image_url", ImageURL: &chatImageURL{URL: "data:" + image.MediaType + ";base64," + base64.StdEncoding.EncodeToString(image.Data)}})
	}
	for _, attachment := range item.Attachments {
		if attachment.MediaType != "text/plain" {
			return message{}, fmt.Errorf("document attachment %q requires the OpenAI Responses, Anthropic Messages, or Gemini provider", attachment.Name)
		}
		content = append(content, chatContentPart{Type: "text", Text: agent.AttachmentText(attachment)})
	}
	return message{Role: "user", Content: content}, nil
}

type chatContentPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *chatImageURL `json:"image_url,omitempty"`
}

type chatImageURL struct {
	URL string `json:"url"`
}

type response struct {
	Choices []struct {
		Message struct {
			Content          *string         `json:"content"`
			ToolCalls        []toolCall      `json:"tool_calls"`
			ReasoningContent json.RawMessage `json:"reasoning_content"`
		} `json:"message"`
	} `json:"choices"`
}

func decodeResponse(contents []byte, provider string) (agent.Turn, error) {
	var payload response
	if err := json.Unmarshal(contents, &payload); err != nil {
		return agent.Turn{}, fmt.Errorf("decode %s response: %w", provider, err)
	}
	if len(payload.Choices) == 0 {
		return agent.Turn{}, fmt.Errorf("%s response contained no choices", provider)
	}
	message := payload.Choices[0].Message
	turn := agent.Turn{}
	if message.Content != nil {
		turn.Text = *message.Content
	}
	if len(message.ReasoningContent) > 0 && string(message.ReasoningContent) != "null" {
		providerData, err := json.Marshal(struct {
			ReasoningContent json.RawMessage `json:"reasoning_content"`
		}{ReasoningContent: message.ReasoningContent})
		if err != nil {
			return agent.Turn{}, fmt.Errorf("encode %s reasoning state: %w", provider, err)
		}
		turn.ProviderData = providerData
	}
	for _, item := range message.ToolCalls {
		if item.Type != "" && item.Type != "function" || strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Function.Name) == "" || !json.Valid([]byte(item.Function.Arguments)) {
			return agent.Turn{}, fmt.Errorf("%s response contains an invalid function call", provider)
		}
		turn.ToolCalls = append(turn.ToolCalls, agent.ToolCall{ID: item.ID, ProviderID: item.ID, Name: item.Function.Name, Arguments: json.RawMessage(item.Function.Arguments)})
	}
	if strings.TrimSpace(turn.Text) == "" && len(turn.ToolCalls) == 0 {
		return agent.Turn{}, fmt.Errorf("%s response contained no output text or function calls", provider)
	}
	return turn, nil
}

func describeAPIError(provider string, status int, contents []byte) error {
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(contents, &payload); err == nil && payload.Error.Message != "" {
		return fmt.Errorf("%s returned HTTP %d: %s", provider, status, payload.Error.Message)
	}
	return fmt.Errorf("%s returned HTTP %d", provider, status)
}
