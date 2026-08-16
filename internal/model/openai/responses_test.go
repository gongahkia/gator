package openai

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

func TestResponsesCompleteConvertsToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		var body responseRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != "test-model" || body.Store || len(body.Tools) != 1 || body.Tools[0].Name != "read_file" {
			t.Fatalf("request = %#v", body)
		}
		if body.Tools[0].Strict {
			t.Fatal("tool schema should not be marked strict until every tool schema is strict-compatible")
		}
		_, _ = io.WriteString(writer, `{"output":[{"id":"fc_item","type":"function_call","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"README.md\"}"}]}`)
	}))
	defer server.Close()

	model := Responses{APIKey: "test-key", Model: "test-model", BaseURL: server.URL, Client: server.Client()}
	turn, err := model.Complete(context.Background(), agent.TurnRequest{
		System:   "be precise",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "read the readme"}},
		Tools:    []agent.ToolDefinition{{Name: "read_file", Parameters: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if len(turn.ToolCalls) != 1 {
		t.Fatalf("tool calls = %#v", turn.ToolCalls)
	}
	call := turn.ToolCalls[0]
	if call.ID != "call_1" || call.ProviderID != "fc_item" || call.Name != "read_file" || string(call.Arguments) != `{"path":"README.md"}` {
		t.Fatalf("tool call = %#v", call)
	}
}

func TestResponsesCompleteReplaysFunctionCallAndOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body responseRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(body.Input) != 3 {
			t.Fatalf("input = %#v", body.Input)
		}
		call, output := body.Input[1], body.Input[2]
		if call.Type != "function_call" || call.ID != "fc_item" || call.CallID != "call_1" || call.Arguments != `{"path":"README.md"}` {
			t.Fatalf("replayed call = %#v", call)
		}
		if output.Type != "function_call_output" || output.CallID != "call_1" || output.Output != `{"ok":true}` {
			t.Fatalf("replayed output = %#v", output)
		}
		_, _ = io.WriteString(writer, `{"output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}]}`)
	}))
	defer server.Close()

	model := Responses{APIKey: "test-key", BaseURL: server.URL, Client: server.Client()}
	turn, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{
		{Role: agent.RoleUser, Content: "read the readme"},
		{Role: agent.RoleAgent, ToolCalls: []agent.ToolCall{{ID: "call_1", ProviderID: "fc_item", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`)}}},
		{Role: agent.RoleTool, ToolCallID: "call_1", ToolName: "read_file", Content: `{"ok":true}`},
	}})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if turn.Text != "done" || len(turn.ToolCalls) != 0 {
		t.Fatalf("turn = %#v", turn)
	}
}

func TestResponsesEncodesImageInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body responseRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		content, ok := body.Input[0].Content.([]any)
		if !ok || len(content) != 2 {
			t.Fatalf("image content = %#v", body.Input[0].Content)
		}
		_, _ = io.WriteString(writer, `{"output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}]}`)
	}))
	defer server.Close()
	model := Responses{APIKey: "test-key", BaseURL: server.URL, Client: server.Client()}
	if _, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "inspect", Images: []agent.Image{{Name: "screen.png", MediaType: "image/png", Data: []byte("png")}}}}}); err != nil {
		t.Fatalf("complete image input: %v", err)
	}
}

func TestResponsesCompleteDescribesAPIErrorsWithoutKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(writer, `{"error":{"message":"rate limit reached"}}`)
	}))
	defer server.Close()

	model := Responses{APIKey: "do-not-leak", BaseURL: server.URL, Client: server.Client()}
	_, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello"}}})
	if err == nil || !strings.Contains(err.Error(), "HTTP 429: rate limit reached") || strings.Contains(err.Error(), "do-not-leak") {
		t.Fatalf("API error = %v", err)
	}
}

func TestResponsesRequiresAPIKey(t *testing.T) {
	_, err := (Responses{}).Complete(context.Background(), agent.TurnRequest{})
	if err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatalf("missing-key error = %v", err)
	}
}

func TestResponsesCompleteStreamForwardsDeltasAndReturnsCompletedTurn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body responseRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode stream request: %v", err)
		}
		if !body.Stream || request.Header.Get("Accept") != "text/event-stream" {
			t.Fatalf("stream request = %#v, accept = %q", body, request.Header.Get("Accept"))
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, "event: response.output_text.delta\n")
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello \"}\n\n")
		_, _ = io.WriteString(writer, "event: response.output_text.delta\n")
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"world\"}\n\n")
		_, _ = io.WriteString(writer, "event: response.completed\n")
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello world\"}]}]}}\n\n")
	}))
	defer server.Close()

	model := Responses{APIKey: "test-key", BaseURL: server.URL, Client: server.Client()}
	var deltas []string
	turn, err := model.CompleteStream(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello"}}}, func(delta string) { deltas = append(deltas, delta) })
	if err != nil {
		t.Fatalf("complete stream: %v", err)
	}
	if strings.Join(deltas, "") != "hello world" || turn.Text != "hello world" {
		t.Fatalf("deltas = %#v, turn = %#v", deltas, turn)
	}
}
