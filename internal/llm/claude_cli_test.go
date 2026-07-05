package llm

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestClaudeCLITransportRequestShape(t *testing.T) {
	runner := &fakeCLIRunner{stdout: `{"type":"result","result":"{\"done\":true}"}`}
	client := newClaudeCLIClientWithRunner("sonnet", runner)
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
	if runner.inv.Command != "claude" {
		t.Fatalf("command = %q", runner.inv.Command)
	}
	for _, want := range []string{"-p", "--permission-mode", "plan", "--output-format", "json", "--model", "sonnet", "--json-schema", `{"type":"object"}`} {
		if !slices.Contains(runner.inv.Args, want) {
			t.Fatalf("args missing %q: %#v", want, runner.inv.Args)
		}
	}
	if slices.Contains(runner.inv.Args, "--bare") {
		t.Fatalf("args unexpectedly contain --bare: %#v", runner.inv.Args)
	}
	if !strings.Contains(runner.inv.Stdin, "Do not modify files") || !strings.Contains(runner.inv.Stdin, "USER:\nplan") {
		t.Fatalf("stdin = %q", runner.inv.Stdin)
	}
}

func TestParseClaudeOutputReturnsRawJSONObject(t *testing.T) {
	got, err := parseClaudeOutput(cliResult{Stdout: `{"done":true}`})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != `{"done":true}` {
		t.Fatalf("got = %q", got)
	}
}

func TestFactorySupportsClaudeCLITransport(t *testing.T) {
	client, err := newClient(EndpointConfig{Transport: "claude-cli", Model: "sonnet"}, 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, ok := client.(*cliClient); !ok {
		t.Fatalf("client type = %T", client)
	}
}
