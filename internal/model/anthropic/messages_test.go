package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
)

func TestMessagesConvertsToolCallsAndGroupsToolResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("x-api-key") != "test-key" || request.Header.Get("anthropic-version") != "2023-06-01" {
			t.Fatalf("headers = %#v", request.Header)
		}
		var body requestBodyView
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != "claude-test" || body.System != "be precise" || len(body.Tools) != 1 || len(body.Messages) != 3 {
			t.Fatalf("request = %#v", body)
		}
		if body.Messages[1].Role != "assistant" || body.Messages[2].Role != "user" || body.Messages[2].Content[0].ToolUseID != "call_1" {
			t.Fatalf("messages = %#v", body.Messages)
		}
		_, _ = io.WriteString(writer, `{"content":[{"type":"text","text":"reading"},{"type":"tool_use","id":"call_2","name":"git_status","input":{}}]}`)
	}))
	defer server.Close()

	model := Messages{APIKey: "test-key", Model: "claude-test", BaseURL: server.URL, Client: server.Client()}
	turn, err := model.Complete(context.Background(), agent.TurnRequest{
		System: "be precise",
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "inspect"},
			{Role: agent.RoleAgent, ToolCalls: []agent.ToolCall{{ID: "call_1", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`)}}},
			{Role: agent.RoleTool, ToolCallID: "call_1", ToolName: "read_file", Content: `{"ok":true}`},
		},
		Tools: []agent.ToolDefinition{{Name: "git_status", Parameters: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if turn.Text != "reading" || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].ID != "call_2" || turn.ToolCalls[0].Name != "git_status" {
		t.Fatalf("turn = %#v", turn)
	}
}

func TestMessagesStreamAssemblesToolArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body requestBodyView
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil || !body.Stream {
			t.Fatalf("stream request = %#v, err = %v", body, err)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, "event: content_block_start\n")
		_, _ = io.WriteString(writer, "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		_, _ = io.WriteString(writer, "event: content_block_delta\n")
		_, _ = io.WriteString(writer, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n")
		_, _ = io.WriteString(writer, "event: content_block_start\n")
		_, _ = io.WriteString(writer, "data: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"call_1\",\"name\":\"git_status\",\"input\":{}}}\n\n")
		_, _ = io.WriteString(writer, "event: content_block_delta\n")
		_, _ = io.WriteString(writer, "data: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{}\"}}\n\n")
		_, _ = io.WriteString(writer, "event: message_stop\n")
		_, _ = io.WriteString(writer, "data: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	model := Messages{APIKey: "test-key", Model: "claude-test", BaseURL: server.URL, Client: server.Client()}
	var deltas []string
	turn, err := model.CompleteStream(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello"}}}, func(delta string) { deltas = append(deltas, delta) })
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if strings.Join(deltas, "") != "hello" || turn.Text != "hello" || len(turn.ToolCalls) != 1 || string(turn.ToolCalls[0].Arguments) != "{}" {
		t.Fatalf("deltas = %#v, turn = %#v", deltas, turn)
	}
}

func TestMessagesEncodesImageInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body requestBodyView
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(body.Messages) != 1 || len(body.Messages[0].Content) != 2 || body.Messages[0].Content[1].Source == nil || body.Messages[0].Content[1].Source.MediaType != "image/png" {
			t.Fatalf("image request = %#v", body.Messages)
		}
		_, _ = io.WriteString(writer, `{"content":[{"type":"text","text":"done"}]}`)
	}))
	defer server.Close()
	model := Messages{APIKey: "test-key", Model: "claude-test", BaseURL: server.URL, Client: server.Client()}
	if _, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "inspect", Images: []agent.Image{{Name: "screen.png", MediaType: "image/png", Data: []byte("png")}}}}}); err != nil {
		t.Fatalf("complete image input: %v", err)
	}
}

type requestBodyView struct {
	Model    string         `json:"model"`
	System   string         `json:"system"`
	Messages []message      `json:"messages"`
	Tools    []functionTool `json:"tools"`
	Stream   bool           `json:"stream"`
}
