package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/auth"
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

func TestRunVersion(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"version"}, &output); err != nil {
		t.Fatalf("run version: %v", err)
	}
	if !strings.Contains(output.String(), "gator ") {
		t.Fatalf("version output = %q", output.String())
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	var output bytes.Buffer
	err := run([]string{"ship"}, &output)
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run error = %v, want unknown-command error", err)
	}
}

func TestLoginAndLogoutStoreOnlyGatorCredential(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	t.Setenv("OPENAI_API_KEY", "test-key")
	var output bytes.Buffer
	if err := run([]string{"login", "openai"}, &output); err != nil {
		t.Fatalf("login: %v", err)
	}
	if !strings.Contains(output.String(), "Stored a Gator credential for openai") || strings.Contains(output.String(), "test-key") {
		t.Fatalf("login output = %q", output.String())
	}
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	credential, found, err := credentials.Read("openai")
	if err != nil || !found || !credential.IsAPIKey() || credential.Key != "test-key" {
		t.Fatalf("stored credential = %#v, found=%v, error=%v", credential, found, err)
	}
	output.Reset()
	if err := run([]string{"logout", "openai"}, &output); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, found, err := credentials.Read("openai"); err != nil || found {
		t.Fatalf("credential after logout found=%v, error=%v", found, err)
	}
}

func TestCodexExecutorUsesDirectModelAdapter(t *testing.T) {
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	if err := credentials.Put("codex", auth.Credential{Type: "oauth", Access: "access-token", Expires: time.Now().Add(time.Hour).UnixMilli(), Extra: map[string]string{"chatgpt_account_id": "account_123"}}); err != nil {
		t.Fatalf("store Codex OAuth credential: %v", err)
	}
	executor, err := newExecutor("codex", "", "")
	if err != nil {
		t.Fatalf("new codex executor: %v", err)
	}
	if executor.Model == nil {
		t.Fatal("Codex did not resolve to a direct model adapter")
	}
}

func TestExecutorReadsWebSearchKeyFromEnvironmentOnly(t *testing.T) {
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("BRAVE_SEARCH_API_KEY", "do-not-print-this-search-token")
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatalf("new credentials: %v", err)
	}
	if err := credentials.Put("codex", auth.Credential{Type: "oauth", Access: "access-token", Expires: time.Now().Add(time.Hour).UnixMilli(), Extra: map[string]string{"chatgpt_account_id": "account_123"}}); err != nil {
		t.Fatalf("store Codex credential: %v", err)
	}
	executor, err := newExecutor("codex", "", "")
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	if executor.HTTP.BraveSearchAPIKey != "do-not-print-this-search-token" {
		t.Fatalf("web search key did not reach the in-memory executor options")
	}
}

func TestRunRejectsUnsupportedCursorProviderWithoutFallback(t *testing.T) {
	var output bytes.Buffer
	err := runTask([]string{"--provider", "cursor", "--verify", "go test ./...", "Add a focused feature"}, &output)
	if err == nil || !strings.Contains(err.Error(), "no supported direct model API integration") || !strings.Contains(err.Error(), "will not launch") {
		t.Fatalf("cursor run error = %v", err)
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
