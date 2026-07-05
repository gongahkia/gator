package llm

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestOpenCodeCLITransportRequestShape(t *testing.T) {
	runner := &fakeCLIRunner{stdout: `{"type":"message","message":{"content":"{\"done\":true}"}}`}
	client := newOpenCodeCLIClientWithRunner("opencode/big-pickle", runner)
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
	if runner.inv.Command != "opencode" {
		t.Fatalf("command = %q", runner.inv.Command)
	}
	for _, want := range []string{"run", "--format", "json", "--model", "opencode/big-pickle"} {
		if !slices.Contains(runner.inv.Args, want) {
			t.Fatalf("args missing %q: %#v", want, runner.inv.Args)
		}
	}
	prompt := runner.inv.Args[len(runner.inv.Args)-1]
	if !strings.Contains(prompt, "Do not modify files") || !strings.Contains(prompt, "Return only JSON matching this schema") || !strings.Contains(prompt, "USER:\nplan") {
		t.Fatalf("prompt = %q", prompt)
	}
}

func TestParseOpenCodeOutputUsesLastJSONEventText(t *testing.T) {
	got, err := parseOpenCodeOutput(cliResult{Stdout: strings.Join([]string{
		`{"type":"status","text":"starting"}`,
		`{"type":"message","message":{"content":"{\"done\":true}"}}`,
	}, "\n")})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != `{"done":true}` {
		t.Fatalf("got = %q", got)
	}
}

func TestFactorySupportsOpenCodeCLITransport(t *testing.T) {
	client, err := newClient(EndpointConfig{Transport: "opencode-cli", Model: "opencode/big-pickle"}, 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, ok := client.(*cliClient); !ok {
		t.Fatalf("client type = %T", client)
	}
}
