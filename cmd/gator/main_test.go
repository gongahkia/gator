package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/auth"
)

func TestRunHelp(t *testing.T) {
	for _, command := range []string{"--help", "-h"} {
		var output bytes.Buffer
		if err := run([]string{command}, &output); err != nil {
			t.Fatalf("run %s: %v", command, err)
		}
		if !strings.Contains(output.String(), "Start Work:") {
			t.Fatalf("help output = %q, want a Work-first entry point", output.String())
		}
		for _, value := range []string{"gator \"research X and produce a report\"", "Work", "History", "Learnings", "Jobs", "Settings", "gator work review", "gator work -- TASK"} {
			if !strings.Contains(output.String(), value) {
				t.Fatalf("help output is missing %q", value)
			}
		}
		for _, hidden := range []string{"gator agent acp", "--kind json|webhook", "snapshot-max-files", "--actions forbid|draft|approve"} {
			if strings.Contains(output.String(), hidden) {
				t.Fatalf("normal help exposes advanced plumbing %q: %q", hidden, output.String())
			}
		}
	}
}

func TestRunVersion(t *testing.T) {
	for _, command := range []string{"--version", "-v"} {
		var output bytes.Buffer
		if err := run([]string{command}, &output); err != nil {
			t.Fatalf("run %s: %v", command, err)
		}
		if !strings.Contains(output.String(), "gator ") {
			t.Fatalf("version output = %q", output.String())
		}
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	var output bytes.Buffer
	err := run([]string{"ship"}, &output)
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run error = %v, want unknown-command error", err)
	}
}

func TestQuotedWorkRequestUsesTheWorkEntryPoint(t *testing.T) {
	t.Setenv("GATOR_PROVIDER", "not-a-provider")
	var output bytes.Buffer
	err := run([]string{"research release notes and produce a report"}, &output)
	if err == nil || !strings.Contains(err.Error(), "unknown provider") || strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("quoted request error = %v", err)
	}
}

func TestRunRejectsRemovedRootAliases(t *testing.T) {
	for _, command := range []string{"tui", "help", "version", "connect", "login", "logout", "code", "run", "fork", "clone", "local", "learning", "rpc", "serve", "child", "delegate"} {
		var output bytes.Buffer
		err := run([]string{command}, &output)
		if err == nil || !strings.Contains(err.Error(), "unknown command") || !strings.Contains(err.Error(), "gator --help") {
			t.Fatalf("%s error = %v", command, err)
		}
	}
}

func TestLegacyProtocolAndDelegationCommandsAreRemoved(t *testing.T) {
	for _, arguments := range [][]string{{"agent", "rpc"}, {"agent", "serve"}, {"agent", "child"}, {"agent", "delegate"}, {"--mode", "rpc"}} {
		var output bytes.Buffer
		err := run(arguments, &output)
		if err == nil {
			t.Fatalf("%s unexpectedly succeeded", strings.Join(arguments, " "))
		}
		for _, removed := range []string{"gator agent rpc", "gator agent serve", "gator agent child", "gator agent delegate"} {
			if strings.Contains(err.Error(), removed) || strings.Contains(output.String(), removed) {
				t.Fatalf("%s still exposes removed surface %q: error=%q output=%q", strings.Join(arguments, " "), removed, err, output.String())
			}
		}
	}
}

func TestMovedRootCommandsExplainTheirCanonicalFamily(t *testing.T) {
	for command, canonical := range map[string]string{
		"acp":  "gator agent acp",
		"hook": "gator config hook", "connector": "gator provider connector", "inbox": "gator job inbox", "snapshot": "gator work snapshot",
		"inspect": "gator work inspect", "resume": "gator work resume", "eval": "gator work eval",
		"review": "gator work review", "export": "gator work export", "apply": "gator work apply",
	} {
		var output bytes.Buffer
		err := run([]string{command}, &output)
		if err == nil || !strings.Contains(err.Error(), "has moved") || !strings.Contains(err.Error(), canonical) {
			t.Fatalf("%s error = %v, want migration to %s", command, err, canonical)
		}
	}
}

