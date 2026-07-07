package llm

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestGooseCLITransportRequestShape(t *testing.T) {
	runner := &fakeCLIRunner{stdout: `{"response":"{\"done\":true}"}`}
	client := newGooseCLIClientWithRunner("ollama", "qwen3:8b", runner)
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
	if runner.inv.Command != "goose" {
		t.Fatalf("command = %q", runner.inv.Command)
	}
	for _, want := range []string{
		"run",
		"--no-session",
		"--quiet",
		"--output-format",
		"json",
		"--no-profile",
		"--max-turns",
		"1",
		"--provider",
		"ollama",
		"--model",
		"qwen3:8b",
		"--text",
	} {
		if !slices.Contains(runner.inv.Args, want) {
			t.Fatalf("args missing %q: %#v", want, runner.inv.Args)
		}
	}
	textAt := slices.Index(runner.inv.Args, "--text")
	if textAt < 0 || textAt+1 >= len(runner.inv.Args) {
		t.Fatalf("args missing text value: %#v", runner.inv.Args)
	}
	text := runner.inv.Args[textAt+1]
	if !strings.Contains(text, "Do not inspect files") || !strings.Contains(text, "Do not edit files") || !strings.Contains(text, "Return only JSON matching this schema") || !strings.Contains(text, "USER:\nplan") {
		t.Fatalf("text = %q", text)
	}
}

func TestFactorySupportsGooseCLITransport(t *testing.T) {
	client, err := newClient(EndpointConfig{Transport: "goose-cli", Provider: "ollama", Model: "qwen3:8b"}, 0, TLSConfig{})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, ok := client.(*cliClient); !ok {
		t.Fatalf("client type = %T", client)
	}
}
