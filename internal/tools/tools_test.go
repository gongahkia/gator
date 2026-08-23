package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/terminal"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestReadFileReturnsBoundedRange(t *testing.T) {
	root := testWorkspace(t)
	writeTestFile(t, root.Path(), "pkg/example.txt", "one\ntwo\nthree\nfour\n")
	tool := ReadFile{Root: root, MaxLines: 2}

	result := executeTool(t, tool, `{"path":"pkg/example.txt","start_line":2}`)
	if !strings.Contains(result, `"start_line":2`) || !strings.Contains(result, `"content":"two\nthree"`) || !strings.Contains(result, `"truncated":true`) {
		t.Fatalf("read result = %s", result)
	}
}

func TestReadFileRejectsOutsideSymlink(t *testing.T) {
	root := testWorkspace(t)
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(external, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root.Path(), "outside")); err != nil {
		t.Fatal(err)
	}
	_, err := ReadFile{Root: root}.Execute(context.Background(), json.RawMessage(`{"path":"outside/secret.txt"}`))
	if err == nil || !strings.Contains(err.Error(), "escapes the workspace") {
		t.Fatalf("read outside symlink error = %v", err)
	}
}

func TestListFilesSkipsGitMetadata(t *testing.T) {
	root := testWorkspace(t)
	writeTestFile(t, root.Path(), "cmd/main.go", "package main\n")
	writeTestFile(t, root.Path(), ".git/config", "[core]\n")

	result := executeTool(t, ListFiles{Root: root}, `{}`)
	if !strings.Contains(result, "cmd/main.go") || strings.Contains(result, ".git/config") {
		t.Fatalf("list result = %s", result)
	}
}

func TestSearchFilesScansPastNonMatchingFiles(t *testing.T) {
	root := testWorkspace(t)
	for index := 0; index < 10; index++ {
		writeTestFile(t, root.Path(), filepath.Join("a", string(rune('a'+index))+".txt"), "nothing here\n")
	}
	writeTestFile(t, root.Path(), "z/target.txt", "implement feature flag\n")

	result := executeTool(t, SearchFiles{Root: root, MaxResults: 1}, `{"query":"feature flag"}`)
	if !strings.Contains(result, "z/target.txt") || !strings.Contains(result, "implement feature flag") {
		t.Fatalf("search result = %s", result)
	}
}

func TestApplyPatchChangesOnlyWorktree(t *testing.T) {
	root := testWorkspace(t)
	initializeGitRepository(t, root.Path())
	writeTestFile(t, root.Path(), "hello.txt", "old\n")
	patch := "--- a/hello.txt\n+++ b/hello.txt\n@@ -1 +1 @@\n-old\n+new\n"

	result := executeTool(t, ApplyPatch{Root: root}, `{"patch":`+quoteJSON(patch)+`}`)
	if !strings.Contains(result, `"applied":true`) {
		t.Fatalf("patch result = %s", result)
	}
	contents, err := os.ReadFile(filepath.Join(root.Path(), "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "new\n" {
		t.Fatalf("patched contents = %q", contents)
	}
}

func TestRunCommandRequiresExactPolicyMatch(t *testing.T) {
	root := testWorkspace(t)
	tool := RunCommand{Root: root, Policy: CommandPolicy{Allowed: [][]string{{"go", "version"}}}}

	result := executeTool(t, tool, `{"argv":["go","version"]}`)
	if !strings.Contains(result, `"exit_code":0`) {
		t.Fatalf("command result = %s", result)
	}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"argv":["go","env"]}`))
	if err == nil || !strings.Contains(err.Error(), "require developer approval") {
		t.Fatalf("unapproved command error = %v", err)
	}
}

func TestRunCommandRejectsArgvAndCommandTogether(t *testing.T) {
	root := testWorkspace(t)
	_, err := RunCommand{Root: root, Policy: CommandPolicy{Approve: allowOnce}}.Execute(context.Background(), json.RawMessage(`{"argv":["echo","hi"],"command":"echo hi"}`))
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("combined arguments error = %v", err)
	}
}

func TestRunCommandDenyDoesNotExecute(t *testing.T) {
	root := testWorkspace(t)
	marker := filepath.Join(root.Path(), "should-not-exist")
	tool := RunCommand{Root: root, Policy: CommandPolicy{Approve: denyAll}}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"argv":["touch","should-not-exist"]}`))
	if err == nil || !strings.Contains(err.Error(), "denied by developer") {
		t.Fatalf("denied command error = %v", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("denied command still created %s", marker)
	}
}

