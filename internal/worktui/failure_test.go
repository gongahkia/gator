package worktui

import (
	"strings"
	"testing"
)

func TestLocalConnectionFailureExplainsRuntimeRecovery(t *testing.T) {
	model := New(Config{ModelStatus: func() (ModelStatus, error) {
		return ModelStatus{Provider: "gator-local", Model: "qwen2.5-coder:0.5b", Access: "local model configured"}, nil
	}})
	model.home = false
	updated, command := model.Update(runDone(RunResult{Error: `model turn 1: request custom provider gator-local response: Post "http://127.0.0.1:11434/v1/chat/completions": dial tcp 127.0.0.1:11434: connect: connection refused`}))
	if command != nil {
		t.Fatal("run failure unexpectedly scheduled a command")
	}
	for _, expected := range []string{
		"Local Ollama connection refused",
		"gator-local/qwen2.5-coder:0.5b",
		"Recovery: open /model, choose Local, and press s",
		"ollama serve",
		"ollama ps",
		"Detail:",
	} {
		if !strings.Contains(updated.(Model).messages[len(updated.(Model).messages)-1].text, expected) {
			t.Fatalf("failure explanation omitted %q:\n%s", expected, updated.(Model).messages[len(updated.(Model).messages)-1].text)
		}
	}
}

func TestProviderAuthenticationFailureSuggestsModelSetup(t *testing.T) {
	message := explainRunFailure("model turn 1: upstream returned status 401", ModelStatus{Provider: "openai", Model: "gpt-5.6"})
	for _, expected := range []string{"Model authentication failed", "openai/gpt-5.6", "open /model", "Detail:"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("failure explanation omitted %q:\n%s", expected, message)
		}
	}
}
