package chatcompletions

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
)

func TestModelConvertsToolCallsAndReplaysResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role       string `json:"role"`
				ToolCallID string `json:"tool_call_id"`
			} `json:"messages"`
			Tools []json.RawMessage `json:"tools"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != "test-model" || len(body.Tools) != 1 || len(body.Messages) != 4 || body.Messages[3].ToolCallID != "call_1" {
			t.Fatalf("request = %#v", body)
		}
		_, _ = io.WriteString(writer, `{"choices":[{"message":{"content":"done","tool_calls":[{"id":"call_2","type":"function","function":{"name":"git_status","arguments":"{}"}}]}}]}`)
	}))
	defer server.Close()

	model := Model{Config: Config{APIKey: "test-key", APIKeyEnv: "TEST_API_KEY", BaseURL: server.URL, Model: "test-model", ProviderName: "Test API", Client: server.Client()}}
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
	if turn.Text != "done" || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].ID != "call_2" || turn.ToolCalls[0].Name != "git_status" {
		t.Fatalf("turn = %#v", turn)
	}
}

func TestModelUsesConfiguredAPIKeyHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("api-key") != "azure-key" || request.Header.Get("Authorization") != "" {
			t.Fatalf("headers = %#v", request.Header)
		}
		_, _ = io.WriteString(writer, `{"choices":[{"message":{"content":"done"}}]}`)
	}))
	defer server.Close()

	model := Model{Config: Config{APIKey: "azure-key", APIKeyEnv: "AZURE_OPENAI_API_KEY", BaseURL: server.URL, Model: "deployment", AuthorizationHeader: "api-key", Client: server.Client()}}
	if _, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello"}}}); err != nil {
		t.Fatalf("complete: %v", err)
	}
}