func TestRunCommandAllowOnceThenAlways(t *testing.T) {
	root := testWorkspace(t)
	memory := NewCommandMemory(nil)
	calls := 0
	tool := RunCommand{Root: root, Policy: CommandPolicy{
		Remembered: memory,
		Approve: func(context.Context, []string) (CommandDecision, error) {
			calls++
			if calls == 1 {
				return CommandAllowOnce, nil
			}
			return CommandAllowAlways, nil
		},
	}}
	if result := executeTool(t, tool, `{"argv":["echo","once"]}`); !strings.Contains(result, `"exit_code":0`) {
		t.Fatalf("allow once result = %s", result)
	}
	if result := executeTool(t, tool, `{"argv":["echo","once"]}`); !strings.Contains(result, `"exit_code":0`) {
		t.Fatalf("second allow result = %s", result)
	}
	if calls != 2 {
		t.Fatalf("approve calls = %d, want 2", calls)
	}
	if result := executeTool(t, tool, `{"argv":["echo","once"]}`); !strings.Contains(result, `"exit_code":0`) {
		t.Fatalf("always-allowed result = %s", result)
	}
	if calls != 2 {
		t.Fatalf("remembered command still requested approval: calls = %d", calls)
	}
}

func TestRunCommandShellFormUsesEffectiveArgv(t *testing.T) {
	root := testWorkspace(t)
	result := executeTool(t, RunCommand{Root: root, Policy: CommandPolicy{Approve: allowOnce}}, `{"command":"echo gator-shell"}`)
	if !strings.Contains(result, "gator-shell") || !strings.Contains(result, `"exit_code":0`) {
		t.Fatalf("shell command result = %s", result)
	}
}

func TestTerminalToolsRequireApprovalAndRedactInputFromEvents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the PTY dependency reports unsupported on Windows")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is unavailable")
	}
	root := testWorkspace(t)
	manager := terminal.New(terminal.Config{Root: root, Policy: sandbox.Policy{Mode: sandbox.Off}})
	defer manager.Close()
	denied := TerminalTools(manager, CommandPolicy{Approve: denyAll})[0]
	if _, err := denied.Execute(context.Background(), json.RawMessage(`{"argv":["`+sh+`","-lc","sleep 1"]}`)); err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("denied terminal start = %v", err)
	}
	if len(manager.List()) != 0 {
		t.Fatalf("denied terminal start created a task: %#v", manager.List())
	}

	var approvals [][]string
	var events []agent.Event
	surface := TerminalTools(manager, CommandPolicy{
		Remembered: NewCommandMemory(nil),
		Approve: func(_ context.Context, argv []string) (CommandDecision, error) {
			approvals = append(approvals, append([]string(nil), argv...))
			return CommandAllowOnce, nil
		},
		OnEvent: func(event agent.Event) { events = append(events, event) },
	})
	start := surface[0]
	result := executeTool(t, start, `{"argv":["`+sh+`","-lc","IFS= read line; printf 'input:%s\\n' \"$line\""]}`)
	var started struct {
		Result terminal.Task `json:"result"`
	}
	if err := json.Unmarshal([]byte(result), &started); err != nil || started.Result.ID == "" {
		t.Fatalf("terminal start result = %q, %v", result, err)
	}
	write := surface[2]
	if _, err := write.Execute(context.Background(), json.RawMessage(`{"id":"`+started.Result.ID+`","input":"private value\r"}`)); err != nil {
		t.Fatalf("terminal write: %v", err)
	}
	if len(approvals) != 2 || len(approvals[1]) != 3 || approvals[1][0] != "terminal_write" || strings.Contains(strings.Join(approvals[1], " "), "private value") {
		t.Fatalf("terminal approvals = %#v", approvals)
	}
	for _, event := range events {
		if strings.Contains(event.Text, "private value") || strings.Contains(strings.Join(event.Argv, " "), "private value") {
			t.Fatalf("terminal input leaked into approval event: %#v", event)
		}
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if task := manager.List()[0]; task.Status == "exited" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("terminal task did not exit after approved input")
}

