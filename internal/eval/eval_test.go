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

func TestLoadCheckedInLiveSmokeSuite(t *testing.T) {
	suite, err := LoadSuite("testdata")
	if err != nil {
		t.Fatal(err)
	}
	if suite.ID != "gator-live-smoke" || len(suite.Cases) != 1 {
		t.Fatalf("suite = %#v", suite)
	}
	spec, err := LoadSpec("testdata/live-greeting")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Sandbox != "strict" || len(spec.AllowedCommandPrefixes) != 1 {
		t.Fatalf("live smoke spec = %#v", spec)
	}
	turns, err := LoadScript("testdata/greeting")
	if err != nil || len(turns) != 5 {
		t.Fatalf("offline fixture script = %#v, %v", turns, err)
	}
}

func TestLoadCheckedInCoreSuiteRequiresStrictHiddenScoring(t *testing.T) {
	suite, err := LoadSuite("testdata/core-v1")
	if err != nil {
		t.Fatal(err)
	}
	if suite.ID != "gator-core-v1" || len(suite.Cases) != 4 {
		t.Fatalf("suite = %#v", suite)
	}
	for _, fixture := range suite.Cases {
		spec, err := LoadSpec(filepath.Join("testdata/core-v1", fixture))
		if err != nil {
			t.Fatal(err)
		}
		if spec.Sandbox != string(sandbox.Strict) || len(spec.Score) == 0 || spec.ScoreTimeoutSeconds < 1 || len(spec.Labels) == 0 {
			t.Fatalf("core spec %q is not a scored strict task: %#v", fixture, spec)
		}
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

func TestLoadSpecRejectsUnsafeHeadlessPolicy(t *testing.T) {
	tests := []string{
		`{"version":1,"id":"x","task":"x","max_steps":1,"verify":[["true"]],"repository":"../outside"}`,
		`{"version":1,"id":"x","task":"x","max_steps":1,"verify":[["true"]],"allowed_command_prefixes":[["sh"]]}`,
		`{"version":1,"id":"x","task":"x","max_steps":1,"verify":[["true"]],"scopes":["."]}`,
		`{"version":1,"id":"x","task":"x","max_steps":1,"verify":[["true"]],"score":[["true","{{unknown}}"]]}`,
	}
	for _, contents := range tests {
		directory := t.TempDir()
		if err := os.WriteFile(filepath.Join(directory, "eval.json"), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadSpec(directory); err == nil {
			t.Fatalf("unsafe spec was accepted: %s", contents)
		}
	}
}

func TestRunRejectsUnversionedSpecBeforeExecution(t *testing.T) {
	_, err := Run(context.Background(), Options{
		Spec:       Spec{ID: "x", Task: "x", MaxSteps: 1, Verify: [][]string{{"true"}}},
		RunID:      "unversioned-001",
		Repository: t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported eval spec version") {
		t.Fatalf("unversioned eval spec error = %v", err)
	}
}

func TestRunRecordsResolvedOfflineEvaluation(t *testing.T) {
	repository := greetingRepository(t)
	spec := Spec{
		Version:                1,
		ID:                     "greeting-feature",
		Task:                   "Add a Greeting feature with a focused test",
		MaxSteps:               8,
		TimeoutSeconds:         60,
		Verify:                 [][]string{{"go", "test", "./..."}},
		Sandbox:                "off",
		Network:                "deny",
		Scopes:                 []string{"README.md"},
		AllowedCommands:        [][]string{{"go", "env", "GOMOD"}},
		AllowedCommandPrefixes: [][]string{{"go", "test"}},
		Setup:                  [][]string{{"go", "version"}},
		Score:                  [][]string{{"go", "version"}},
	}
	model := &scriptedModel{turns: greetingTurns(t)}
	report, err := Run(context.Background(), Options{
		Spec:        spec,
		RunID:       "eval-greeting-001",
		Provider:    "test",
		ModelName:   "scripted",
		StateDir:    t.TempDir(),
		Repository:  repository,
		FixturePath: t.TempDir(),
		Executor:    gatorrun.Executor{Model: model, Sandbox: PolicyFromSpec(spec), Now: func() time.Time { return time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC) }},
	})
	if err != nil {
		t.Fatalf("run eval: %v", err)
	}
	if report.Status != "resolved" || report.AgentStatus != "resolved" || report.ScoreStatus != "passed" || len(report.ScoreResults) != 1 || report.Steps != 6 || report.Error != "" || report.StatePath == "" {
		t.Fatalf("report = %#v", report)
	}
	if len(report.AllowedCommands) != 1 || len(report.AllowedCommandPrefixes) != 1 || len(report.Setup) != 1 || len(report.Scopes) != 1 {
		t.Fatalf("report did not retain evaluation policy: %#v", report)
	}
	if _, err := os.Stat(filepath.Join(report.WorktreePath, "greeting.go")); err != nil {
		t.Fatalf("evaluated worktree missing greeting.go: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repository, "greeting.go")); !os.IsNotExist(err) {
		t.Fatalf("eval mutated the fixture checkout: %v", err)
	}
}

func TestRunScoreFailureDowngradesResolvedAgentOutcome(t *testing.T) {
	repository := greetingRepository(t)
	fixture := t.TempDir()
	spec := Spec{
		Version:        1,
		ID:             "score-failure",
		Task:           "Add a Greeting feature with a focused test",
		MaxSteps:       8,
		TimeoutSeconds: 60,
		Verify:         [][]string{{"go", "test", "./..."}},
		Sandbox:        "off",
		Score:          [][]string{{"go", "tool", "not-a-real-tool"}},
	}
	report, err := Run(context.Background(), Options{
		Spec:        spec,
		RunID:       "score-failure-001",
		Provider:    "test",
		ModelName:   "scripted",
		StateDir:    t.TempDir(),
		Repository:  repository,
		FixturePath: fixture,
		Executor:    gatorrun.Executor{Model: &scriptedModel{turns: greetingTurns(t)}, Sandbox: PolicyFromSpec(spec)},
	})
	if err != nil {
		t.Fatalf("run eval: %v", err)
	}
	if report.AgentStatus != "resolved" || report.Status != "unresolved" || report.ScoreStatus != "failed" || len(report.ScoreResults) != 1 {
		t.Fatalf("scored report = %#v", report)
	}
}

func TestFixtureSHA256ChangesOnlyWhenFixtureContentChanges(t *testing.T) {
	fixture := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixture, "case.txt"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := FixtureSHA256(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(fixture, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, ".git", "ignored"), []byte("ignored\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := FixtureSHA256(fixture)
	if err != nil || second != first {
		t.Fatalf("Git metadata changed fixture digest: %q, %v", second, err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "case.txt"), []byte("two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	third, err := FixtureSHA256(fixture)
	if err != nil || third == first {
		t.Fatalf("fixture content did not change digest: %q, %v", third, err)
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
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("report mode = %o, want 600", info.Mode().Perm())
	}
}

func TestPrepareRepositoryCreatesFreshGitBaseline(t *testing.T) {
	source := greetingRepository(t)
	repository, err := PrepareRepository(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if repository == source {
		t.Fatal("prepared repository reused fixture source")
	}
	if output := gitOutput(t, repository, "status", "--porcelain"); output != "" {
		t.Fatalf("prepared baseline is dirty: %q", output)
	}
	if output := gitOutput(t, repository, "log", "-1", "--format=%s"); output != "gator evaluation baseline" {
		t.Fatalf("prepared baseline commit = %q", output)
	}
	if err := os.WriteFile(filepath.Join(repository, "changed.txt"), []byte("isolated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(source, "changed.txt")); !os.IsNotExist(err) {
		t.Fatalf("prepared repository changed source fixture: %v", err)
	}
}

func TestCopyRepositoryRejectsSymlinks(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "file.txt"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("file.txt", filepath.Join(source, "link.txt")); err != nil {
		t.Skipf("create symlink: %v", err)
	}
	if err := CopyRepository(source, filepath.Join(t.TempDir(), "copy")); err == nil {
		t.Fatal("copy accepted a fixture symlink")
	}
}

func TestLoadAndSummarizeSuite(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "suite.json"), []byte(`{"version":1,"id":"smoke","cases":["feature","bug"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	suite, err := LoadSuite(directory)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	report := SummarizeSuite(suite, "smoke-001", "openai", "test-model", started, started.Add(time.Second), []Report{{ID: "feature", Attempt: 1, Status: "resolved", ScoreStatus: "passed"}, {ID: "feature", Attempt: 2, Status: "unresolved", ScoreStatus: "failed"}, {ID: "bug", Attempt: 1, Status: "error", ScoreStatus: "error"}})
	if report.Status != "error" || report.Resolved != 1 || report.Unresolved != 1 || report.Errors != 1 || report.ScorePassed != 1 || report.ScoreFailed != 1 || report.ScoreErrors != 1 || report.Attempts != 2 || len(report.CaseSummaries) != 2 || report.Duration != time.Second {
		t.Fatalf("suite report = %#v", report)
	}
	if err := WriteSuiteReport(filepath.Join(directory, "reports", "suite.json"), report); err != nil {
		t.Fatal(err)
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
		{ToolCalls: []agent.ToolCall{{ID: "inspect-module", Name: "run_command", Arguments: json.RawMessage(`{"argv":["go","env","GOMOD"]}`)}}},
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

func gitOutput(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
	return strings.TrimSpace(string(output))
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
