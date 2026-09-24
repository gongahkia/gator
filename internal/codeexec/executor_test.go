package codeexec

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/sandbox"
)

type scriptedModel struct {
	turns []agent.Turn
	next  int
}

func (m *scriptedModel) Complete(_ context.Context, _ agent.TurnRequest) (agent.Turn, error) {
	if m.next >= len(m.turns) {
		return agent.Turn{Text: "done"}, nil
	}
	turn := m.turns[m.next]
	m.next++
	return turn, nil
}

func codeCall(name string, value any) agent.Turn {
	payload, _ := json.Marshal(value)
	return agent.Turn{ToolCalls: []agent.ToolCall{{ID: name, Name: name, Arguments: payload}}}
}

func TestExecuteRetainsIsolatedWorktreeAndVerificationEvidence(t *testing.T) {
	repository := codeRepository(t)
	model := &scriptedModel{turns: []agent.Turn{
		codeCall("apply_patch", map[string]string{"patch": "diff --git a/greeting.txt b/greeting.txt\n--- a/greeting.txt\n+++ b/greeting.txt\n@@ -1 +1 @@\n-base\n+changed\n"}),
		codeCall("git_status", map[string]any{}),
		codeCall("git_diff", map[string]any{}),
		codeCall("run_command", map[string]any{"argv": []string{"git", "diff", "--check"}}),
		{Text: "Changed greeting with verification."},
	}}
	outcome, err := (Executor{Model: model, Sandbox: sandbox.Policy{Mode: sandbox.Off}}).Execute(context.Background(), Request{
		RepositoryPath: repository, RunID: "codeexec-success-001", Task: "Change greeting", MaxSteps: 8,
		Verification: [][]string{{"git", "diff", "--check"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Worktree.Path == "" || outcome.Worktree.Path == repository || !strings.Contains(outcome.Worktree.Path, "-gator-runs") {
		t.Fatalf("isolated worktree = %#v", outcome.Worktree)
	}
	contents, err := os.ReadFile(filepath.Join(outcome.Worktree.Path, "greeting.txt"))
	if err != nil || string(contents) != "changed\n" {
		t.Fatalf("isolated change = %q, %v", contents, err)
	}
	original, err := os.ReadFile(filepath.Join(repository, "greeting.txt"))
	if err != nil || string(original) != "base\n" {
		t.Fatalf("source mutated = %q, %v", original, err)
	}
	if outcome.Result.FinalText == "" || !hasToolEvent(outcome.Events, "run_command") {
		t.Fatalf("result/evidence = %#v", outcome)
	}
}

func TestExecuteRetainsWorktreeWhenCompletionFails(t *testing.T) {
	repository := codeRepository(t)
	outcome, err := (Executor{Model: &scriptedModel{turns: []agent.Turn{{Text: "I changed nothing."}}}, Sandbox: sandbox.Policy{Mode: sandbox.Off}}).Execute(context.Background(), Request{
		RepositoryPath: repository, RunID: "codeexec-failed-001", Task: "Change greeting", MaxSteps: 1,
		Verification: [][]string{{"git", "diff", "--check"}},
	})
	if err == nil || !strings.Contains(err.Error(), "step limit") {
		t.Fatalf("execution error = %v", err)
	}
	if outcome.Worktree.Path == "" {
		t.Fatalf("failed Code worktree was not retained: %#v", outcome)
	}
	if _, statErr := os.Stat(outcome.Worktree.Path); statErr != nil {
		t.Fatalf("failed Code worktree is unavailable: %v", statErr)
	}
}

func hasToolEvent(events []agent.Event, name string) bool {
	for _, event := range events {
		if event.ToolCall != nil && event.ToolCall.Name == name {
			return true
		}
	}
	return false
}

func codeRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	if err := os.WriteFile(filepath.Join(repository, "greeting.txt"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "greeting.txt"}, {"-c", "user.name=Gator", "-c", "user.email=gator@invalid", "commit", "--quiet", "-m", "base"}} {
		command := exec.Command("git", args...)
		command.Dir = repository
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
		}
	}
	return repository
}
