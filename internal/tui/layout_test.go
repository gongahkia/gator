package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/journal"
)

func TestWindowSizeResizesInputsAndKeepsComposerWithinTerminal(t *testing.T) {
	model := New(Config{Verification: [][]string{{"go", "test", "./..."}}})
	model.task.SetValue("Implement responsive terminal layout")

	next, _ := model.Update(tea.WindowSizeMsg{Width: 44, Height: 18})
	small := next.(Model)
	if got, want := small.task.Height(), 2; got != want {
		t.Fatalf("small task height = %d, want %d", got, want)
	}
	if got, want := small.verification.Height(), 1; got != want {
		t.Fatalf("small verification height = %d, want %d", got, want)
	}
	if got, want := small.provider.Width, 40; got != want {
		t.Fatalf("small provider width = %d, want %d", got, want)
	}
	assertViewFits(t, small, 44, 18)

	next, _ = small.Update(tea.WindowSizeMsg{Width: 120, Height: 52})
	large := next.(Model)
	if got, want := large.task.Height(), 7; got != want {
		t.Fatalf("large task height = %d, want %d", got, want)
	}
	if got, want := large.verification.Height(), 3; got != want {
		t.Fatalf("large verification height = %d, want %d", got, want)
	}
	if got, want := large.provider.Width, 116; got != want {
		t.Fatalf("large provider width = %d, want %d", got, want)
	}
	assertViewFits(t, large, 120, 52)
}

func TestResponsiveViewsFitConstrainedTerminal(t *testing.T) {
	base := New(Config{Verification: [][]string{{"go", "test", "./..."}}})
	base.task.SetValue("Implement a responsive layout that remains useful when the terminal is narrow or short.")

	running := base
	running.screen = runningScreen
	running.events = []timelineEntry{
		{step: 1, text: "[01] agent: inspecting the repository"},
		{step: 2, text: "[02] tool git_status ok"},
		{step: 3, text: "[03] agent: updating the layout"},
	}

	review := base
	review.screen = reviewScreen
	review.diff = "diff --git a/internal/tui/app.go b/internal/tui/app.go\n+responsive terminal layout\n+bounded output"

	help := base
	help.screen = helpScreen

	recent := base
	recent.screen = recentScreen
	recent.recentIndex = 3
	recent.recentThreads = []journal.RecentThread{
		{Provider: "openai", Model: "gpt-5.6", Task: "First retained task", TurnCount: 1, UpdatedAt: time.Now(), Available: true},
		{Provider: "anthropic", Model: "claude-sonnet-5", Task: "Second retained task", TurnCount: 1, UpdatedAt: time.Now(), Available: true},
		{Provider: "gemini", Model: "gemini-3-pro", Task: "Third retained task", TurnCount: 1, UpdatedAt: time.Now(), Available: true},
		{Provider: "codex", Model: "", Task: "Selected retained task", TurnCount: 2, UpdatedAt: time.Now(), Available: true},
		{Provider: "mistral", Model: "mistral-large", Task: "Fifth retained task", TurnCount: 1, UpdatedAt: time.Now(), Available: false},
	}
	thread := base
	thread.screen = threadScreen
	thread.threadIndex = 2
	thread.threadTurns = []journal.ThreadTurn{
		{Provider: "openai", Model: "gpt-5.6", Mode: "execute", Task: "First retained task", Status: "completed", FinishedAt: time.Now(), FinalText: "First turn complete."},
		{Provider: "openai", Model: "gpt-5.6", Mode: "plan", Task: "Second retained task", Status: "completed", FinishedAt: time.Now(), FinalText: "Plan ready."},
		{Provider: "openai", Model: "gpt-5.6", Mode: "execute", Task: "Selected retained task", Status: "failed", FinishedAt: time.Now(), FinalText: "Verifier failed."},
	}

	for name, model := range map[string]Model{
		"compose": base,
		"running": running,
		"review":  review,
		"help":    help,
		"recent":  recent,
		"thread":  thread,
	} {
		t.Run(name, func(t *testing.T) {
			for _, size := range []struct{ width, height int }{{44, 18}, {18, 10}} {
				next, _ := model.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
				assertViewFits(t, next.(Model), size.width, size.height)
			}
		})
	}
}

func TestConstrainedProviderDropdownWindowsAroundSelection(t *testing.T) {
	model := New(Config{Provider: "openai"})
	model.focus = providerField
	options := model.dropdownOptions()
	model.dropdownIndex = len(options) - 1

	next, _ := model.Update(tea.WindowSizeMsg{Width: 44, Height: 18})
	updated := next.(Model)
	view := updated.View()
	selected := options[len(options)-1]
	if !strings.Contains(view, selected.label) {
		t.Fatalf("selected provider %q is not visible: %s", selected.label, view)
	}
	assertViewFits(t, updated, 44, 18)
}

