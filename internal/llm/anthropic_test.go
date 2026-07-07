package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gongahkia/paw/internal/llm/faketest"
	pawlog "github.com/gongahkia/paw/internal/log"
)

func TestAnthropicChatRequestShapeAndToolResponse(t *testing.T) {
	srv := faketest.NewServer()
	defer srv.Close()
	srv.RespondAnthropicTool("shape", map[string]any{"ok": true})

	client := NewAnthropicClient(srv.URL, "test-key", "claude-test")
	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages: []ChatMessage{
			{Role: "system", Content: "system prompt"},
			{Role: "user", Content: "shape"},
		},
		MaxTokens:   123,
		Temperature: 0.25,
		JSONSchema:  json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}}}`),
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Content != `{"ok":true}` {
		t.Fatalf("content = %q", resp.Content)
	}
	if resp.Usage.InputTokens != 17 || resp.Usage.OutputTokens != 9 || resp.Usage.TokenSource != TokenSourceProvider {
		t.Fatalf("usage = %#v", resp.Usage)
	}

	req := srv.LastRequest()
	if req.Path != "/v1/messages" {
		t.Fatalf("path = %q", req.Path)
	}
	if got := req.Header.Get("x-api-key"); got != "test-key" {
		t.Fatalf("x-api-key = %q", got)
	}
	if got := req.Header.Get("anthropic-version"); got != anthropicVersion {
		t.Fatalf("anthropic-version = %q", got)
	}
	var body map[string]any
	decodeBody(t, req.Body, &body)
	if body["model"] != "claude-test" || body["max_tokens"] != float64(123) || body["temperature"] != 0.25 {
		t.Fatalf("body = %#v", body)
	}
	system := body["system"].([]any)
	systemBlock := system[0].(map[string]any)
	if systemBlock["text"] != "system prompt" {
		t.Fatalf("system = %#v", body["system"])
	}
	cacheControl := systemBlock["cache_control"].(map[string]any)
	if cacheControl["type"] != "ephemeral" {
		t.Fatalf("cache_control = %#v", cacheControl)
	}
	messages := body["messages"].([]any)
	if len(messages) != 1 || messages[0].(map[string]any)["role"] != "user" {
		t.Fatalf("messages = %#v", messages)
	}
	tools := body["tools"].([]any)
	tool := tools[0].(map[string]any)
	if tool["name"] != "response" {
		t.Fatalf("tool = %#v", tool)
	}
	schema := tool["input_schema"].(map[string]any)
	if schema["type"] != "object" {
		t.Fatalf("input_schema = %#v", schema)
	}
	choice := body["tool_choice"].(map[string]any)
	if choice["type"] != "tool" || choice["name"] != "response" {
		t.Fatalf("tool_choice = %#v", choice)
	}
}

func TestAnthropicPromptCachingUsage(t *testing.T) {
	srv := faketest.NewServer()
	defer srv.Close()
	srv.Respond("cache", http.StatusOK, `{
		"content":[{"type":"text","text":"ok"}],
		"usage":{
			"input_tokens":17,
			"output_tokens":9,
			"cache_creation_input_tokens":456,
			"cache_read_input_tokens":1234
		}
	}`)
	var logs bytes.Buffer
	logger, err := pawlog.New(pawlog.Config{Writer: &logs})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}

	client := NewAnthropicClient(srv.URL, "", "claude-test")
	resp, err := client.Chat(pawlog.With(context.Background(), logger), ChatRequest{
		Messages: []ChatMessage{
			{Role: "system", Content: "cacheable system"},
			{Role: "user", Content: "cache"},
		},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Usage.CacheCreationInputTokens != 456 || resp.Usage.CacheReadInputTokens != 1234 {
		t.Fatalf("usage = %#v", resp.Usage)
	}
	if !strings.Contains(logs.String(), "anthropic prompt cache hit") || !strings.Contains(logs.String(), "cache_read_input_tokens=1234") {
		t.Fatalf("logs = %q", logs.String())
	}
}

func TestAnthropicTextResponse(t *testing.T) {
	srv := faketest.NewServer()
	defer srv.Close()
	srv.RespondAnthropicText("plain", "ok")

	client := NewAnthropicClient(srv.URL, "", "claude-test")
	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "plain"}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q", resp.Content)
	}
}

func TestAnthropicJSONSchemaFallbackOn4xx(t *testing.T) {
	srv := faketest.NewServer()
	defer srv.Close()
	srv.Respond("fallback", http.StatusBadRequest, `{"error":"tools unsupported"}`)
	srv.RespondAnthropicText("fallback", `{"ok":true}`)

	client := NewAnthropicClient(srv.URL, "", "claude-test")
	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages:   []ChatMessage{{Role: "user", Content: "fallback"}},
		JSONSchema: json.RawMessage(`{"type":"object"}`),
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Content != `{"ok":true}` {
		t.Fatalf("content = %q", resp.Content)
	}
	requests := srv.Requests()
	if len(requests) != 2 {
		t.Fatalf("requests = %d", len(requests))
	}
	var first map[string]any
	decodeBody(t, requests[0].Body, &first)
	if _, ok := first["tools"]; !ok {
		t.Fatalf("first request missing tools: %#v", first)
	}
	var retryBody map[string]any
	decodeBody(t, requests[1].Body, &retryBody)
	if _, ok := retryBody["tools"]; ok {
		t.Fatalf("fallback request has tools: %#v", retryBody)
	}
	retrySystem := retryBody["system"].([]any)[0].(map[string]any)
	if !strings.Contains(retrySystem["text"].(string), "Return only JSON matching this schema") {
		t.Fatalf("fallback system = %#v", retryBody["system"])
	}
}

func TestAnthropicFactory(t *testing.T) {
	srv := faketest.NewServer()
	defer srv.Close()
	srv.RespondAnthropicText("factory", "ok")

	client, err := NewBrainClient(FactoryConfig{
		Brain: EndpointConfig{
			Transport: "anthropic",
			BaseURL:   srv.URL,
			APIKey:    "test-key",
			Model:     "claude-test",
		},
	})
	if err != nil {
		t.Fatalf("brain client: %v", err)
	}
	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "factory"}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q", resp.Content)
	}
	if srv.LastRequest().Path != "/v1/messages" {
		t.Fatalf("path = %q", srv.LastRequest().Path)
	}
}

func TestAnthropicBrainRequiresKey(t *testing.T) {
	client, err := NewBrainClient(FactoryConfig{
		Brain: EndpointConfig{Transport: "anthropic"},
	})
	if err == nil || !strings.Contains(err.Error(), "PAW_BRAIN_API_KEY") {
		t.Fatalf("expected missing key factory error, got client=%T err=%v", client, err)
	}
}
