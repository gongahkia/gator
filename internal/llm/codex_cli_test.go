package llm

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestCodexCLITransportRequestShape(t *testing.T) {
	runner := &fakeCLIRunner{stdout: `{"done":true}`}
	client := newCodexCLIClientWithRunner("gpt-test", runner)
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
	if runner.inv.Command != "codex" {
		t.Fatalf("command = %q", runner.inv.Command)
	}
	for _, want := range []string{"exec", "--ephemeral", "--sandbox", "read-only", "--model", "gpt-test", "--output-schema", "-"} {
		if !slices.Contains(runner.inv.Args, want) {
			t.Fatalf("args missing %q: %#v", want, runner.inv.Args)
		}
	}
	if !strings.Contains(runner.inv.Stdin, "Do not modify files") || !strings.Contains(runner.inv.Stdin, "USER:\nplan") {
		t.Fatalf("stdin = %q", runner.inv.Stdin)
	}
}

func TestFactorySupportsCodexCLITransport(t *testing.T) {
	client, err := newClient(EndpointConfig{Transport: "codex-cli", Model: "gpt-test"}, 0, TLSConfig{})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, ok := client.(*cliClient); !ok {
		t.Fatalf("client type = %T", client)
	}
}
