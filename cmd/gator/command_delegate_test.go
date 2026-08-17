package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/model"
)

func TestDelegateCodexLoginUsesFirstPartyCLI(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "command.log")
	script := delegatedFixture(t, "printf '%s\\n' \"$@\" > \"$GATOR_DELEGATE_LOG\"\nprintf 'logged in\\n'\n")
	t.Setenv("GATOR_CODEX_COMMAND", script)
	t.Setenv("GATOR_DELEGATE_LOG", logPath)

	var output bytes.Buffer
	if err := delegate([]string{"codex", "login", "--device"}, &output); err != nil {
		t.Fatalf("delegate Codex login: %v", err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read delegated command: %v", err)
	}
	if got := string(logged); got != "login\n--device-auth\n" {
		t.Fatalf("Codex login args = %q", got)
	}
	if !strings.Contains(output.String(), "logged in") {
		t.Fatalf("Codex login output = %q", output.String())
	}
}

func TestDelegateCodexRunUsesIsolatedWorktreeAndVerifies(t *testing.T) {
	repository := delegatedRepository(t)
	changeDirectory(t, repository)
	logPath := filepath.Join(t.TempDir(), "command.log")
	script := delegatedFixture(t, "printf '%s\\n' \"$@\" > \"$GATOR_DELEGATE_LOG\"\nprintf 'delegated agent output\\n'\n")
	t.Setenv("GATOR_CODEX_COMMAND", script)
	t.Setenv("GATOR_DELEGATE_LOG", logPath)

	var output bytes.Buffer
	err := delegate([]string{"codex", "run", "--model", "gpt-test", "--verify", "git status --short", "Add", "a", "feature"}, &output)
	if err != nil {
		t.Fatalf("delegate Codex run: %v", err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read delegated command: %v", err)
	}
	arguments := string(logged)
	if !strings.Contains(arguments, "exec\n--sandbox\nworkspace-write\n--approve-for-me\n--cd\n") {
		t.Fatalf("Codex run did not use delegated workspace-write mode: %q", arguments)
	}
	if !strings.Contains(arguments, "--model\ngpt-test\n--\nAdd a feature\n") {
		t.Fatalf("Codex run args = %q", arguments)
	}
	if !strings.Contains(output.String(), "Delegated run complete. Review worktree:") {
		t.Fatalf("delegate output = %q", output.String())
	}
	if strings.Contains(output.String(), "Worktree: "+repository+"\n") {
		t.Fatalf("delegated run reused active repository: %q", output.String())
	}
}

func TestDelegateClaudeRefusesSubscriptionCredentials(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	var output bytes.Buffer
	err := delegate([]string{"claude", "run", "--verify", "git status --short", "Add", "a", "feature"}, &output)
	if err == nil || !strings.Contains(err.Error(), "Anthropic API key is required") {
		t.Fatalf("Claude delegated run error = %v", err)
	}
	err = delegate([]string{"claude", "login"}, &output)
	if err == nil || !strings.Contains(err.Error(), "does not offer Claude.ai login") {
		t.Fatalf("Claude delegated login error = %v", err)
	}
}

func TestDelegateClaudeUsesGatorAPIKeyWithoutReadingClaudeCredentials(t *testing.T) {
	repository := delegatedRepository(t)
	changeDirectory(t, repository)
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if err := credentials.Put(string(model.Anthropic), auth.Credential{Type: "api_key", Key: "gator-anthropic-key"}); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "claude.log")
	script := delegatedFixture(t, "printf 'key=%s\\nargs=%s\\n' \"$ANTHROPIC_API_KEY\" \"$*\" > \"$GATOR_DELEGATE_LOG\"\n")
	t.Setenv("GATOR_CLAUDE_COMMAND", script)
	t.Setenv("GATOR_DELEGATE_LOG", logPath)

	var output bytes.Buffer
	err = delegate([]string{"claude", "run", "--verify", "git status --short", "Add", "a", "feature"}, &output)
	if err != nil {
		t.Fatalf("delegate Claude run: %v", err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logged), "key=gator-anthropic-key\n") || !strings.Contains(string(logged), "--bare --print --permission-mode acceptEdits") {
		t.Fatalf("Claude delegated environment and args = %q", logged)
	}
	if strings.Contains(output.String(), "gator-anthropic-key") {
		t.Fatalf("delegate output exposed an API key: %q", output.String())
	}
}

