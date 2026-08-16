// Package gemini implements Gator's stateless adapter for Gemini's
// GenerateContent API.
package gemini

import (
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
	defaultBaseURL  = "https://generativelanguage.googleapis.com/v1beta"
	maxResponseSize = 2 * 1024 * 1024
)

// GenerateContent is a stateless Gemini GenerateContent adapter. It retains
// the exact provider content for agent messages because Gemini thought
// signatures must be replayed alongside function calls on later turns.
type GenerateContent struct {
	APIKey  string
	Model   string
	BaseURL string
	Client  *http.Client
}

// Complete implements agent.Model.
func (g GenerateContent) Complete(ctx context.Context, turn agent.TurnRequest) (agent.Turn, error) {
	if strings.TrimSpace(g.APIKey) == "" {
		return agent.Turn{}, errors.New("GEMINI_API_KEY is required")
	}
	if strings.TrimSpace(g.Model) == "" {
		return agent.Turn{}, errors.New("Gemini model is required")
	}
	body, err := requestBody(turn)
	if err != nil {
		return agent.Turn{}, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return agent.Turn{}, fmt.Errorf("encode Gemini request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint(), bytes.NewReader(payload))
	if err != nil {
		return agent.Turn{}, fmt.Errorf("create Gemini request: %w", err)
	}
	request.Header.Set("x-goog-api-key", g.APIKey)
	request.Header.Set("content-type", "application/json")
	response, err := g.client().Do(request)
	if err != nil {
		return agent.Turn{}, fmt.Errorf("request Gemini response: %w", err)
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize))
	if err != nil {
		return agent.Turn{}, fmt.Errorf("read Gemini response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return agent.Turn{}, describeAPIError(response.StatusCode, contents)
	}
	return decodeResponse(contents)
}

func (g GenerateContent) endpoint() string {
	base := strings.TrimRight(g.BaseURL, "/")
	if base == "" {
		base = defaultBaseURL
	}
	return base + "/models/" + url.PathEscape(g.Model) + ":generateContent"
}

func (g GenerateContent) client() *http.Client {
	if g.Client != nil {
		return g.Client
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

type request struct {
	SystemInstruction *content  `json:"systemInstruction,omitempty"`
	Contents          []content `json:"contents"`
	Tools             []tool    `json:"tools,omitempty"`
}

type tool struct {
	FunctionDeclarations []functionDeclaration `json:"functionDeclarations"`
}

type functionDeclaration struct {
	Name                 string          `json:"name"`
	Description          string          `json:"description,omitempty"`
	ParametersJSONSchema json.RawMessage `json:"parametersJsonSchema"`
}

type content struct {
	Role  string            `json:"role,omitempty"`
	Parts []json.RawMessage `json:"parts"`
}

func requestBody(turn agent.TurnRequest) (request, error) {
	contents, err := encodeContents(turn.Messages)
	if err != nil {
		return request{}, err
	}
	tools := make([]tool, 0, 1)
	if len(turn.Tools) > 0 {
		declarations := make([]functionDeclaration, 0, len(turn.Tools))
		for _, definition := range turn.Tools {
			if strings.TrimSpace(definition.Name) == "" || !json.Valid(definition.Parameters) {
				return request{}, fmt.Errorf("tool %q has invalid JSON Schema", definition.Name)
			}
			declarations = append(declarations, functionDeclaration{
				Name:                 definition.Name,
				Description:          definition.Description,
				ParametersJSONSchema: append(json.RawMessage(nil), definition.Parameters...),
			})
		}
		tools = append(tools, tool{FunctionDeclarations: declarations})
	}
	body := request{Contents: contents, Tools: tools}
	if strings.TrimSpace(turn.System) != "" {
		body.SystemInstruction = &content{Parts: []json.RawMessage{partText(turn.System)}}
	}
	return body, nil
}

func encodeContents(source []agent.Message) ([]content, error) {
	contents := make([]content, 0, len(source))
	for index := 0; index < len(source); {
		item := source[index]
		switch item.Role {
		case agent.RoleUser:
			parts := make([]json.RawMessage, 0, len(item.Images)+len(item.Attachments)+1)
			if item.Content != "" {
				parts = append(parts, partText(item.Content))
			}
			for _, image := range item.Images {
				parts = append(parts, partImage(image))
			}
			for _, attachment := range item.Attachments {
				switch attachment.MediaType {
				case "application/pdf":
					parts = append(parts, partAttachment(attachment))
				case "text/plain":
					parts = append(parts, partText(agent.AttachmentText(attachment)))
				default:
					return nil, fmt.Errorf("Gemini attachment %q has unsupported media type %q", attachment.Name, attachment.MediaType)
				}
			}
			if len(parts) == 0 {
				return nil, errors.New("agent history contains an empty user message")
			}
			contents = append(contents, content{Role: "user", Parts: parts})
			index++
		case agent.RoleAgent:
			if json.Valid(item.ProviderData) {
				var preserved content
				if err := json.Unmarshal(item.ProviderData, &preserved); err != nil || len(preserved.Parts) == 0 {
					return nil, errors.New("agent history contains invalid Gemini provider data")
				}
				contents = append(contents, preserved)
				index++
				continue
			}
			parts := make([]json.RawMessage, 0, len(item.ToolCalls)+1)
			if item.Content != "" {
				parts = append(parts, partText(item.Content))
			}
			for _, call := range item.ToolCalls {
				if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" || !json.Valid(call.Arguments) {
					return nil, errors.New("agent history contains an invalid tool call")
				}
				parts = append(parts, partFunctionCall(call.ID, call.Name, call.Arguments))
			}
			if len(parts) == 0 {
				return nil, errors.New("agent history contains an empty agent message")
			}
			contents = append(contents, content{Role: "model", Parts: parts})
			index++
		case agent.RoleTool:
			parts := make([]json.RawMessage, 0)
			for index < len(source) && source[index].Role == agent.RoleTool {
				result := source[index]
				if strings.TrimSpace(result.ToolCallID) == "" || strings.TrimSpace(result.ToolName) == "" {
					return nil, errors.New("agent history contains a tool result without a call id and name")
				}
				parts = append(parts, partFunctionResponse(result.ToolCallID, result.ToolName, result.Content))
				index++
			}
			contents = append(contents, content{Role: "tool", Parts: parts})
		default:
			return nil, fmt.Errorf("agent history contains unsupported role %q", item.Role)
		}
	}
	return contents, nil
}

func partText(text string) json.RawMessage {
	payload, _ := json.Marshal(struct {
		Text string `json:"text"`
	}{Text: text})
	return payload
}

func partImage(image agent.Image) json.RawMessage {
	return partInlineData(image.MediaType, image.Data)
}

func partAttachment(attachment agent.Attachment) json.RawMessage {
	return partInlineData(attachment.MediaType, attachment.Data)
}

func partInlineData(mediaType string, data []byte) json.RawMessage {
	var encoded struct {
		InlineData struct {
			MIMEType string `json:"mimeType"`
			Data     string `json:"data"`
		} `json:"inlineData"`
	}
	encoded.InlineData.MIMEType = mediaType
	encoded.InlineData.Data = base64.StdEncoding.EncodeToString(data)
	payload, _ := json.Marshal(encoded)
	return payload
}

func partFunctionCall(id, name string, args json.RawMessage) json.RawMessage {
	var encoded struct {
		FunctionCall struct {
			ID   string          `json:"id,omitempty"`
			Name string          `json:"name"`
			Args json.RawMessage `json:"args"`
		} `json:"functionCall"`
	}
	encoded.FunctionCall.ID = id
	encoded.FunctionCall.Name = name
	encoded.FunctionCall.Args = args
	payload, _ := json.Marshal(encoded)
	return payload
}

func partFunctionResponse(id, name, result string) json.RawMessage {
	var encoded struct {
		FunctionResponse struct {
			ID       string         `json:"id,omitempty"`
			Name     string         `json:"name"`
			Response map[string]any `json:"response"`
		} `json:"functionResponse"`
	}
	encoded.FunctionResponse.ID = id
	encoded.FunctionResponse.Name = name
	encoded.FunctionResponse.Response = map[string]any{"result": result}
	payload, _ := json.Marshal(encoded)
	return payload
}

type response struct {
	Candidates []struct {
		Content json.RawMessage `json:"content"`
	} `json:"candidates"`
}

func decodeResponse(contents []byte) (agent.Turn, error) {
	var payload response
	if err := json.Unmarshal(contents, &payload); err != nil {
		return agent.Turn{}, fmt.Errorf("decode Gemini response: %w", err)
	}
	if len(payload.Candidates) == 0 || !json.Valid(payload.Candidates[0].Content) {
		return agent.Turn{}, errors.New("Gemini response contained no candidate content")
	}
	var candidate content
	if err := json.Unmarshal(payload.Candidates[0].Content, &candidate); err != nil {
		return agent.Turn{}, fmt.Errorf("decode Gemini candidate content: %w", err)
	}
	turn := agent.Turn{ProviderData: append(json.RawMessage(nil), payload.Candidates[0].Content...)}
	var text strings.Builder
	for _, rawPart := range candidate.Parts {
		var part struct {
			Text         string `json:"text"`
			FunctionCall *struct {
				ID   string          `json:"id"`
				Name string          `json:"name"`
				Args json.RawMessage `json:"args"`
			} `json:"functionCall"`
		}
		if err := json.Unmarshal(rawPart, &part); err != nil {
			return agent.Turn{}, fmt.Errorf("decode Gemini candidate part: %w", err)
		}
		if part.Text != "" {
			text.WriteString(part.Text)
		}
		if part.FunctionCall != nil {
			call := part.FunctionCall
			if strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Name) == "" || !json.Valid(call.Args) {
				return agent.Turn{}, errors.New("Gemini response contains an invalid function call")
			}
			turn.ToolCalls = append(turn.ToolCalls, agent.ToolCall{ID: call.ID, ProviderID: call.ID, Name: call.Name, Arguments: append(json.RawMessage(nil), call.Args...)})
		}
	}
	turn.Text = text.String()
	if strings.TrimSpace(turn.Text) == "" && len(turn.ToolCalls) == 0 {
		return agent.Turn{}, errors.New("Gemini response contained no output text or function calls")
	}
	return turn, nil
}

func describeAPIError(status int, contents []byte) error {
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(contents, &payload); err == nil && payload.Error.Message != "" {
		return fmt.Errorf("Gemini API returned HTTP %d: %s", status, payload.Error.Message)
	}
	return fmt.Errorf("Gemini API returned HTTP %d", status)
}
