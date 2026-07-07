package llm

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestAiderCLITransportRequestShape(t *testing.T) {
	runner := &fakeCLIRunner{stdout: `{"result":"{\"done\":true}"}`}
	client := newAiderCLIClientWithRunner("aider-model", runner)
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
	if runner.inv.Command != "aider" {
		t.Fatalf("command = %q", runner.inv.Command)
	}
	for _, want := range []string{
		"--message",
		"--dry-run",
		"--no-git",
		"--no-gitignore",
		"--no-auto-commits",
		"--no-dirty-commits",
		"--no-auto-lint",
		"--no-auto-test",
		"--no-suggest-shell-commands",
		"--model",
		"aider-model",
	} {
		if !slices.Contains(runner.inv.Args, want) {
			t.Fatalf("args missing %q: %#v", want, runner.inv.Args)
		}
	}
	messageAt := slices.Index(runner.inv.Args, "--message")
	if messageAt < 0 || messageAt+1 >= len(runner.inv.Args) {
		t.Fatalf("args missing message value: %#v", runner.inv.Args)
	}
	message := runner.inv.Args[messageAt+1]
	if !strings.Contains(message, "Do not inspect files") || !strings.Contains(message, "Do not edit files") || !strings.Contains(message, "Return only JSON matching this schema") || !strings.Contains(message, "USER:\nplan") {
		t.Fatalf("message = %q", message)
	}
}

func TestFactorySupportsAiderCLITransport(t *testing.T) {
	client, err := newClient(EndpointConfig{Transport: "aider-cli", Model: "aider-model"}, 0, TLSConfig{})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, ok := client.(*cliClient); !ok {
		t.Fatalf("client type = %T", client)
	}
}
