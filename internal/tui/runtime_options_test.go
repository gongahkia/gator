package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/journal"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/sandbox"
)

func TestDoctorScreenIsInspectOnlyAndOmitsSecrets(t *testing.T) {
	backend := &fakeDoctorBackend{snapshot: DoctorSnapshot{
		Provider: "openai", AuthKind: "Gator credential", AuthStatus: "stored",
		Sandbox: "available", WebSearchStatus: "configured (requires --network allow)",
		SuggestedVerify:  []string{"go test ./..."},
		EffectiveSandbox: "strict", EffectiveNetwork: "deny",
	}}
	model := New(Config{Doctor: backend})
	model.width, model.height = 100, 40
	model.provider.SetValue("openai")
	model.task.SetValue("/doctor")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if command == nil {
		t.Fatal("doctor did not load a snapshot")
	}
	next, _ = model.Update(command())
	model = next.(Model)
	if model.screen != doctorScreen || backend.calls != 1 {
		t.Fatalf("doctor screen=%d calls=%d", model.screen, backend.calls)
	}
	view := model.View()
	if strings.Contains(view, "sk-") || strings.Contains(view, "secret") {
		t.Fatalf("doctor view leaked a secret: %q", view)
	}
	if !strings.Contains(view, "Inspect-only") && !strings.Contains(model.notice.text, "Inspect-only") {
		t.Fatalf("doctor missing inspect-only notice: %q", view)
	}
	if !strings.Contains(view, "go test ./...") {
		t.Fatalf("doctor view missing suggested verify: %q", view)
	}
}

