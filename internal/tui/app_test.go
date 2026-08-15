package tui

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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
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

func TestComposerRejectsEmptyTaskBeforeRun(t *testing.T) {
	model := New(Config{})
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if command != nil {
		t.Fatal("empty task started an execution")
	}
	updated := next.(Model)
	if updated.notice.kind != noticeError || !strings.Contains(updated.notice.text, "Describe a task") {
		t.Fatalf("notice = %#v", updated.notice)
	}
}

func TestComposerReportsProviderConfigurationFailureBeforeRun(t *testing.T) {
	model := New(Config{
		Verification: [][]string{{"go", "test", "./..."}},
		NewExecutor: func(string, string, string) (gatorrun.Executor, error) {
			return gatorrun.Executor{}, errors.New("ANTHROPIC_API_KEY is required")
		},
	})
	model.task.SetValue("Add a greeting")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if command != nil {
		t.Fatal("missing API key started an execution")
	}
	updated := next.(Model)
	if updated.notice.kind != noticeError || !strings.Contains(updated.notice.text, "ANTHROPIC_API_KEY") {
		t.Fatalf("notice = %#v", updated.notice)
	}
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

func TestSlashPaletteFiltersAndFocusesModel(t *testing.T) {
	model := New(Config{})
	model.task.SetValue("/mo")
	if commands := model.matchingCommands(); len(commands) != 1 || commands[0].name != "/model" {
		t.Fatalf("commands = %#v", commands)
	}
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("model command did not focus the model field")
	}
	updated := next.(Model)
	if updated.focus != modelField || updated.task.Value() != "" {
		t.Fatalf("composer = %#v", updated)
	}
}

func TestQuestionMarkOpensCommandPalette(t *testing.T) {
	model := New(Config{})
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if command != nil {
		t.Fatal("question mark returned an unexpected command")
	}
	updated := next.(Model)
	if updated.task.Value() != "/" || !updated.commandPaletteVisible() {
		t.Fatalf("palette was not opened: task=%q", updated.task.Value())
	}
}

func TestTabAcceptsSlashCommandRecommendation(t *testing.T) {
	model := New(Config{})
	model.task.SetValue("/prov")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	if command == nil {
		t.Fatal("provider command did not focus the provider field")
	}
	updated := next.(Model)
	if updated.focus != providerField || updated.task.Value() != "" {
		t.Fatalf("composer = %#v", updated)
	}
}

func TestProviderDropdownSelectsProviderAndRecommendedModel(t *testing.T) {
	model := New(Config{Provider: "openai", Model: "gpt-5.6"})
	model.focus = providerField
	model.normalizeDropdownSelection()
	for index, option := range model.dropdownOptions() {
		if option.value == "anthropic" {
			model.dropdownIndex = index
			break
		}
	}
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	if command == nil {
		t.Fatal("provider dropdown did not focus the model field")
	}
	updated := next.(Model)
	if updated.focus != modelField || updated.provider.Value() != "anthropic" || updated.model.Value() != "claude-sonnet-5" {
		t.Fatalf("composer = provider %q, model %q, focus %v", updated.provider.Value(), updated.model.Value(), updated.focus)
	}
	updated.width = 100
	updated.height = 40
	updated.resizeInputs()
	if !strings.Contains(updated.View(), "Recommended models") {
		t.Fatalf("model dropdown missing from view: %s", updated.View())
	}
}

func TestModelDropdownKeepsCustomModelEntryForEndpointSpecificProviders(t *testing.T) {
	model := New(Config{Provider: "azure-openai"})
	model.focus = modelField
	options := model.dropdownOptions()
	if len(options) != 1 || !options[0].custom || options[0].label != "custom model ID" {
		t.Fatalf("Azure model options = %#v", options)
	}
}

func TestCustomModelDropdownDoesNotClearTypedModel(t *testing.T) {
	model := New(Config{Provider: "azure-openai", Model: "my-deployment"})
	model.focus = modelField
	model.normalizeDropdownSelection()
	model.applySelectedDropdown()
	if got := model.model.Value(); got != "my-deployment" {
		t.Fatalf("custom model = %q", got)
	}
}

func TestTaskPathAutocompleteInsertsSelectedReferenceWithTab(t *testing.T) {
	repository := testRepository(t)
	model := New(Config{RepositoryPath: repository})
	model.task.SetValue("Inspect @hel")
	model.normalizeContextSelection()
	if matches := model.contextCompletions(); len(matches) != 1 || matches[0] != "hello.go" {
		t.Fatalf("path matches = %#v", matches)
	}
	model.width = 100
	model.height = 40
	model.resizeInputs()
	if !strings.Contains(model.View(), "Path suggestions") {
		t.Fatalf("path dropdown missing from view: %s", model.View())
	}
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	if command != nil {
		t.Fatal("path completion returned an unexpected command")
	}
	updated := next.(Model)
	if updated.task.Value() != "Inspect @hello.go " {
		t.Fatalf("task = %q", updated.task.Value())
	}
}