func TestParseVerificationSplitsOneCommandPerLine(t *testing.T) {
	commands, err := parseVerification("go test ./...\n\nnpm test")
	if err != nil {
		t.Fatalf("parse verification: %v", err)
	}
	if got, want := formatVerification(commands), "go test ./...\nnpm test"; got != want {
		t.Fatalf("commands = %q, want %q", got, want)
	}
}

func TestReviewContinuationLoadsRetainedSession(t *testing.T) {
	model := New(Config{})
	model.screen = reviewScreen
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if command != nil {
		t.Fatal("missing run record returned a command")
	}
	updated := next.(Model)
	if updated.notice.kind != noticeError || !strings.Contains(updated.notice.text, "no resume record") {
		t.Fatalf("notice = %#v", updated.notice)
	}
}

func TestRenderEventDoesNotLeakMultilineOutputIntoTimeline(t *testing.T) {
	entry := renderEvent(agentEvent(3, "one\ntwo"))
	if entry.text != "[03] agent: one two" {
		t.Fatalf("timeline entry = %q", entry.text)
	}
}

func TestSlashPaletteOpensUnifiedModelCatalog(t *testing.T) {
	model := New(Config{LocalModels: &fakeLocalModelManager{}})
	model.task.SetValue("/mo")
	if commands := model.matchingCommands(); len(commands) != 1 || commands[0].name != "/model" {
		t.Fatalf("commands = %#v", commands)
	}
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("model command did not open the catalog")
	}
	updated := next.(Model)
	if updated.screen != localModelsScreen || updated.task.Value() != "" {
		t.Fatalf("composer = %#v", updated)
	}
}

func TestModelCatalogStartsCloudOAuthAndCompletesWithoutStartingRun(t *testing.T) {
	t.Setenv("GATOR_CODEX_OAUTH_CLIENT_ID", "gator-client")
	login := &fakeOAuthLogin{url: "https://auth.example.test/authorize"}
	model := New(Config{
		Provider:    "codex",
		LocalModels: &fakeLocalModelManager{},
		BeginOAuthLogin: func(provider string) (OAuthLogin, error) {
			if provider != "codex" {
				t.Fatalf("OAuth provider = %q", provider)
			}
			return login, nil
		},
	})
	model = selectCloudModelCatalog(t, model, "codex")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if command == nil {
		t.Fatal("login did not start asynchronous completion")
	}
	started := next.(Model)
	if started.execution != nil || started.oauthLogin == nil || !strings.Contains(started.commandOutput, login.url) {
		t.Fatalf("OAuth login state = %#v", started)
	}
	finished := runTeaCommand(t, started, command)
	if !login.completed || finished.oauthLogin != nil || !strings.Contains(finished.notice.text, "credential stored") || finished.execution != nil {
		t.Fatalf("OAuth completion state = %#v", finished)
	}
}

func TestCopilotModelDropdownUsesStoredAccountCatalog(t *testing.T) {
	stateDir := t.TempDir()
	store, err := auth.New(stateDir)
	if err != nil {
		t.Fatalf("new auth store: %v", err)
	}
	if err := store.Put("copilot", auth.Credential{Type: "oauth", Access: "access", Refresh: "refresh", Expires: time.Now().Add(time.Hour).UnixMilli(), Extra: map[string]string{"available_model_ids": "gpt-5, claude-sonnet-5"}}); err != nil {
		t.Fatalf("store Copilot catalog: %v", err)
	}
	model := New(Config{StateDir: stateDir, Provider: "copilot"})
	options := model.modelDropdownOptions("copilot")
	if len(options) != 3 || options[0].value != "claude-sonnet-5" || options[1].value != "gpt-5" || !options[2].custom {
		t.Fatalf("Copilot model options = %#v", options)
	}
}

type fakeOAuthLogin struct {
	url       string
	completed bool
}

func (l *fakeOAuthLogin) URL() string { return l.url }

func (l *fakeOAuthLogin) Complete(context.Context) error {
	l.completed = true
	return nil
}

func (l *fakeOAuthLogin) Cancel() {}

func selectCloudModelCatalog(t *testing.T, model Model, provider string) Model {
	t.Helper()
	next, _ := model.openModelCatalog()
	model = next.(Model)
	for index, entry := range model.cloudModels() {
		if entry.provider == provider {
			model.localModels.section = cloudModelSection
			model.localModels.cloudIndex = index
			return model
		}
	}
	t.Fatalf("cloud provider %q was not found in model catalog", provider)
	return Model{}
}
