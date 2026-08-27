package eval

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
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/sandbox"
)

func TestLoadSpecReadsCheckedInGreetingFixture(t *testing.T) {
	spec, err := LoadSpec("testdata/greeting")
	if err != nil {
		t.Fatal(err)
	}
	if spec.ID != "greeting-feature" || spec.MaxSteps != 8 || len(spec.Verify) != 1 {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestLoadSpecRejectsIncompleteContracts(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "eval.json"), []byte(`{"version":1,"id":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSpec(directory); err == nil {
		t.Fatal("incomplete spec was accepted")
	}
}

func TestRunRecordsResolvedOfflineEvaluation(t *testing.T) {
	repository := greetingRepository(t)
	spec := Spec{
		Version:        1,
		ID:             "greeting-feature",
		Task:           "Add a Greeting feature with a focused test",
		MaxSteps:       8,
		TimeoutSeconds: 60,
		Verify:         [][]string{{"go", "test", "./..."}},
		Sandbox:        "off",
		Network:        "deny",
	}
	model := &scriptedModel{turns: greetingTurns(t)}
	report, err := Run(context.Background(), Options{
		Spec:       spec,
		RunID:      "eval-greeting-001",
		Provider:   "test",
		ModelName:  "scripted",
		StateDir:   t.TempDir(),
		Repository: repository,
		Executor:   gatorrun.Executor{Model: model, Sandbox: PolicyFromSpec(spec), Now: func() time.Time { return time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC) }},
	})
	if err != nil {
		t.Fatalf("run eval: %v", err)
	}
	if report.Status != "resolved" || report.Steps != 5 || report.Error != "" || report.StatePath == "" {
		t.Fatalf("report = %#v", report)
	}
	if _, err := os.Stat(filepath.Join(report.WorktreePath, "greeting.go")); err != nil {
		t.Fatalf("evaluated worktree missing greeting.go: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repository, "greeting.go")); !os.IsNotExist(err) {
		t.Fatalf("eval mutated the fixture checkout: %v", err)
	}
}

func TestRunRecordsUnresolvedWhenVerifierFails(t *testing.T) {
	repository := greetingRepository(t)
	spec := Spec{
		Version:        1,
		ID:             "greeting-feature",
		Task:           "Add a Greeting feature with a focused test",
		MaxSteps:       4,
		TimeoutSeconds: 60,
		Verify:         [][]string{{"go", "test", "./..."}},
	}
	model := &scriptedModel{turns: []agent.Turn{{Text: "I did not change anything."}}}
	report, err := Run(context.Background(), Options{
		Spec:       spec,
		RunID:      "eval-greeting-unresolved",
		Provider:   "test",
		ModelName:  "scripted",
		StateDir:   t.TempDir(),
		Repository: repository,
		Executor:   gatorrun.Executor{Model: model, Sandbox: sandbox.Policy{Mode: sandbox.Off}, Now: func() time.Time { return time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC) }},
	})
	if err != nil {
		t.Fatalf("run eval: %v", err)
	}
	if report.Status != "unresolved" || report.Error == "" {
		t.Fatalf("unresolved report = %#v", report)
	}
}

func TestWriteReportOmitsSecretsAndRequiresRunID(t *testing.T) {
	_, err := Run(context.Background(), Options{Spec: Spec{ID: "x", Task: "t", MaxSteps: 1, Verify: [][]string{{"true"}}}, Repository: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "run_id") {
		t.Fatalf("missing run_id = %v", err)
	}
	path := filepath.Join(t.TempDir(), "report.json")
	report := Report{ID: "greeting-feature", RunID: "eval-greeting-001", Status: "resolved", Provider: "test"}
	if err := WriteReport(path, report); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "sk-") || strings.Contains(string(contents), "api_key") {
		t.Fatalf("report leaked a secret: %s", contents)
	}
	var decoded Report
	if err := json.Unmarshal(contents, &decoded); err != nil || decoded.RunID != "eval-greeting-001" {
		t.Fatalf("report = %s, %v", contents, err)
	}
}

func greetingRepository(t *testing.T) string {
	t.Helper()
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "init", "--quiet")
	if err := os.WriteFile(filepath.Join(repository, "go.mod"), []byte("module example.com/feature\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("# Feature Fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "go.mod", "README.md")
	runGit(t, repository, "-c", "user.name=Gator Test", "-c", "user.email=gator@example.invalid", "commit", "--quiet", "-m", "fixture")
	return repository
}

func greetingTurns(t *testing.T) []agent.Turn {
	t.Helper()
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
	encoded, err := json.Marshal(struct {
		Patch string `json:"patch"`
	}{Patch: patch})
	if err != nil {
		t.Fatal(err)
	}
	return []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "patch", Name: "apply_patch", Arguments: encoded}}},
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "test", Name: "run_command", Arguments: json.RawMessage(`{"argv":["go","test","./..."]}`)}}},
		{Text: "Added Greeting with its focused test."},
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

type scriptedModel struct {
	turns    []agent.Turn
	requests []agent.TurnRequest
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
