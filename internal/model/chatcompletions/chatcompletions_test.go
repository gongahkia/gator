package chatcompletions

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

func TestModelPreservesReasoningContentAcrossToolCalls(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		var body struct {
			Messages []struct {
				Role             string          `json:"role"`
				ReasoningContent json.RawMessage `json:"reasoning_content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if requests == 2 {
			if len(body.Messages) < 2 || string(body.Messages[1].ReasoningContent) != `"private reasoning"` {
				t.Fatalf("reasoning content was not replayed: %#v", body.Messages)
			}
		}
		_, _ = io.WriteString(writer, `{"choices":[{"message":{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{}"}}],"reasoning_content":"private reasoning"}}]}`)
	}))
	defer server.Close()

	model := Model{Config: Config{APIKey: "test-key", BaseURL: server.URL, Model: "test-model", Client: server.Client()}}
	first, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "inspect"}}})
	if err != nil {
		t.Fatalf("first completion: %v", err)
	}
	if string(first.ProviderData) != `{"reasoning_content":"private reasoning"}` {
		t.Fatalf("provider state = %s", first.ProviderData)
	}
	if _, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{
		{Role: agent.RoleUser, Content: "inspect"},
		{Role: agent.RoleAgent, ToolCalls: first.ToolCalls, ProviderData: first.ProviderData},
		{Role: agent.RoleTool, ToolCallID: "call_1", ToolName: "read_file", Content: `{"ok":true}`},
	}}); err != nil {
		t.Fatalf("replayed completion: %v", err)
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

func TestModelAllowsExplicitKeylessEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(writer, `{"choices":[{"message":{"content":"done"}}]}`)
	}))
	defer server.Close()
	model := Model{Config: Config{AllowEmptyAPIKey: true, BaseURL: server.URL, Model: "local-model", Client: server.Client()}}
	if _, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello"}}}); err != nil {
		t.Fatalf("complete keyless endpoint: %v", err)
	}
}

func TestModelResolvesCredentialForEachRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Header.Get("Authorization") != "Bearer refreshed-key" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(writer, `{"choices":[{"message":{"content":"done"}}]}`)
	}))
	defer server.Close()

	model := Model{Config: Config{
		APIKeyEnv: "Google Cloud credentials",
		APIKeySource: func(context.Context) (string, error) {
			return "refreshed-key", nil
		},
		BaseURL: server.URL,
		Model:   "test-model",
		Client:  server.Client(),
	}}
	if _, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello"}}}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d", requests)
	}
}

func TestModelEncodesImageInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Messages []struct {
				Content []json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil || len(body.Messages) != 1 || len(body.Messages[0].Content) != 2 {
			t.Fatalf("image request = %#v, err = %v", body, err)
		}
		_, _ = io.WriteString(writer, `{"choices":[{"message":{"content":"done"}}]}`)
	}))
	defer server.Close()
	model := Model{Config: Config{APIKey: "test-key", BaseURL: server.URL, Model: "test-model", Client: server.Client()}}
	if _, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "inspect", Images: []agent.Image{{Name: "screen.png", MediaType: "image/png", Data: []byte("png")}}}}}); err != nil {
		t.Fatalf("complete image input: %v", err)
	}
}

func TestModelRejectsPDFAttachment(t *testing.T) {
	_, err := requestBody(agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Attachments: []agent.Attachment{{Name: "report.pdf", MediaType: "application/pdf", Data: []byte("pdf")}}}}}, "test-model")
	if err == nil || !strings.Contains(err.Error(), "requires the OpenAI Responses") {
		t.Fatalf("PDF attachment error = %v", err)
	}
}
