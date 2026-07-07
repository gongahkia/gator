package llm

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestQwenCLITransportRequestShape(t *testing.T) {
	runner := &fakeCLIRunner{stdout: `{"response":"{\"done\":true}"}`}
	client := newQwenCLIClientWithRunner("qwen3-coder-plus", runner)
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
	if runner.inv.Command != "qwen" {
		t.Fatalf("command = %q", runner.inv.Command)
	}
	for _, want := range []string{
		"--prompt",
		"--approval-mode",
		"plan",
		"--output-format",
		"json",
		"--model",
		"qwen3-coder-plus",
	} {
		if !slices.Contains(runner.inv.Args, want) {
			t.Fatalf("args missing %q: %#v", want, runner.inv.Args)
		}
	}
	promptAt := slices.Index(runner.inv.Args, "--prompt")
	if promptAt < 0 || promptAt+1 >= len(runner.inv.Args) {
		t.Fatalf("args missing prompt value: %#v", runner.inv.Args)
	}
	prompt := runner.inv.Args[promptAt+1]
	if !strings.Contains(prompt, "Do not inspect files") || !strings.Contains(prompt, "Do not edit files") || !strings.Contains(prompt, "Return only JSON matching this schema") || !strings.Contains(prompt, "USER:\nplan") {
		t.Fatalf("prompt = %q", prompt)
	}
}

func TestFactorySupportsQwenCLITransport(t *testing.T) {
	client, err := newClient(EndpointConfig{Transport: "qwen-cli", Model: "qwen3-coder-plus"}, 0, TLSConfig{})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, ok := client.(*cliClient); !ok {
		t.Fatalf("client type = %T", client)
	}
}
