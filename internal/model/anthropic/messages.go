// Package anthropic implements Gator's adapter for the Anthropic Messages API.
package anthropic

import (
	"bufio"
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
	defaultBaseURL  = "https://api.anthropic.com/v1/messages"
	defaultMaxToken = 8192
	maxResponseSize = 2 * 1024 * 1024
)

// Messages is a stateless Anthropic Messages API adapter.
type Messages struct {
	APIKey  string
	Model   string
	BaseURL string
	Client  *http.Client
}

// Complete implements agent.Model.
func (m Messages) Complete(ctx context.Context, turn agent.TurnRequest) (agent.Turn, error) {
	response, err := m.do(ctx, turn, false)
	if err != nil {
		return agent.Turn{}, err
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize))
	if err != nil {
		return agent.Turn{}, fmt.Errorf("read Anthropic response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return agent.Turn{}, describeAPIError(response.StatusCode, contents)
	}
	return decodeResponse(contents)
}

// CompleteStream implements agent.StreamingModel using the Messages SSE event
// stream. Tool call arguments are assembled only after their complete JSON is
// received, so the core never receives partial tool requests.
func (m Messages) CompleteStream(ctx context.Context, turn agent.TurnRequest, onDelta func(string)) (agent.Turn, error) {
	response, err := m.do(ctx, turn, true)
	if err != nil {
		return agent.Turn{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		contents, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseSize))
		if readErr != nil {
			return agent.Turn{}, fmt.Errorf("read Anthropic stream error: %w", readErr)
		}
		return agent.Turn{}, describeAPIError(response.StatusCode, contents)
	}
	return decodeSSE(response.Body, onDelta)
}

func (m Messages) do(ctx context.Context, turn agent.TurnRequest, stream bool) (*http.Response, error) {
	if strings.TrimSpace(m.APIKey) == "" {
		return nil, errors.New("ANTHROPIC_API_KEY is required")
	}
	if strings.TrimSpace(m.Model) == "" {
		return nil, errors.New("Anthropic model is required")
	}
	body, err := requestBody(turn, m.Model)
	if err != nil {
		return nil, err
	}
	body.Stream = stream
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode Anthropic request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create Anthropic request: %w", err)
	}
	request.Header.Set("x-api-key", m.APIKey)
	request.Header.Set("anthropic-version", "2023-06-01")
	request.Header.Set("content-type", "application/json")
	if stream {
		request.Header.Set("accept", "text/event-stream")
	}
	response, err := m.client().Do(request)
	if err != nil {
		return nil, fmt.Errorf("request Anthropic response: %w", err)
	}
	return response, nil
}

func (m Messages) baseURL() string {
	if strings.TrimSpace(m.BaseURL) != "" {
		return m.BaseURL
	}
	return defaultBaseURL
}

func (m Messages) client() *http.Client {
	if m.Client != nil {
		return m.Client
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

type request struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system,omitempty"`
	Messages  []message       `json:"messages"`
	Tools     []functionTool  `json:"tools,omitempty"`
	Stream    bool            `json:"stream,omitempty"`
	ToolChoice json.RawMessage `json:"tool_choice,omitempty"`
}

type message struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type functionTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

func requestBody(turn agent.TurnRequest, model string) (request, error) {
	messages, err := encodeMessages(turn.Messages)
	if err != nil {
		return request{}, err
	}
	tools := make([]functionTool, 0, len(turn.Tools))
	for _, definition := range turn.Tools {
		if strings.TrimSpace(definition.Name) == "" || !json.Valid(definition.Parameters) {
			return request{}, fmt.Errorf("tool %q has invalid JSON Schema", definition.Name)
		}
		tools = append(tools, functionTool{Name: definition.Name, Description: definition.Description, InputSchema: append(json.RawMessage(nil), definition.Parameters...)})
	}
	return request{Model: model, MaxTokens: defaultMaxToken, System: turn.System, Messages: messages, Tools: tools}, nil
}

func encodeMessages(source []agent.Message) ([]message, error) {
	messages := make([]message, 0, len(source))
	for index := 0; index < len(source); {
		item := source[index]
		switch item.Role {
		case agent.RoleUser:
			appendMessage(&messages, "user", []contentBlock{{Type: "text", Text: item.Content}})
			index++
		case agent.RoleAgent:
			blocks := make([]contentBlock, 0, len(item.ToolCalls)+1)
			if item.Content != "" {
				blocks = append(blocks, contentBlock{Type: "text", Text: item.Content})
			}
			for _, call := range item.ToolCalls {
				if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" || !json.Valid(call.Arguments) {
					return nil, errors.New("agent history contains an invalid tool call")
				}
				blocks = append(blocks, contentBlock{Type: "tool_use", ID: call.ID, Name: call.Name, Input: append(json.RawMessage(nil), call.Arguments...)})
			}
			if len(blocks) == 0 {
				return nil, errors.New("agent history contains an empty agent message")
			}
			appendMessage(&messages, "assistant", blocks)
			index++
		case agent.RoleTool:
			blocks := make([]contentBlock, 0)
			for index < len(source) && source[index].Role == agent.RoleTool {
				result := source[index]
				if strings.TrimSpace(result.ToolCallID) == "" {
					return nil, errors.New("agent history contains a tool result without a call id")
				}
				blocks = append(blocks, contentBlock{Type: "tool_result", ToolUseID: result.ToolCallID, Content: result.Content})
				index++
			}
			appendMessage(&messages, "user", blocks)
		default:
			return nil, fmt.Errorf("agent history contains unsupported role %q", item.Role)
		}
	}
	return messages, nil
}