func TestShortCLIFormsRouteToTheirCommandFamilies(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	var output bytes.Buffer
	if err := run([]string{"-c"}, &output); err != nil || !strings.Contains(output.String(), "Settings\n") {
		t.Fatalf("-c output = %q, err = %v", output.String(), err)
	}
	output.Reset()
	if err := run([]string{"-p", "list"}, &output); err != nil || !strings.Contains(output.String(), "No custom providers configured") {
		t.Fatalf("-p list output = %q, err = %v", output.String(), err)
	}
	if err := run([]string{"-u", "unexpected"}, io.Discard); err == nil || !strings.Contains(err.Error(), "usage: gator update") {
		t.Fatalf("-u error = %v", err)
	}
	if err := run([]string{"-d", "--provider", "not-a-provider"}, io.Discard); err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("-d error = %v", err)
	}
	output.Reset()
	if err := run([]string{"-a", "--help"}, &output); err != nil || !strings.Contains(output.String(), "gator agent acp") {
		t.Fatalf("-a output = %q, err = %v", output.String(), err)
	}
	if err := run([]string{"-e"}, io.Discard); err == nil || !strings.Contains(err.Error(), "extension") {
		t.Fatalf("-e error = %v", err)
	}
	if err := run([]string{"-l"}, io.Discard); err == nil || !strings.Contains(err.Error(), "gator lsp") {
		t.Fatalf("-l error = %v", err)
	}
	if err := run([]string{"-m"}, io.Discard); err == nil || !strings.Contains(err.Error(), "gator mcp") {
		t.Fatalf("-m error = %v", err)
	}
	if err := run([]string{"-j", "inbox", "unexpected"}, io.Discard); err == nil || !strings.Contains(err.Error(), "gator job inbox") {
		t.Fatalf("-j inbox error = %v", err)
	}
	if err := run([]string{"-b"}, io.Discard); err == nil || !strings.Contains(err.Error(), "gator browser") {
		t.Fatalf("-b error = %v", err)
	}
	output.Reset()
	if err := run([]string{"-t", "list"}, &output); err != nil || !strings.Contains(output.String(), "contrast") {
		t.Fatalf("-t output = %q, err = %v", output.String(), err)
	}
	output.Reset()
	if err := run([]string{"-w", "code", "--help"}, &output); err != nil || !strings.Contains(output.String(), "gator work code") {
		t.Fatalf("-w code output = %q, err = %v", output.String(), err)
	}
}

func TestNestedCommandFamiliesRouteToTheirHandlers(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	for _, test := range []struct {
		arguments []string
		contains  string
	}{
		{[]string{"agent", "acp", "unexpected"}, "usage: gator agent acp"},
		{[]string{"-a", "acp", "unexpected"}, "usage: gator agent acp"},
		{[]string{"config", "hook"}, "gator config hook status"},
		{[]string{"-c", "hook"}, "gator config hook status"},
		{[]string{"provider", "connector", "status"}, "connector ID"},
		{[]string{"-p", "connector", "status"}, "connector ID"},
		{[]string{"job", "inbox", "unexpected"}, "gator job inbox"},
		{[]string{"work", "eval"}, "usage: gator work eval"},
		{[]string{"-w", "eval"}, "usage: gator work eval"},
		{[]string{"work", "snapshot", "show"}, "gator work snapshot show"},
		{[]string{"work", "review"}, "usage: gator work review"},
	} {
		if err := run(test.arguments, io.Discard); err == nil || !strings.Contains(err.Error(), test.contains) {
			t.Fatalf("%s error = %v, want %q", strings.Join(test.arguments, " "), err, test.contains)
		}
	}
}

func TestCodeRouteThroughTheMainOrchestrationEntryPoint(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "")
	for _, command := range []string{"code"} {
		var output bytes.Buffer
		err := run([]string{"work", command, "task without verification"}, &output)
		if err == nil || strings.Contains(err.Error(), "--verify") || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
			t.Fatalf("%s entry point error = %v", command, err)
		}
	}
}

func TestCodeRouteExplainsTheManagerOwnedWorkflow(t *testing.T) {
	for _, command := range []string{"code"} {
		var output bytes.Buffer
		if err := run([]string{"work", command, "--help"}, &output); err != nil {
			t.Fatalf("%s help: %v", command, err)
		}
		for _, expected := range []string{"main Gator orchestration path", "internal Code specialist", "--code-capability", "strict sandbox"} {
			if !strings.Contains(output.String(), expected) {
				t.Fatalf("%s help is missing %q: %s", command, expected, output.String())
			}
		}
	}
}

func TestStandaloneCodeConversationCommandsAreRetired(t *testing.T) {
	for _, command := range []string{"resume", "fork", "clone"} {
		var output bytes.Buffer
		err := run([]string{"work", "code", command, "legacy-id"}, &output)
		if err == nil || !strings.Contains(err.Error(), "standalone Code sessions are retired") {
			t.Fatalf("work code %s error = %v", command, err)
		}
	}
}

func TestSupportedPlatformAllowsOnlyLinuxAndMacOS(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		if !supportedPlatform(goos) {
			t.Fatalf("supported platform rejected: %s", goos)
		}
	}
	for _, goos := range []string{"freebsd", "plan9"} {
		if supportedPlatform(goos) {
			t.Fatalf("unsupported platform accepted: %s", goos)
		}
	}
}

func TestLoginAndLogoutStoreOnlyGatorCredential(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	t.Setenv("OPENAI_API_KEY", "test-key")
	var output bytes.Buffer
	if err := run([]string{"-p", "login", "openai"}, &output); err != nil {
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
	if err := run([]string{"provider", "logout", "openai"}, &output); err != nil {
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
	executor, err := newCodeExecutor("codex", "", "")
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
	executor, err := newCodeExecutor("codex", "", "")
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	if executor.HTTP.BraveSearchAPIKey != "do-not-print-this-search-token" {
		t.Fatalf("web search key did not reach the in-memory executor options")
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
