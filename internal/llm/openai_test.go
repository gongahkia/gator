package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gongahkia/paw/internal/llm/faketest"
)

func TestOpenAIChatRequestShapeAndResponse(t *testing.T) {
	srv := faketest.NewServer()
	defer srv.Close()
	srv.RespondOpenAI("shape", `{"ok":true}`)

	client := NewOpenAIClient(srv.URL, "test-key", "glm-test")
	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "shape"}},
		JSONSchema: json.RawMessage(`{
			"type":"object",
			"additionalProperties":false,
			"properties":{"ok":{"type":"boolean"}}
		}`),
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Content != `{"ok":true}` {
		t.Fatalf("content = %q", resp.Content)
	}
	if resp.Usage.InputTokens != 11 || resp.Usage.OutputTokens != 7 || resp.Usage.TokenSource != TokenSourceProvider {
		t.Fatalf("usage = %#v", resp.Usage)
	}

	req := srv.LastRequest()
	if req.Path != "/chat/completions" {
		t.Fatalf("path = %q", req.Path)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer test-key" {
		t.Fatalf("authorization = %q", got)
	}
	var body map[string]any
	decodeBody(t, req.Body, &body)
	if body["model"] != "glm-test" || body["stream"] != false {
		t.Fatalf("body = %#v", body)
	}
	format := body["response_format"].(map[string]any)
	if format["type"] != "json_schema" {
		t.Fatalf("response_format = %#v", format)
	}
	schema := format["json_schema"].(map[string]any)
	if schema["strict"] != true {
		t.Fatalf("json_schema = %#v", schema)
	}
}

func TestOpenAIJSONSchemaFallbackOn4xx(t *testing.T) {
	srv := faketest.NewServer()
	defer srv.Close()
	srv.Respond("fallback", http.StatusBadRequest, `{"error":"schema rejected"}`)
	srv.RespondOpenAI("fallback", `{"ok":true}`)

	client := NewOpenAIClient(srv.URL, "", "glm-test")
	_, err := client.Chat(context.Background(), ChatRequest{
		Messages:   []ChatMessage{{Role: "user", Content: "fallback"}},
		JSONSchema: json.RawMessage(`{"type":"object"}`),
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	requests := srv.Requests()
	if len(requests) != 2 {
		t.Fatalf("requests = %d", len(requests))
	}
	var retryBody map[string]any
	decodeBody(t, requests[1].Body, &retryBody)
	format := retryBody["response_format"].(map[string]any)
	if format["type"] != "json_object" {
		t.Fatalf("fallback response_format = %#v", format)
	}
}

func TestOpenAIRetryOn500(t *testing.T) {
	withFastRetries(t)
	srv := faketest.NewServer()
	defer srv.Close()
	srv.Respond("retry", http.StatusInternalServerError, `{"error":"boom"}`)
	srv.RespondOpenAI("retry", "ok")

	client := NewRetryClient(NewOpenAIClient(srv.URL, "", "glm-test"), 0)
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

func TestOpenAINoRetryOn400(t *testing.T) {
	withFastRetries(t)
	srv := faketest.NewServer()
	defer srv.Close()
	srv.Respond("bad", http.StatusBadRequest, `{"error":"bad request"}`)

	client := NewRetryClient(NewOpenAIClient(srv.URL, "", "glm-test"), 0)
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

func withFastRetries(t *testing.T) {
	t.Helper()
	old := retryBackoffs
	retryBackoffs = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { retryBackoffs = old })
}

func decodeBody(t *testing.T, raw string, out any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
}
