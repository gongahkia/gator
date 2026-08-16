package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
)

func TestRunHelp(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"help"}, &output); err != nil {
		t.Fatalf("run help: %v", err)
	}
	if !strings.Contains(output.String(), "Usage:") {
		t.Fatalf("help output = %q, want usage", output.String())
	}
	if strings.Contains(output.String(), "allow-external-cli") {
		t.Fatalf("help still exposes delegated CLI approval: %q", output.String())
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	var output bytes.Buffer
	err := run([]string{"ship"}, &output)
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run error = %v, want unknown-command error", err)
	}
}

func TestCodexExecutorUsesDirectModelAdapter(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	executor, err := newExecutor("codex", "", "")
	if err != nil {
		t.Fatalf("new codex executor: %v", err)
	}
	if executor.Model == nil {
		t.Fatal("Codex did not resolve to a direct model adapter")
	}
}

func TestRunRejectsUnsupportedLegacyProviderWithoutFallback(t *testing.T) {
	var output bytes.Buffer
	err := runTask([]string{"--provider", "copilot", "--verify", "go test ./...", "Add a focused feature"}, &output)
	if err == nil || !strings.Contains(err.Error(), "no supported direct model API integration") || !strings.Contains(err.Error(), "will not launch") {
		t.Fatalf("copilot run error = %v", err)
	}
}

func TestVerificationFlagsParseArgv(t *testing.T) {
	var flags verificationFlags
	if err := flags.Set("go test ./..."); err != nil {
		t.Fatalf("set verification: %v", err)
	}
	if got := strings.Join(flags[0], " "); got != "go test ./..." {
		t.Fatalf("verification argv = %q", got)
	}
}

func TestSuggestedVerificationCommands(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(directory+string(os.PathSeparator)+"go.mod", []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := suggestedVerificationCommands(directory); len(got) != 1 || got[0] != "go test ./..." {
		t.Fatalf("suggestions = %#v", got)
	}
}

func TestEventPrinterGroupsStreamingText(t *testing.T) {
	var output bytes.Buffer
	printer := eventPrinter{out: &output}
	printer.Print(agent.Event{Kind: agent.EventTextDelta, Step: 1, Text: "hello "})
	printer.Print(agent.Event{Kind: agent.EventTextDelta, Step: 1, Text: "world"})
	printer.Print(agent.Event{Kind: agent.EventToolCalled, Step: 1, ToolCall: &agent.ToolCall{Name: "git_status"}})
	if got, want := output.String(), "[01] agent: hello world\n[01] tool → git_status\n"; got != want {
		t.Fatalf("terminal output = %q, want %q", got, want)
	}
}