func TestRunEditorRejectsShellPrefixAndRequiresCopyIgnoredConfirm(t *testing.T) {
	model := New(Config{})
	model.width, model.height = 120, 48
	model.task.SetValue("/run")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.screen != runOptionsScreen {
		t.Fatalf("run options screen = %d", model.screen)
	}
	model.runOptions.prefixes.SetValue("bash -lc echo")
	model.refreshPreflight()
	if len(model.collectRunOptionIssues()) == 0 {
		t.Fatal("shell prefix was accepted")
	}
	model.runOptions.prefixes.SetValue("go test")
	if issues := model.collectRunOptionIssues(); len(issues) != 0 {
		t.Fatalf("literal prefix rejected: %v", issues)
	}
	model.runOptions.field = runFieldCopyIgnored
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	model = next.(Model)
	if !model.runOptions.copyIgnored || model.runOptions.copyIgnoredAck || model.runOptions.confirm != "copy-ignored" {
		t.Fatalf("copy-ignored confirm = %#v", model.runOptions)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = next.(Model)
	model.runOptions.copyIgnored = true
	model.runOptions.copyIgnoredAck = false
	model.config.NewExecutor = func(string, string, string) (gatorrun.Executor, error) {
		return gatorrun.Executor{}, nil
	}
	model.screen = composeScreen
	model.task.SetValue("implement the feature")
	model.verification.SetValue("go test ./...")
	next, _ = model.startRun()
	model = next.(Model)
	if model.screen != runOptionsScreen || model.runOptions.confirm != "copy-ignored" {
		t.Fatalf("start without ack = screen %d confirm %q notice %q", model.screen, model.runOptions.confirm, model.notice.text)
	}
}

func TestOneRunBaseURLReachesExecutorWithoutChangingConfig(t *testing.T) {
	var seen string
	model := New(Config{
		RepositoryPath: testRepository(t),
		Provider:       "openai",
		Model:          "test-model",
		BaseURL:        "https://config.example/v1",
		Verification:   [][]string{{"go", "test", "./..."}},
		NewExecutor: func(_, _, baseURL string) (gatorrun.Executor, error) {
			seen = baseURL
			return gatorrun.Executor{}, errors.New("stop after capturing base URL")
		},
	})
	const override = "https://run.example/v1"
	model.runOptions.baseURL.SetValue(override)
	model.task.SetValue("implement the feature")
	next, _ := model.startRun()
	model = next.(Model)
	if seen != override {
		t.Fatalf("executor base URL = %q, want %q", seen, override)
	}
	if model.config.BaseURL != "https://config.example/v1" {
		t.Fatalf("config base URL mutated: %q", model.config.BaseURL)
	}
}

func TestSandboxOffRequiresConfirmationBeforeStart(t *testing.T) {
	model := New(Config{
		RepositoryPath: testRepository(t),
		Provider:       "openai",
		Model:          "test-model",
		Verification:   [][]string{{"go", "test", "./..."}},
		Execution:      sandbox.Policy{Mode: sandbox.Strict, Network: sandbox.DenyNetwork},
		NewExecutor: func(string, string, string) (gatorrun.Executor, error) {
			return gatorrun.Executor{}, nil
		},
	})
	model.runOptions.sandboxMode = string(sandbox.Off)
	model.runOptions.sandboxAck = false
	model.task.SetValue("implement the feature")
	next, _ := model.startRun()
	model = next.(Model)
	if model.screen != runOptionsScreen || model.runOptions.confirm != "sandbox-off" {
		t.Fatalf("start without sandbox ack = screen %d confirm %q", model.screen, model.runOptions.confirm)
	}
	executor, err := model.applyExecutorOverrides(gatorrun.Executor{})
	if err != nil {
		t.Fatal(err)
	}
	if executor.Sandbox.Mode != sandbox.Off {
		t.Fatalf("effective sandbox = %q", executor.Sandbox.Mode)
	}
	if model.config.Execution.Normalize().Mode != sandbox.Strict {
		t.Fatalf("config sandbox mutated: %#v", model.config.Execution)
	}
}

func TestResumeTargetWithInstructionBindsAndStarts(t *testing.T) {
	repository, stateDirectory, record := retainedRunRecord(t)
	var seenURL string
	calls := 0
	model := New(Config{
		RepositoryPath: repository,
		StateDir:       stateDirectory,
		Provider:       "openai",
		Model:          "gpt-5.6",
		NewExecutor: func(_, _, baseURL string) (gatorrun.Executor, error) {
			calls++
			seenURL = baseURL
			return gatorrun.Executor{}, errors.New("stop after bind")
		},
	})
	model.runOptions.baseURL.SetValue("https://run.example/v1")
	model.task.SetValue("/resume " + record.StatePath + " continue the retained patch")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.resumeStatePath != record.StatePath {
		t.Fatalf("resume path = %q, want %q notice %q", model.resumeStatePath, record.StatePath, model.notice.text)
	}
	if calls == 0 || seenURL != "https://run.example/v1" {
		t.Fatalf("executor calls=%d url=%q", calls, seenURL)
	}
}

func TestReviewTargetLoadsOutcomeFromRunRecord(t *testing.T) {
	repository, stateDirectory, record := retainedRunRecord(t)
	model := New(Config{RepositoryPath: repository, StateDir: stateDirectory})
	model.task.SetValue("/review " + record.StatePath)
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.screen != reviewScreen || model.outcome == nil || model.outcome.StatePath != record.StatePath {
		t.Fatalf("review target screen=%d outcome=%#v notice=%q", model.screen, model.outcome, model.notice.text)
	}
	if command == nil {
		t.Fatal("review target did not load a snapshot")
	}
}

func TestBrowserReviewRejectsNonLoopbackAndStartsOnLoopback(t *testing.T) {
	repository, stateDirectory, record := retainedRunRecord(t)
	opened := ""
	model := New(Config{
		RepositoryPath: repository,
		StateDir:       stateDirectory,
		OpenBrowser: func(url string) error {
			opened = url
			return nil
		},
	})
	model.outcome = &gatorrun.Outcome{StatePath: record.StatePath}
	model.reviewWeb.listen.SetValue("0.0.0.0:39001")
	next, command := model.startReviewWeb()
	model = next.(Model)
	if command != nil || !strings.Contains(model.notice.text, "loopback") {
		t.Fatalf("non-loopback listen command=%v notice=%q", command != nil, model.notice.text)
	}

	model.reviewWeb.listen.SetValue("127.0.0.1:0")
	model.reviewWeb.openBrowser = true
	next, command = model.startReviewWeb()
	model = next.(Model)
	if command == nil {
		t.Fatal("loopback browser review did not start")
	}
	msg, ok := command().(reviewWebStartedMsg)
	if !ok {
		t.Fatalf("start message type = %T", command())
	}
	model.applyReviewWebStarted(msg)
	defer model.stopReviewWeb()
	if msg.err != nil || msg.server == nil || !strings.Contains(model.reviewWeb.url, "http://") {
		t.Fatalf("started = %#v url=%q", msg, model.reviewWeb.url)
	}
	if !strings.HasPrefix(opened, "http://") || strings.Contains(opened, "0.0.0.0") {
		t.Fatalf("opened URL = %q", opened)
	}
	if strings.Contains(model.commandOutput, model.reviewWeb.url) || strings.Contains(model.plainTranscript(), model.reviewWeb.url) {
		t.Fatalf("one-use review URL leaked into command output or transcript")
	}
}

func retainedRunRecord(t *testing.T) (string, string, journal.Record) {
	t.Helper()
	repository := testRepository(t)
	stateDirectory := t.TempDir()
	entry, record, err := journal.Open(repository, "run-tui-target-001", repository, stateDirectory, time.Now())
	if err != nil {
		t.Fatalf("open run journal: %v", err)
	}
	if err := entry.SaveSession(journal.Session{
		Version: 2, Repository: repository, WorktreePath: repository, Provider: "openai", Model: "gpt-5.6",
		Task: "Retained task", ThreadID: "thread-tui-target", Verification: [][]string{{"go", "test", "./..."}},
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if err := entry.Close(); err != nil {
		t.Fatalf("close journal: %v", err)
	}
	if err := journal.SaveThread(stateDirectory, journal.Thread{
		Version: 1, ID: "thread-tui-target", Repository: repository, WorktreePath: repository,
		Provider: "openai", Model: "gpt-5.6", Task: "Retained task", HeadStatePath: record.StatePath, TurnCount: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("save thread: %v", err)
	}
	return repository, stateDirectory, record
}

type fakeDoctorBackend struct {
	snapshot DoctorSnapshot
	calls    int
}

func (backend *fakeDoctorBackend) Snapshot(string) (DoctorSnapshot, error) {
	backend.calls++
	return backend.snapshot, nil
}
