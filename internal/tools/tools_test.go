package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
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
