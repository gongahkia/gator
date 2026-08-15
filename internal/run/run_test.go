package run

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/harness"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/worktree"
)

func TestExecutorCompletesReviewableFeatureRun(t *testing.T) {
	repository := featureRepository(t)
	patch := "diff --git a/greeting.go b/greeting.go\n" +
		"new file mode 100644\n" +
		"--- /dev/null\n" +
		"+++ b/greeting.go\n" +
		"@@ -0,0 +1,5 @@\n" +
		"+package feature\n" +
		"+\n" +
		"+func Greeting(name string) string {\n" +
		"+\treturn \"Hello, \" + name\n" +
		"+}\n" +
		"diff --git a/greeting_test.go b/greeting_test.go\n" +
		"new file mode 100644\n" +
		"--- /dev/null\n" +
		"+++ b/greeting_test.go\n" +
		"@@ -0,0 +1,9 @@\n" +
		"+package feature\n" +
		"+\n" +
		"+import \"testing\"\n" +
		"+\n" +
		"+func TestGreeting(t *testing.T) {\n" +
		"+\tif got := Greeting(\"Ada\"); got != \"Hello, Ada\" {\n" +
		"+\t\tt.Fatalf(\"Greeting() = %q\", got)\n" +
		"+\t}\n" +
		"+}\n"
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "patch", Name: "apply_patch", Arguments: objectArguments(t, struct {
			Patch string `json:"patch"`
		}{Patch: patch})}}},
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "test", Name: "run_command", Arguments: json.RawMessage(`{"argv":["go","test","./..."]}`)}}},
		{Text: "Added Greeting with its focused test. `go test ./...` passed; the patch is ready for review."},
	}}
	executor := Executor{
		Model: model,
		Now:   func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) },
	}

	outcome, err := executor.Execute(context.Background(), Request{
		RepositoryPath: repository,
		Task:           "Add a Greeting feature with a focused test",
		Provider:       "test",
		Model:          "test-model",
		RunID:          "run_feature_001",
		MaxSteps:       8,
		Verification:   [][]string{{"go", "test", "./..."}},
		StateDir:       t.TempDir(),
	})
	if err != nil {
		t.Fatalf("execute feature run: %v", err)
	}
	if outcome.Result.FinalText == "" || outcome.Result.Steps != 5 {
		t.Fatalf("outcome = %#v", outcome)
	}
	if !strings.Contains(model.requests[0].System, "Gator, a careful coding agent") {
		t.Fatalf("system prompt was not sent: %q", model.requests[0].System)
	}
	if _, err := os.Stat(filepath.Join(outcome.Worktree.Path, "greeting.go")); err != nil {
		t.Fatalf("feature file missing from worktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outcome.StatePath, "events.jsonl")); err != nil {
		t.Fatalf("run journal missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outcome.Worktree.Path, "greeting_test.go")); err != nil {
		t.Fatalf("feature test missing from worktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repository, "greeting.go")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("active checkout changed, stat error = %v", err)
	}
	if !containsEvent(outcome.Events, agent.EventRunFinished) {
		t.Fatalf("events did not contain completed run: %#v", outcome.Events)
	}
}

func TestExecutorRetainsWorktreeWhenEvidenceIsMissing(t *testing.T) {
	repository := featureRepository(t)
	model := &scriptedModel{turns: []agent.Turn{{Text: "Done."}}}
	executor := Executor{Model: model}

	outcome, err := executor.Execute(context.Background(), Request{
		RepositoryPath: repository,
		Task:           "Add a feature",
		Provider:       "test",
		Model:          "test-model",
		RunID:          "run_missing_evidence",
		MaxSteps:       2,
		StateDir:       t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "unexpected model call") {
		t.Fatalf("execute error = %v", err)
	}
	if outcome.Worktree.Path == "" {
		t.Fatal("failed run did not retain worktree details")
	}
	if !containsEvent(outcome.Events, agent.EventCompletionBlocked) {
		t.Fatalf("events did not record blocked completion: %#v", outcome.Events)
	}
}

func TestValidateVerification(t *testing.T) {
	if err := validateVerification([][]string{{}}); err == nil {
		t.Fatal("empty verification command was accepted")
	}
	if err := validateVerification([][]string{{"go", "test", "./..."}}); err != nil {
		t.Fatalf("valid verification command: %v", err)
	}
}

func TestExecutorLoadsRootAgentInstructions(t *testing.T) {
	repository := featureRepository(t)
	writeFile(t, repository, "AGENTS.md", "Always name the feature tests clearly.\n")
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}},
		{Text: "No changes were needed."},
	}}

	_, err := (Executor{Model: model}).Execute(context.Background(), Request{
		RepositoryPath: repository,
		Task:           "Inspect the fixture",
		Provider:       "test",
		Model:          "test-model",
		RunID:          "run_instructions_001",
		MaxSteps:       4,
		StateDir:       t.TempDir(),
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(model.requests[0].System, "Always name the feature tests clearly.") {
		t.Fatalf("system prompt did not include AGENTS.md: %q", model.requests[0].System)
	}
}

func TestExecutorResumesWithFreshEvidence(t *testing.T) {
	repository := featureRepository(t)
	retained, err := worktree.Create(context.Background(), repository, "run_original_001")
	if err != nil {
		t.Fatalf("create retained worktree: %v", err)
	}
	previous := journal.Session{
		Version:      2,
		Repository:   repository,
		WorktreePath: retained.Path,
		Provider:     "test",
		Model:        "test-model",
		Task:         "Add a feature",
		MaxSteps:     4,
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "Add a feature"},
			{Role: agent.RoleTool, ToolName: "git_status", ToolCallID: "old-status", Content: `{"ok":true}`},
			{Role: agent.RoleTool, ToolName: "git_diff", ToolCallID: "old-diff", Content: `{"ok":true}`},
		},
	}
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}},
		{Text: "The retained patch is ready for review."},
	}}
	stateDirectory := t.TempDir()
	executor := Executor{Model: model}

	outcome, err := executor.Resume(context.Background(), previous, "/state/original", "Review the existing patch and finish.", Request{StateDir: stateDirectory})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if outcome.Worktree.Path != retained.Path || outcome.Result.FinalText == "" {
		t.Fatalf("resume outcome = %#v", outcome)
	}
	if got := model.requests[0].Messages[len(model.requests[0].Messages)-1]; got.Role != agent.RoleUser || !strings.Contains(got.Content, "Review the existing patch") {
		t.Fatalf("resume history = %#v", model.requests[0].Messages)
	}
	current, err := journal.LoadSession(outcome.StatePath)
	if err != nil {
		t.Fatalf("load resumed session: %v", err)
	}
	if current.ParentStatePath != "/state/original" {
		t.Fatalf("resumed session parent = %q", current.ParentStatePath)
	}
}