func TestTerminalDetachRequiresInteractiveCapabilityAndSeparateApproval(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the PTY dependency reports unsupported on Windows")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is unavailable")
	}
	root := testWorkspace(t)
	registry := terminal.NewRegistry()
	defer registry.Close()
	manager := terminal.New(terminal.Config{Root: root, Policy: sandbox.Policy{Mode: sandbox.Off}, Registry: registry})
	if hasToolDefinition(TerminalTools(manager, CommandPolicy{Approve: allowOnce}), "terminal_detach") {
		t.Fatal("noninteractive terminal tools exposed terminal_detach")
	}
	var approvals [][]string
	surface := TerminalTools(manager, CommandPolicy{Approve: func(_ context.Context, argv []string) (CommandDecision, error) {
		approvals = append(approvals, append([]string(nil), argv...))
		return CommandAllowOnce, nil
	}}, TerminalToolOptions{AllowDetach: true})
	start := surface[0]
	result := executeTool(t, start, `{"argv":["`+sh+`","-lc","sleep 30"]}`)
	var started struct {
		Result terminal.Task `json:"result"`
	}
	if err := json.Unmarshal([]byte(result), &started); err != nil || started.Result.ID == "" {
		t.Fatalf("terminal start result = %q, %v", result, err)
	}
	var detach agent.Tool
	for _, tool := range surface {
		if tool.Definition().Name == "terminal_detach" {
			detach = tool
			break
		}
	}
	if detach == nil {
		t.Fatal("interactive terminal tools omitted terminal_detach")
	}
	detachedResult := executeTool(t, detach, `{"id":"`+started.Result.ID+`"}`)
	var detached struct {
		Result terminal.Task `json:"result"`
	}
	if err := json.Unmarshal([]byte(detachedResult), &detached); err != nil || !detached.Result.Background {
		t.Fatalf("terminal detach result = %q, %v", detachedResult, err)
	}
	if len(approvals) != 2 || approvals[1][0] != "terminal_detach" || approvals[1][1] != started.Result.ID {
		t.Fatalf("detach approvals = %#v", approvals)
	}
	manager.Close()
	if tasks := registry.Attachment().List(); len(tasks) != 1 || tasks[0].Status != "running" || !tasks[0].Background {
		t.Fatalf("detached task did not outlive manager close: %#v", tasks)
	}
}

func hasToolDefinition(surface []agent.Tool, name string) bool {
	for _, tool := range surface {
		if tool.Definition().Name == name {
			return true
		}
	}
	return false
}

func allowOnce(context.Context, []string) (CommandDecision, error) {
	return CommandAllowOnce, nil
}

func denyAll(context.Context, []string) (CommandDecision, error) {
	return CommandDeny, nil
}

func TestGitDiffIncludesUntrackedFiles(t *testing.T) {
	root := testWorkspace(t)
	initializeGitRepository(t, root.Path())
	writeTestFile(t, root.Path(), "new_feature.go", "package feature\n")

	result := executeTool(t, GitDiff{Root: root}, `{}`)
	if !strings.Contains(result, "new_feature.go") || !strings.Contains(result, "new file mode") {
		t.Fatalf("diff result = %s", result)
	}
}

func TestDecodeArgumentsRejectsTrailingJSON(t *testing.T) {
	var destination struct{}
	err := decodeArguments(json.RawMessage(`{} {}`), &destination)
	if err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("decode trailing JSON error = %v", err)
	}
}

func testWorkspace(t *testing.T) workspace.Root {
	t.Helper()
	root, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	return root
}

func writeTestFile(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func initializeGitRepository(t *testing.T, directory string) {
	t.Helper()
	command := exec.Command("git", "init", "--quiet")
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("initialize git repository: %v: %s", err, output)
	}
}

func executeTool(t *testing.T, tool agent.Tool, arguments string) string {
	t.Helper()
	result, err := tool.Execute(context.Background(), json.RawMessage(arguments))
	if err != nil {
		t.Fatalf("execute %s: %v", tool.Definition().Name, err)
	}
	return result.Content
}

func quoteJSON(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