func TestComposerAllowsQInTaskText(t *testing.T) {
	model := New(Config{})
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	updated := next.(Model)
	if updated.task.Value() != "q" {
		t.Fatalf("task = %q, want q", updated.task.Value())
	}
}

func TestContextReferencesIgnoreEmailAndSupportQuotedPaths(t *testing.T) {
	got := extractContextReferences("Contact dev@example.test; inspect @cmd/gator/main.go, and @\"notes with spaces.md\".")
	if want := []string{"cmd/gator/main.go", "notes with spaces.md"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("references = %#v, want %#v", got, want)
	}
}

func TestContextReferencesAreValidatedAndAddedWithoutSourceContents(t *testing.T) {
	repository := testRepository(t)
	if err := os.WriteFile(filepath.Join(repository, "secret.txt"), []byte("private source content"), 0o644); err != nil {
		t.Fatal(err)
	}
	references, err := resolveContextReferences("Inspect @hello.go and @missing.go", repository)
	if err == nil || !strings.Contains(err.Error(), "@missing.go") {
		t.Fatalf("missing context reference error = %v", err)
	}
	references, err = resolveContextReferences("Inspect @hello.go", repository)
	if err != nil {
		t.Fatalf("resolve context reference: %v", err)
	}
	task := taskWithContextReferences("Inspect @hello.go", references)
	if !strings.Contains(task, "hello.go (file)") || strings.Contains(task, "private source content") {
		t.Fatalf("contextualized task = %q", task)
	}
}

func TestStartRunRejectsEscapingContextReference(t *testing.T) {
	repository := testRepository(t)
	model := New(Config{
		RepositoryPath: repository,
		Verification:   [][]string{{"go", "test", "./..."}},
		NewExecutor: func(string, string, string) (gatorrun.Executor, error) {
			return gatorrun.Executor{Model: &testAgentModel{}}, nil
		},
	})
	model.task.SetValue("Inspect @../outside.go")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if command != nil {
		t.Fatal("escaping context reference started a run")
	}
	updated := next.(Model)
	if updated.notice.kind != noticeError || !strings.Contains(updated.notice.text, "escapes the workspace") {
		t.Fatalf("notice = %#v", updated.notice)
	}
}

func TestInteractiveRunStreamsToReview(t *testing.T) {
	repository := testRepository(t)
	agentModel := &testAgentModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "test", Name: "run_command", Arguments: json.RawMessage(`{"argv":["go","test","./..."]}`)}}},
		{Text: "The repository is verified and ready for review."},
	}}
	model := New(Config{
		RepositoryPath: repository,
		Model:          "test-model",
		Verification:   [][]string{{"go", "test", "./..."}},
		StateDir:       t.TempDir(),
		NewExecutor: func(string, string, string) (gatorrun.Executor, error) {
			return gatorrun.Executor{Model: agentModel, Now: func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) }}, nil
		},
	})
	model.width = 100
	model.height = 40
	model.resizeInputs()
	model.task.SetValue("Inspect @hello.go and verify this repository")

	updated := drive(t, model, tea.KeyMsg{Type: tea.KeyCtrlR})
	if updated.screen != reviewScreen {
		t.Fatalf("screen = %v, want review", updated.screen)
	}
	if updated.runErr != nil {
		t.Fatalf("run error = %v", updated.runErr)
	}
	if updated.outcome == nil || updated.outcome.Worktree.Path == "" {
		t.Fatalf("outcome = %#v", updated.outcome)
	}
	if len(updated.events) < 5 {
		t.Fatalf("timeline events = %#v", updated.events)
	}
	if len(agentModel.requests) == 0 || !strings.Contains(agentModel.requests[0].Messages[0].Content, "hello.go (file)") {
		t.Fatalf("agent did not receive validated context references: %#v", agentModel.requests)
	}
	if !strings.Contains(updated.View(), "The repository is verified") {
		t.Fatalf("review omitted final result: %s", updated.View())
	}
}

func agentEvent(step int, text string) agent.Event {
	return agent.Event{Kind: agent.EventText, Step: step, Text: text}
}

func drive(t *testing.T, model Model, message tea.Msg) Model {
	t.Helper()
	current, command := model.Update(message)
	updated := current.(Model)
	for command != nil {
		current, command = updated.Update(command())
		updated = current.(Model)
	}
	return updated
}

type testAgentModel struct {
	turns    []agent.Turn
	requests []agent.TurnRequest
}

func (m *testAgentModel) Complete(_ context.Context, request agent.TurnRequest) (agent.Turn, error) {
	m.requests = append(m.requests, request)
	if len(m.turns) == 0 {
		return agent.Turn{}, errors.New("unexpected model call")
	}
	turn := m.turns[0]
	m.turns = m.turns[1:]
	return turn, nil
}

func testRepository(t *testing.T) string {
	t.Helper()
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repository, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/tui-test\n\ngo 1.25.0\n")
	write("hello.go", "package tuitest\n\nfunc Hello() string { return \"hello\" }\n")
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "add", ".")
	runGit(t, repository, "-c", "user.name=Gator Test", "-c", "user.email=gator@example.invalid", "commit", "--quiet", "-m", "fixture")
	return repository
}

func runGit(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
}