func TestExecutorVerifiesDelegatedCLIHarnessOutcome(t *testing.T) {
	repository := featureRepository(t)
	delegated := &recordingHarness{run: func(_ context.Context, request harness.Request) (harness.Result, error) {
		if err := os.WriteFile(filepath.Join(request.Root, "delegated.go"), []byte("package feature\n"), 0o644); err != nil {
			return harness.Result{}, err
		}
		return harness.Result{FinalText: "Delegated implementation complete."}, nil
	}}
	executor := Executor{Harness: delegated}
	outcome, err := executor.Execute(context.Background(), Request{
		RepositoryPath:   repository,
		Task:             "Add a delegated feature",
		Provider:         "codex",
		RunID:            "run_harness_001",
		Verification:     [][]string{{"go", "test", "./..."}},
		StateDir:         t.TempDir(),
		AllowExternalCLI: true,
	})
	if err != nil {
		t.Fatalf("execute harness: %v", err)
	}
	if delegated.request.Root != outcome.Worktree.Root.Path() || !containsEvent(outcome.Events, agent.EventHarnessStarted) || !containsEvent(outcome.Events, agent.EventRunFinished) {
		t.Fatalf("outcome = %#v, request = %#v", outcome, delegated.request)
	}
	if _, err := os.Stat(filepath.Join(outcome.Worktree.Path, "delegated.go")); err != nil {
		t.Fatalf("delegated patch missing: %v", err)
	}
	current, err := journal.LoadSession(outcome.StatePath)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if current.Version != 2 || current.Provider != "codex" {
		t.Fatalf("session = %#v", current)
	}
}

func TestExecutorRequiresExplicitExternalCLIApproval(t *testing.T) {
	_, err := (Executor{Harness: &recordingHarness{}}).Execute(context.Background(), Request{
		RepositoryPath: featureRepository(t),
		Task:           "Add a feature",
		Provider:       "codex",
		Verification:   [][]string{{"go", "test", "./..."}},
	})
	if err == nil || !strings.Contains(err.Error(), "explicit approval") {
		t.Fatalf("execute error = %v", err)
	}
}

type scriptedModel struct {
	turns    []agent.Turn
	requests []agent.TurnRequest
}

type recordingHarness struct {
	run     func(context.Context, harness.Request) (harness.Result, error)
	request harness.Request
}

func (h *recordingHarness) Run(ctx context.Context, request harness.Request) (harness.Result, error) {
	h.request = request
	if request.OnEvent != nil {
		request.OnEvent(agent.Event{Kind: agent.EventHarnessStarted, Step: 1, Text: "test"})
	}
	if h.run == nil {
		return harness.Result{}, errors.New("unexpected harness call")
	}
	result, err := h.run(ctx, request)
	if request.OnEvent != nil {
		event := agent.Event{Kind: agent.EventHarnessFinished, Step: 1, Text: "test"}
		if err != nil {
			event.ToolError = err.Error()
		}
		request.OnEvent(event)
	}
	return result, err
}

func (m *scriptedModel) Complete(_ context.Context, request agent.TurnRequest) (agent.Turn, error) {
	m.requests = append(m.requests, request)
	if len(m.turns) == 0 {
		return agent.Turn{}, errors.New("unexpected model call")
	}
	turn := m.turns[0]
	m.turns = m.turns[1:]
	return turn, nil
}

func objectArguments(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode arguments: %v", err)
	}
	return encoded
}

func containsEvent(events []agent.Event, kind agent.EventKind) bool {
	for _, event := range events {
		if event.Kind == kind {
			return true
		}
	}
	return false
}

func featureRepository(t *testing.T) string {
	t.Helper()
	repository := filepath.Join(t.TempDir(), "feature-repository")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "init", "--quiet")
	writeFile(t, repository, "go.mod", "module example.com/feature\n\ngo 1.25.0\n")
	writeFile(t, repository, "README.md", "# Feature Fixture\n")
	runGit(t, repository, "add", "go.mod", "README.md")
	runGit(t, repository, "-c", "user.name=Gator Test", "-c", "user.email=gator@example.invalid", "commit", "--quiet", "-m", "fixture")
	return repository
}

func writeFile(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
}
