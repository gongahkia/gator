package gemini

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

func TestGenerateContentPreservesCandidateDataForToolReplay(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Header.Get("x-goog-api-key") != "test-key" || !strings.HasSuffix(request.URL.Path, "/models/gemini-test:generateContent") {
			t.Fatalf("request = %#v", request)
		}
		var body struct {
			Contents []struct {
				Role  string            `json:"role"`
				Parts []json.RawMessage `json:"parts"`
			} `json:"contents"`
			Tools []json.RawMessage `json:"tools"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if requests == 1 {
			if len(body.Contents) != 1 || len(body.Tools) != 1 {
				t.Fatalf("first request = %#v", body)
			}
			_, _ = io.WriteString(writer, `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call_1","name":"read_file","args":{"path":"README.md"}},"thoughtSignature":"opaque-signature"}]}}]}`)
			return
		}
		if len(body.Contents) != 3 || body.Contents[1].Role != "model" || body.Contents[2].Role != "tool" {
			t.Fatalf("replayed contents = %#v", body.Contents)
		}
		if !strings.Contains(string(body.Contents[1].Parts[0]), "opaque-signature") || !strings.Contains(string(body.Contents[2].Parts[0]), "functionResponse") {
			t.Fatalf("provider data was not faithfully replayed: %#v", body.Contents)
		}
		_, _ = io.WriteString(writer, `{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]}}]}`)
	}))
	defer server.Close()

	model := GenerateContent{APIKey: "test-key", Model: "gemini-test", BaseURL: server.URL, Client: server.Client()}
	first, err := model.Complete(context.Background(), agent.TurnRequest{
		System:   "be precise",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "read the readme"}},
		Tools:    []agent.ToolDefinition{{Name: "read_file", Parameters: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatalf("first complete: %v", err)
	}
	if len(first.ToolCalls) != 1 || first.ToolCalls[0].ID != "call_1" || !strings.Contains(string(first.ProviderData), "opaque-signature") {
		t.Fatalf("first turn = %#v", first)
	}
	second, err := model.Complete(context.Background(), agent.TurnRequest{
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "read the readme"},
			{Role: agent.RoleAgent, ToolCalls: first.ToolCalls, ProviderData: first.ProviderData},
			{Role: agent.RoleTool, ToolCallID: "call_1", ToolName: "read_file", Content: `{"ok":true}`},
		},
	})
	if err != nil {
		t.Fatalf("second complete: %v", err)
	}
	if second.Text != "done" {
		t.Fatalf("second turn = %#v", second)
	}
}
