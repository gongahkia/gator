package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gongahkia/paw/internal/llm/faketest"
)

func TestOllamaChatRequestShapeAndResponse(t *testing.T) {
	srv := faketest.NewServer()
	defer srv.Close()
	srv.RespondOllama("shape", `{"ok":true}`)

	client := NewOllamaClient(srv.URL, "qwen-test")
	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages:    []ChatMessage{{Role: "user", Content: "shape"}},
		Temperature: 0.25,
		JSONSchema:  json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}}}`),
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Content != `{"ok":true}` {
		t.Fatalf("content = %q", resp.Content)
	}
	if resp.Usage.InputTokens != 13 || resp.Usage.OutputTokens != 5 || resp.Usage.TokenSource != TokenSourceProvider {
		t.Fatalf("usage = %#v", resp.Usage)
	}

	req := srv.LastRequest()
	if req.Path != "/api/chat" {
		t.Fatalf("path = %q", req.Path)
	}
	var body map[string]any
	decodeBody(t, req.Body, &body)
	if body["model"] != "qwen-test" || body["stream"] != false {
		t.Fatalf("body = %#v", body)
	}
	format := body["format"].(map[string]any)
	if format["type"] != "object" {
		t.Fatalf("format = %#v", format)
	}
	options := body["options"].(map[string]any)
	if options["temperature"] != 0.25 {
		t.Fatalf("options = %#v", options)
	}
}

func TestOllamaRetryOn500(t *testing.T) {
	withFastRetries(t)
	srv := faketest.NewServer()
	defer srv.Close()
	srv.Respond("retry", http.StatusInternalServerError, `{"error":"boom"}`)
	srv.RespondOllama("retry", "ok")

	client := NewRetryClient(NewOllamaClient(srv.URL, "qwen-test"), 0)
	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "retry"}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q", resp.Content)
	}
	if got := len(srv.Requests()); got != 2 {
		t.Fatalf("requests = %d", got)
	}
}

func TestOllamaNoRetryOn400(t *testing.T) {
	withFastRetries(t)
	srv := faketest.NewServer()
	defer srv.Close()
	srv.Respond("bad", http.StatusBadRequest, `{"error":"bad request"}`)

	client := NewRetryClient(NewOllamaClient(srv.URL, "qwen-test"), 0)
	_, err := client.Chat(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "bad"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := len(srv.Requests()); got != 1 {
		t.Fatalf("requests = %d", got)
	}
}