func appendMessage(messages *[]message, role string, blocks []contentBlock) {
	if len(*messages) > 0 && (*messages)[len(*messages)-1].Role == role {
		(*messages)[len(*messages)-1].Content = append((*messages)[len(*messages)-1].Content, blocks...)
		return
	}
	*messages = append(*messages, message{Role: role, Content: blocks})
}

type response struct {
	Content []contentBlock `json:"content"`
}

func decodeResponse(contents []byte) (agent.Turn, error) {
	var payload response
	if err := json.Unmarshal(contents, &payload); err != nil {
		return agent.Turn{}, fmt.Errorf("decode Anthropic response: %w", err)
	}
	return turnFromBlocks(payload.Content)
}

func turnFromBlocks(blocks []contentBlock) (agent.Turn, error) {
	var text strings.Builder
	turn := agent.Turn{}
	for _, block := range blocks {
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
		case "tool_use":
			if strings.TrimSpace(block.ID) == "" || strings.TrimSpace(block.Name) == "" || !json.Valid(block.Input) {
				return agent.Turn{}, errors.New("Anthropic response contains an invalid tool call")
			}
			turn.ToolCalls = append(turn.ToolCalls, agent.ToolCall{ID: block.ID, ProviderID: block.ID, Name: block.Name, Arguments: append(json.RawMessage(nil), block.Input...)})
		}
	}
	turn.Text = text.String()
	if strings.TrimSpace(turn.Text) == "" && len(turn.ToolCalls) == 0 {
		return agent.Turn{}, errors.New("Anthropic response contained no output text or tool calls")
	}
	return turn, nil
}

func decodeSSE(reader io.Reader, onDelta func(string)) (agent.Turn, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxResponseSize)
	var eventType string
	var dataLines []string
	blocks := make(map[int]contentBlock)
	var order []int
	stopped := false
	flush := func() error {
		if len(dataLines) == 0 {
			eventType = ""
			return nil
		}
		data := strings.Join(dataLines, "\n")
		dataLines = nil
		var event struct {
			Type         string `json:"type"`
			Index        int    `json:"index"`
			ContentBlock contentBlock `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return fmt.Errorf("decode Anthropic stream event: %w", err)
		}
		if event.Type == "" {
			event.Type = eventType
		}
		switch event.Type {
		case "content_block_start":
			blocks[event.Index] = event.ContentBlock
			order = append(order, event.Index)
		case "content_block_delta":
			block := blocks[event.Index]
			switch event.Delta.Type {
			case "text_delta":
				block.Text += event.Delta.Text
				if onDelta != nil && event.Delta.Text != "" {
					onDelta(event.Delta.Text)
				}
			case "input_json_delta":
				block.Input = append(block.Input, event.Delta.PartialJSON...)
			}
			blocks[event.Index] = block
		case "message_stop":
			stopped = true
		case "error":
			if event.Error.Message != "" {
				return fmt.Errorf("Anthropic stream failed: %s", event.Error.Message)
			}
			return errors.New("Anthropic stream failed")
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
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return agent.Turn{}, fmt.Errorf("read Anthropic event stream: %w", err)
	}
	if err := flush(); err != nil {
		return agent.Turn{}, err
	}
	if !stopped {
		return agent.Turn{}, errors.New("Anthropic event stream ended without message_stop")
	}
	content := make([]contentBlock, 0, len(order))
	for _, index := range order {
		content = append(content, blocks[index])
	}
	return turnFromBlocks(content)
}

func describeAPIError(status int, contents []byte) error {
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(contents, &payload); err == nil && payload.Error.Message != "" {
		return fmt.Errorf("Anthropic Messages API returned HTTP %d: %s", status, payload.Error.Message)
	}
	return fmt.Errorf("Anthropic Messages API returned HTTP %d", status)
}
