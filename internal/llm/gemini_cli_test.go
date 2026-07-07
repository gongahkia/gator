package llm

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestGeminiCLITransportRequestShape(t *testing.T) {
	runner := &fakeCLIRunner{stdout: `{"response":"{\"done\":true}"}`}
	client := newGeminiCLIClientWithRunner("gemini-test", runner)
	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages:   []ChatMessage{{Role: "user", Content: "plan"}},
		JSONSchema: json.RawMessage(`{"type":"object"}`),
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Content != `{"done":true}` {
		t.Fatalf("content = %q", resp.Content)
	}
	if runner.inv.Command != "gemini" {
		t.Fatalf("command = %q", runner.inv.Command)
	}
	for _, want := range []string{"--prompt", "--approval-mode", "plan", "--output-format", "json", "--skip-trust", "--model", "gemini-test"} {
		if !slices.Contains(runner.inv.Args, want) {
			t.Fatalf("args missing %q: %#v", want, runner.inv.Args)
		}
	}
	if !strings.Contains(runner.inv.Stdin, "Return only JSON matching this schema") || !strings.Contains(runner.inv.Stdin, "USER:\nplan") {
		t.Fatalf("stdin = %q", runner.inv.Stdin)
	}
}

func TestParseGeminiOutputReturnsRawJSONObject(t *testing.T) {
	got, err := parseGeminiOutput(cliResult{Stdout: `{"done":true}`})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != `{"done":true}` {
		t.Fatalf("got = %q", got)
	}
}

func TestFactorySupportsGeminiCLITransport(t *testing.T) {
	client, err := newClient(EndpointConfig{Transport: "gemini-cli", Model: "gemini-test"}, 0, TLSConfig{})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, ok := client.(*cliClient); !ok {
		t.Fatalf("client type = %T", client)
	}
}
