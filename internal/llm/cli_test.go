package llm

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCLIClientPromptSchemaFileAndUsageEstimate(t *testing.T) {
	runner := &fakeCLIRunner{stdout: `{"ok":true}`}
	client := &cliClient{
		command:       "fake",
		model:         "fake-model",
		runner:        runner,
		useSchemaFile: true,
		build: func(in cliBuildInput) cliInvocation {
			if in.Model != "fake-model" {
				t.Fatalf("model = %q", in.Model)
			}
			if in.SchemaPath == "" {
				t.Fatal("missing schema path")
			}
			if _, err := os.Stat(in.SchemaPath); err != nil {
				t.Fatalf("schema path: %v", err)
			}
			return cliInvocation{
				Args:  []string{"--schema", in.SchemaPath},
				Stdin: in.Prompt,
			}
		},
	}
	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages: []ChatMessage{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "hello"},
		},
		JSONSchema: json.RawMessage(`{"type":"object"}`),
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if resp.Content != `{"ok":true}` {
		t.Fatalf("content = %q", resp.Content)
	}
	if resp.Usage.TokenSource != TokenSourceEstimate || resp.Usage.InputTokens == 0 || resp.Usage.OutputTokens == 0 {
		t.Fatalf("usage = %#v", resp.Usage)
	}
	if runner.inv.Command != "fake" {
		t.Fatalf("command = %q", runner.inv.Command)
	}
	if !strings.Contains(runner.inv.Stdin, "SYSTEM:\nsystem") || !strings.Contains(runner.inv.Stdin, "USER:\nhello") {
		t.Fatalf("stdin = %q", runner.inv.Stdin)
	}
	if _, err := os.Stat(runner.inv.Args[1]); !os.IsNotExist(err) {
		t.Fatalf("schema temp file still exists or stat got %v", err)
	}
}

func TestCLIClientSchemaPromptFallback(t *testing.T) {
	runner := &fakeCLIRunner{stdout: `{"ok":true}`}
	client := &cliClient{
		command:               "fake",
		runner:                runner,
		includeSchemaInPrompt: true,
		build: func(in cliBuildInput) cliInvocation {
			return cliInvocation{Stdin: in.Prompt}
		},
	}
	_, err := client.Chat(context.Background(), ChatRequest{
		Messages:   []ChatMessage{{Role: "user", Content: "hello"}},
		JSONSchema: json.RawMessage(`{"type":"object"}`),
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if !strings.Contains(runner.inv.Stdin, "Return only JSON matching this schema") {
		t.Fatalf("stdin = %q", runner.inv.Stdin)
	}
}

func TestParseTextStdoutRejectsEmptyOutput(t *testing.T) {
	_, err := parseTextStdout(cliResult{Stdout: " \n\t"})
	if err == nil {
		t.Fatal("expected error")
	}
}

type fakeCLIRunner struct {
	inv    cliInvocation
	stdout string
	stderr string
	err    error
}

func (r *fakeCLIRunner) Run(_ context.Context, inv cliInvocation) (cliResult, error) {
	r.inv = inv
	return cliResult{Stdout: r.stdout, Stderr: r.stderr}, r.err
}
