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
		RunID:          "run_feature_001",
		MaxSteps:       8,
		Verification:   [][]string{{"go", "test", "./..."}},
	})
	if err != nil {
		t.Fatalf("execute feature run: %v", err)
	}
	if outcome.Result.FinalText == "" || outcome.Result.Steps != 5 {
		t.Fatalf("outcome = %#v", outcome)
	}
	if _, err := os.Stat(filepath.Join(outcome.Worktree.Path, "greeting.go")); err != nil {
		t.Fatalf("feature file missing from worktree: %v", err)
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
		RunID:          "run_missing_evidence",
		MaxSteps:       2,
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

type scriptedModel struct {
	turns []agent.Turn
}

func (m *scriptedModel) Complete(_ context.Context, _ agent.TurnRequest) (agent.Turn, error) {
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