func TestDelegateCopilotUsesVendorLoginAndNonInteractiveToolPermission(t *testing.T) {
	repository := delegatedRepository(t)
	changeDirectory(t, repository)
	logPath := filepath.Join(t.TempDir(), "copilot.log")
	script := delegatedFixture(t, "printf '%s\\n' \"$@\" > \"$GATOR_DELEGATE_LOG\"\n")
	t.Setenv("GATOR_COPILOT_COMMAND", script)
	t.Setenv("GATOR_DELEGATE_LOG", logPath)

	var output bytes.Buffer
	if err := delegate([]string{"copilot", "login", "--host", "https://example.ghe.com"}, &output); err != nil {
		t.Fatalf("delegate Copilot login: %v", err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(logged); got != "login\n--host\nhttps://example.ghe.com\n" {
		t.Fatalf("Copilot login args = %q", got)
	}

	if err := delegate([]string{"copilot", "run", "--model", "gpt-test", "--verify", "git status --short", "Add", "a", "feature"}, &output); err != nil {
		t.Fatalf("delegate Copilot run: %v", err)
	}
	logged, err = os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(logged); !strings.Contains(got, "--prompt\nAdd a feature\n--allow-all-tools\n--no-ask-user\n--model\ngpt-test\n") {
		t.Fatalf("Copilot run args = %q", got)
	}
}

func TestDelegateKimiAndOpenCodeUseTheirOwnCredentialStores(t *testing.T) {
	repository := delegatedRepository(t)
	changeDirectory(t, repository)
	kimiLog := filepath.Join(t.TempDir(), "kimi.log")
	kimiScript := delegatedFixture(t, "printf '%s\\n' \"$@\" > \"$GATOR_DELEGATE_LOG\"\n")
	t.Setenv("GATOR_KIMI_COMMAND", kimiScript)
	t.Setenv("GATOR_DELEGATE_LOG", kimiLog)

	var output bytes.Buffer
	if err := delegate([]string{"kimi", "login"}, &output); err != nil {
		t.Fatalf("delegate Kimi login: %v", err)
	}
	logged, err := os.ReadFile(kimiLog)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(logged); got != "login\n" {
		t.Fatalf("Kimi login args = %q", got)
	}
	if err := delegate([]string{"kimi", "run", "--model", "kimi-test", "--verify", "git status --short", "Add", "a", "feature"}, &output); err != nil {
		t.Fatalf("delegate Kimi run: %v", err)
	}
	logged, err = os.ReadFile(kimiLog)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(logged); !strings.Contains(got, "--auto\n--prompt\nAdd a feature\n--output-format\ntext\n--model\nkimi-test\n") {
		t.Fatalf("Kimi run args = %q", got)
	}

	opencodeLog := filepath.Join(t.TempDir(), "opencode.log")
	opencodeScript := delegatedFixture(t, "printf '%s\\n' \"$@\" > \"$GATOR_OPENCODE_LOG\"\n")
	t.Setenv("GATOR_OPENCODE_COMMAND", opencodeScript)
	t.Setenv("GATOR_OPENCODE_LOG", opencodeLog)
	if err := delegate([]string{"opencode", "login", "--provider", "xai"}, &output); err != nil {
		t.Fatalf("delegate OpenCode login: %v", err)
	}
	logged, err = os.ReadFile(opencodeLog)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(logged); got != "providers\nlogin\n--provider\nxai\n" {
		t.Fatalf("OpenCode login args = %q", got)
	}
	if err := delegate([]string{"opencode", "run", "--model", "xai/grok-build", "--verify", "git status --short", "Add", "a", "feature"}, &output); err != nil {
		t.Fatalf("delegate OpenCode run: %v", err)
	}
	logged, err = os.ReadFile(opencodeLog)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(logged); !strings.Contains(got, "run\n--dir\n") || !strings.Contains(got, "--auto\n--model\nxai/grok-build\nAdd a feature\n") {
		t.Fatalf("OpenCode run args = %q", got)
	}
}

func TestDelegateExternalPassesOnlyExplicitArgumentsAndContext(t *testing.T) {
	repository := delegatedRepository(t)
	changeDirectory(t, repository)
	logPath := filepath.Join(t.TempDir(), "external.log")
	script := delegatedFixture(t, "printf 'task=%s\\nworktree=%s\\nargs=%s\\n' \"$GATOR_TASK\" \"$GATOR_WORKTREE\" \"$*\" > \"$GATOR_DELEGATE_LOG\"\n")
	t.Setenv("GATOR_DELEGATE_LOG", logPath)

	var output bytes.Buffer
	err := delegate([]string{"external", "run", "--task", "Inspect safely", "--verify", "git status --short", "--", script, "{task}", "{worktree}"}, &output)
	if err != nil {
		t.Fatalf("delegate external run: %v", err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read external command: %v", err)
	}
	if !strings.Contains(string(logged), "task=Inspect safely\n") || !strings.Contains(string(logged), "args=Inspect safely ") {
		t.Fatalf("external runtime context = %q", logged)
	}
	if strings.Contains(string(logged), "worktree="+repository+"\n") {
		t.Fatalf("external runtime reused active repository: %q", logged)
	}
}

func delegatedFixture(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "delegated-agent")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body), 0o700); err != nil {
		t.Fatalf("write delegated fixture: %v", err)
	}
	return path
}

func delegatedRepository(t *testing.T) string {
	t.Helper()
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	delegatedGit(t, repository, "init", "--quiet")
	delegatedGit(t, repository, "add", "README.md")
	delegatedGit(t, repository, "-c", "user.name=Gator Test", "-c", "user.email=gator@example.invalid", "commit", "--quiet", "-m", "fixture")
	return repository
}

func delegatedGit(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
}

func changeDirectory(t *testing.T, directory string) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
}
