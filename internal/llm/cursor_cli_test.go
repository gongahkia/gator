package llm

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestCursorCLITransportRequestShape(t *testing.T) {
	runner := &fakeCLIRunner{stdout: `{"response":"{\"done\":true}"}`}
	client := newCursorCLIClientWithRunner("cursor-model", runner)
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
	if runner.inv.Command != "cursor-agent" {
		t.Fatalf("command = %q", runner.inv.Command)
	}
	for _, want := range []string{"--print", "--output-format", "json", "--mode", "ask", "--model", "cursor-model"} {
		if !slices.Contains(runner.inv.Args, want) {
			t.Fatalf("args missing %q: %#v", want, runner.inv.Args)
		}
	}
	prompt := runner.inv.Args[len(runner.inv.Args)-1]
	if !strings.Contains(prompt, "Do not edit files") || !strings.Contains(prompt, "Return only JSON matching this schema") || !strings.Contains(prompt, "USER:\nplan") {
		t.Fatalf("prompt = %q", prompt)
	}
}

func TestFactorySupportsCursorCLITransport(t *testing.T) {
	client, err := newClient(EndpointConfig{Transport: "cursor-cli", Model: "cursor-model"}, 0)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, ok := client.(*cliClient); !ok {
		t.Fatalf("client type = %T", client)
	}
}
