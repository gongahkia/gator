package radius

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

func TestMessagesDiscoversGatewayAndConvertsStreamingToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/config":
			if request.Header.Get("authorization") != "Bearer radius-key" {
				t.Fatalf("config headers = %#v", request.Header)
			}
			_, _ = io.WriteString(writer, `{"baseUrl":"http://`+request.Host+`/model"}`)
		case "/model/messages":
			if request.Header.Get("authorization") != "Bearer radius-key" || request.Header.Get("accept") != "text/event-stream" {
				t.Fatalf("message headers = %#v", request.Header)
			}
			var payload struct {
				Model string `json:"model"`
				Context struct {
					SystemPrompt string            `json:"systemPrompt"`
					Messages     []json.RawMessage `json:"messages"`
					Tools        []struct {
						Name string `json:"name"`
					} `json:"tools"`
				} `json:"context"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatalf("decode Radius request: %v", err)
			}
			if payload.Model != "auto" || payload.Context.SystemPrompt != "be precise" || len(payload.Context.Messages) != 3 || len(payload.Context.Tools) != 1 || payload.Context.Tools[0].Name != "git_status" {
				t.Fatalf("Radius payload = %#v", payload)
			}
			writer.Header().Set("content-type", "text/event-stream")
			_, _ = io.WriteString(writer, "data: {\"type\":\"start\"}\n\n")
			_, _ = io.WriteString(writer, "data: {\"type\":\"text_delta\",\"delta\":\"checking \"}\n\n")
			_, _ = io.WriteString(writer, "data: {\"type\":\"toolcall_end\",\"toolCall\":{\"id\":\"call_1\",\"name\":\"git_status\",\"arguments\":{}}}\n\n")
			_, _ = io.WriteString(writer, "data: {\"type\":\"done\"}\n\n")
		default:
			t.Fatalf("path = %q", request.URL.Path)
		}
	}))
	defer server.Close()
	model := Messages{APIKey: "radius-key", Model: "auto", Gateway: server.URL, Client: server.Client()}
	var deltas []string
	turn, err := model.CompleteStream(context.Background(), agent.TurnRequest{
		System: "be precise",
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "inspect"},
			{Role: agent.RoleAgent, ToolCalls: []agent.ToolCall{{ID: "prior", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`)}}},
			{Role: agent.RoleTool, ToolCallID: "prior", ToolName: "read_file", Content: "contents"},
		},
		Tools: []agent.ToolDefinition{{Name: "git_status", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}, func(delta string) { deltas = append(deltas, delta) })
	if err != nil {
		t.Fatalf("complete Radius stream: %v", err)
	}
	if strings.Join(deltas, "") != "checking " || turn.Text != "checking " || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].ID != "call_1" || string(turn.ToolCalls[0].Arguments) != "{}" {
		t.Fatalf("Radius turn = %#v, deltas = %#v", turn, deltas)
	}
}

func TestMessagesRejectsMissingTerminalEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("content-type", "text/event-stream")
		_, _ = io.WriteString(writer, "data: {\"type\":\"text_delta\",\"delta\":\"partial\"}\n\n")
	}))
	defer server.Close()
	_, err := (Messages{APIKey: "radius-key", Model: "auto", BaseURL: server.URL, Client: server.Client()}).Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "inspect"}}})
	if err == nil || !strings.Contains(err.Error(), "without a terminal event") {
		t.Fatalf("missing-terminal error = %v", err)
	}
}
