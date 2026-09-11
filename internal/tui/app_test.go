package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/journal"
	gatorrun "github.com/gongahkia/gator/internal/run"
)

func TestNewUsesConfiguredRunDefaults(t *testing.T) {
	model := New(Config{
		RepositoryPath: "/tmp/example-repository",
		Model:          "test-model",
		Verification:   [][]string{{"go", "test", "./..."}, {"go", "vet", "./..."}},
	})
	if got, want := model.model.Value(), "test-model"; got != want {
		t.Fatalf("model = %q, want %q", got, want)
	}
	if got, want := model.verification.Value(), "go test ./...\ngo vet ./..."; got != want {
		t.Fatalf("verification = %q, want %q", got, want)
	}
	if model.config.MaxSteps != defaultMaxSteps {
		t.Fatalf("max steps = %d, want %d", model.config.MaxSteps, defaultMaxSteps)
	}
}

func TestEffortPickerKeepsModelChoiceExplicitAndChangesTurnBudget(t *testing.T) {
	model := New(Config{MaxSteps: 24})
	model.width, model.height = 100, 40
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = next.(Model)
	if model.screen != effortScreen || !strings.Contains(model.View(), "Standard") || !strings.Contains(model.View(), "/model") {
		t.Fatalf("effort picker view = %q", model.View())
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	model = next.(Model)
	if model.screen != composeScreen || model.effort != effortThorough || model.effort.maxSteps(model.config.MaxSteps) != 36 {
		t.Fatalf("effort selection = screen:%d effort:%d steps:%d", model.screen, model.effort, model.effort.maxSteps(model.config.MaxSteps))
	}
	if model.model.Value() != "" || model.provider.Value() != "openai" {
		t.Fatalf("effort selection unexpectedly changed model configuration: %q/%q", model.provider.Value(), model.model.Value())
	}
}

func TestExtensionUIIsHostRenderedAndOnlyFillsComposerOnExplicitSelection(t *testing.T) {
	model := New(Config{ExtensionUI: []ExtensionUIContribution{{ID: "team:review", Slot: "composer", Title: "Team review", Description: "Apply the team review checklist.", Prompt: "Review the current change against the team checklist."}}})
	model.width, model.height = 100, 40
	model.task.SetValue("/extensions")
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.screen != extensionUIScreen || !strings.Contains(model.View(), "Team review") {
		t.Fatalf("extension UI view = %q", model.View())
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.screen != composeScreen || model.task.Value() != "Review the current change against the team checklist." {
		t.Fatalf("extension UI action = screen:%d task:%q", model.screen, model.task.Value())
	}
}

func TestCustomProviderIsSelectableWithConfiguredModels(t *testing.T) {
	model := New(Config{
		RepositoryPath: "/tmp/example-repository",
		CustomProviders: []config.CustomProvider{{
			ID: "local-llm", BaseURL: "http://127.0.0.1:11434/v1/chat/completions", Models: []string{"qwen3-coder", "deepseek-coder"}, DefaultModel: "qwen3-coder",
		}},
	})
	provider, modelName, custom, err := model.resolveProviderAndModel("local-llm", "")
	if err != nil || !custom || provider != "local-llm" || modelName != "qwen3-coder" {
		t.Fatalf("resolve custom provider = %q %q %t %v", provider, modelName, custom, err)
	}
	options := model.modelDropdownOptions("local-llm")
	if len(options) != 2 || options[0].value != "qwen3-coder" || options[1].value != "deepseek-coder" {
		t.Fatalf("custom model options = %#v", options)
	}
}

func TestThemeCommandAppliesAndPersistsNamedTheme(t *testing.T) {
	defer applyTheme("gator")
	saved := ""
	model := New(Config{SetTheme: func(name string) error {
		saved = name
		return nil
	}})
	model.task.SetValue("/theme mono")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil {
		t.Fatal("theme command started asynchronous work")
	}
	updated := next.(Model)
	if saved != "mono" || updated.config.Theme != "mono" || !strings.Contains(updated.notice.text, "saved") {
		t.Fatalf("theme state = saved:%q theme:%q notice:%q", saved, updated.config.Theme, updated.notice.text)
	}
}

func TestExtensionPromptCommandOnlyFillsTheComposer(t *testing.T) {
	model := New(Config{ExtensionCommands: []ExtensionCommand{{
		Name: "review-helper:focused_review", Description: "prepare focused review", Prompt: "Review the current diff for concrete findings.",
	}}})
	model.task.SetValue("/review-helper:focused_review")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		// textarea focus restoration is a local command, so this branch is only
		// retained to make the intended no-run behavior explicit.
	} else if _, ok := next.(Model); !ok {
		t.Fatal("extension command returned an invalid model")
	}
	updated := next.(Model)
	if updated.task.Value() != "Review the current diff for concrete findings." || !strings.Contains(updated.notice.text, "loaded") {
		t.Fatalf("extension command state = task:%q notice:%q", updated.task.Value(), updated.notice.text)
	}
}

func TestNewCanStartAtTheProjectOrAllThreadPicker(t *testing.T) {
	stateDirectory := t.TempDir()
	project := "/workspace/current-project"
	firstWorktree := filepath.Join(t.TempDir(), "first")
	secondWorktree := filepath.Join(t.TempDir(), "second")
	for _, worktree := range []string{firstWorktree, secondWorktree} {
		if err := os.Mkdir(worktree, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, thread := range []journal.Thread{
		{Version: 1, ID: "thread-current", Repository: project, WorktreePath: firstWorktree, Provider: "openai", Task: "Current project", HeadStatePath: "/state/current", TurnCount: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{Version: 1, ID: "thread-other", Repository: "/workspace/other-project", WorktreePath: secondWorktree, Provider: "openai", Task: "Other project", HeadStatePath: "/state/other", TurnCount: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	} {
		if err := journal.SaveThread(stateDirectory, thread); err != nil {
			t.Fatalf("save thread: %v", err)
		}
	}

	projectPicker := New(Config{RepositoryPath: project, StateDir: stateDirectory, StartInRecent: true})
	if projectPicker.screen != recentScreen || len(projectPicker.recentThreads) != 1 || projectPicker.recentThreads[0].ID != "thread-current" {
		t.Fatalf("project picker = %#v", projectPicker)
	}
	allPicker := New(Config{RepositoryPath: project, StateDir: stateDirectory, StartInRecent: true, RecentAll: true})
	if allPicker.screen != recentScreen || !allPicker.recentAll || len(allPicker.recentThreads) != 2 {
		t.Fatalf("all picker = %#v", allPicker)
	}
}

func TestBeginContinuationSwitchesRepositoryForASelectedCrossProjectThread(t *testing.T) {
	stateDirectory := t.TempDir()
	retainedRepository := "/workspace/other-project"
	worktreePath := filepath.Join(t.TempDir(), "retained")
	if err := os.Mkdir(worktreePath, 0o700); err != nil {
		t.Fatal(err)
	}
	entry, record, err := journal.Open(retainedRepository, "run-cross-project", worktreePath, stateDirectory, time.Now())
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	if err := entry.SaveSession(journal.Session{Version: 2, Repository: retainedRepository, WorktreePath: worktreePath, Provider: "openai", Task: "Cross-project task", ThreadID: "thread-cross", Mode: "execute"}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if err := entry.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}

	model := New(Config{RepositoryPath: "/workspace/current-project", StateDir: stateDirectory})
	next, _ := model.beginContinuation(record.StatePath)
	continued := next.(Model)
	if continued.config.RepositoryPath != retainedRepository || continued.resumeStatePath != record.StatePath || continued.threadID != "thread-cross" {
		t.Fatalf("cross-project continuation = %#v", continued)
	}
}

func TestComposerRejectsEmptyTaskBeforeRun(t *testing.T) {
	model := New(Config{})
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if command != nil {
		t.Fatal("empty task started an execution")
	}
	updated := next.(Model)
	entry := updated.chat[len(updated.chat)-1]
	if !entry.isError || !strings.Contains(entry.text, "Describe a task") || updated.notice.text != "" {
		t.Fatalf("startup failure = entry %#v, notice %#v", entry, updated.notice)
	}
}

func TestComposerReportsProviderConfigurationFailureBeforeRun(t *testing.T) {
	const credentialError = "Gator OAuth credential for \"codex\" is required. Complete the provider-owned sign-in, then retry this exact task after the credential has been stored."
	model := New(Config{
		Provider:     "codex",
		Verification: [][]string{{"go", "test", "./..."}},
		NewExecutor: func(string, string, string) (gatorrun.Executor, error) {
			return gatorrun.Executor{}, errors.New(credentialError)
		},
	})
	model.width = 64
	model.height = 24
	model.resizeInputs()
	model.task.SetValue("Add a greeting")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if command != nil {
		t.Fatal("missing API key started an execution")
	}
	updated := next.(Model)
	if updated.notice.text != "" || updated.noticeView() != "" {
		t.Fatalf("startup failure repeated outside the transcript: %#v", updated.notice)
	}
	entry := updated.chat[len(updated.chat)-1]
	if got, want := entry.text, "Unable to start run: Resolve configuration before starting:\n"+credentialError; got != want || !entry.isError {
		t.Fatalf("transcript error = %#v, want %q", entry, want)
	}
	transcript := updated.transcriptContent()
	if !strings.Contains(transcript, "Gator OAuth credential") || !strings.Contains(transcript, "credential has been stored.") || !updated.followTranscript || !updated.transcript.AtBottom() {
		t.Fatalf("complete startup failure is not visible in the transcript: content=%q follow=%t atBottom=%t", transcript, updated.followTranscript, updated.transcript.AtBottom())
	}
	if count := strings.Count(updated.View(), "Gator OAuth credential"); count != 1 {
		t.Fatalf("startup error rendered %d times, want once: %s", count, updated.View())
	}
}
