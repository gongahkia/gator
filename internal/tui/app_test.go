package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gongahkia/gator/internal/agent"
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
	if got, want := small.provider.Width, 36; got != want {
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
	if got, want := large.provider.Width, 112; got != want {
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

	for name, model := range map[string]Model{
		"compose": base,
		"running": running,
		"review":  review,
		"help":    help,
		"recent":  recent,
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

func TestTabCompletesSlashCommandWithoutExecutingIt(t *testing.T) {
	model := New(Config{})
	model.task.SetValue("/prov")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	if command != nil {
		t.Fatal("Tab executed the provider command")
	}
	updated := next.(Model)
	if updated.focus != taskField || updated.task.Value() != "/provider" {
		t.Fatalf("Tab completion = %#v", updated)
	}
	next, command = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("Enter did not execute the completed provider command")
	}
	updated = next.(Model)
	if updated.focus != providerField || updated.task.Value() != "" {
		t.Fatalf("executed command = %#v", updated)
	}
}

func TestEnterSendsMessageOutsideVimMode(t *testing.T) {
	model := New(Config{})
	model.task.SetValue("Explain this repository.")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil {
		t.Fatal("missing executor started an asynchronous command")
	}
	updated := next.(Model)
	if updated.vim != vimOff || updated.task.Value() != "Explain this repository." || !strings.Contains(updated.notice.text, "No model provider") {
		t.Fatalf("Enter did not attempt to send the message: %#v", updated.notice)
	}
}

func TestVimModeKeepsEnterForNewlinesAndHandlesNormalCommands(t *testing.T) {
	model := New(Config{})
	model.task.SetValue("/vim")
	updated := drive(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if updated.vim != vimNormal || updated.task.Value() != "" {
		t.Fatalf("Vim mode = %v, task = %q", updated.vim, updated.task.Value())
	}

	updated.task.SetValue("one")
	updated = drive(t, updated, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("0")})
	updated = drive(t, updated, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if updated.task.Value() != "ne" {
		t.Fatalf("Vim x command = %q, want %q", updated.task.Value(), "ne")
	}
	updated = drive(t, updated, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if updated.vim != vimInsert {
		t.Fatalf("Vim i command mode = %v, want insert", updated.vim)
	}
	updated = drive(t, updated, tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(updated.task.Value(), "\n") || updated.screen != composeScreen {
		t.Fatalf("Vim Insert Enter = task %q, screen %v", updated.task.Value(), updated.screen)
	}
	updated = drive(t, updated, tea.KeyMsg{Type: tea.KeyEsc})
	if updated.vim != vimNormal {
		t.Fatalf("Vim Escape mode = %v, want normal", updated.vim)
	}
}

func TestVimWriteCommandsSubmitAndQuitOnlyAfterSuccessfulWork(t *testing.T) {
	model := New(Config{})
	model.vim = vimNormal
	model.task.SetValue("Explain this repository.")
	model = drive(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	model = drive(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if model.vimCommand != ":w" {
		t.Fatalf("Vim command = %q", model.vimCommand)
	}
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil {
		t.Fatal(":w unexpectedly started an asynchronous command without an executor")
	}
	updated := next.(Model)
	if updated.vimCommand != "" || updated.task.Value() != "Explain this repository." || !strings.Contains(updated.notice.text, "No model provider") {
		t.Fatalf(":w state = command %q, task %q, notice %#v", updated.vimCommand, updated.task.Value(), updated.notice)
	}

	updated.screen = runningScreen
	updated.execution = &executionStream{steering: make(chan string, 1), steeringSupported: true}
	updated = drive(t, updated, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	updated = drive(t, updated, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	updated = drive(t, updated, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	updated = drive(t, updated, tea.KeyMsg{Type: tea.KeyEnter})
	if !updated.quitAfterRun || updated.task.Value() != "" {
		t.Fatalf(":wq state = quitAfterRun %v, task %q", updated.quitAfterRun, updated.task.Value())
	}
	select {
	case instruction := <-updated.execution.steering:
		if instruction != "Explain this repository." {
			t.Fatalf(":wq steering = %q", instruction)
		}
	default:
		t.Fatal(":wq did not submit the running instruction")
	}

	next, command = updated.Update(executionDoneMsg{done: executionDone{}})
	if command == nil {
		t.Fatal(":wq did not request exit after successful completion")
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf(":wq command = %T, want tea.QuitMsg", command())
	}
	if next.(Model).quitAfterRun {
		t.Fatal(":wq exit state was not cleared")
	}
}

func TestRunningTabQueuesPromptWithoutSteeringTheActiveRun(t *testing.T) {
	model := New(Config{})
	model.screen = runningScreen
	model.execution = &executionStream{steering: make(chan string, 1), steeringSupported: true}
	model.task.SetValue("After this, add focused tests.")

	next, command := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	if command != nil {
		t.Fatal("Tab unexpectedly started a command")
	}
	updated := next.(Model)
	if updated.screen != runningScreen || updated.task.Value() != "" || len(updated.queue) != 1 || updated.queue[0].kind != queuedPrompt || updated.queue[0].text != "After this, add focused tests." {
		t.Fatalf("queued running state = %#v", updated)
	}
	select {
	case instruction := <-updated.execution.steering:
		t.Fatalf("Tab steered active run with %q", instruction)
	default:
	}
}

func TestRunningTabCompletesPartialSlashCommandBeforeQueueingIt(t *testing.T) {
	model := New(Config{})
	model.screen = runningScreen
	model.execution = &executionStream{steering: make(chan string, 1), steeringSupported: true}
	model.task.SetValue("/pla")

	next, command := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	if command != nil {
		t.Fatal("Tab unexpectedly started a command")
	}
	completed := next.(Model)
	if completed.task.Value() != "/plan" || len(completed.queue) != 0 {
		t.Fatalf("partial command Tab = task %q, queue %#v", completed.task.Value(), completed.queue)
	}
	next, command = completed.Update(tea.KeyMsg{Type: tea.KeyTab})
	if command != nil {
		t.Fatal("second Tab unexpectedly started a command")
	}
	queued := next.(Model)
	if queued.task.Value() != "" || len(queued.queue) != 1 || queued.queue[0].kind != queuedCommand || queued.queue[0].text != "/plan" {
		t.Fatalf("exact command Tab = %#v", queued)
	}
}

func TestRunningEnterSteersNativeProviderAtBoundary(t *testing.T) {
	model := New(Config{})
	model.screen = runningScreen
	model.execution = &executionStream{steering: make(chan string, 1), steeringSupported: true}
	model.task.SetValue("Do not change the public API.")

	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil {
		t.Fatal("Enter unexpectedly started a command")
	}
	updated := next.(Model)
	select {
	case instruction := <-updated.execution.steering:
		if instruction != "Do not change the public API." {
			t.Fatalf("steering instruction = %q", instruction)
		}
	default:
		t.Fatal("Enter did not send a steering instruction")
	}
	if updated.task.Value() != "" || !strings.Contains(updated.notice.text, "next model or tool boundary") {
		t.Fatalf("steering state = %#v", updated.notice)
	}
}

func TestRunningEnterDoesNotPretendToSteerDelegatedCLI(t *testing.T) {
	model := New(Config{})
	model.screen = runningScreen
	model.execution = &executionStream{steering: make(chan string, 1), steeringSupported: false}
	model.task.SetValue("Use a smaller diff.")

	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil {
		t.Fatal("Enter unexpectedly started a command")
	}
	updated := next.(Model)
	if updated.task.Value() != "Use a smaller diff." || !strings.Contains(updated.notice.text, "Press Tab to queue") {
		t.Fatalf("delegated steering state = %#v", updated.notice)
	}
	select {
	case instruction := <-updated.execution.steering:
		t.Fatalf("delegated CLI received steering %q", instruction)
	default:
	}
}

func TestRunningViewShowsEventBackedActivityVerificationAndQueue(t *testing.T) {
	model := New(Config{Verification: [][]string{{"go", "test", "./..."}}})
	model.width = 120
	model.height = 60
	model.resizeInputs()
	model.screen = runningScreen
	model.execution = &executionStream{steering: make(chan string, 1), steeringSupported: true}
	model.queue = []queuedInput{{kind: queuedPrompt, text: "Add a regression test after this."}}
	model.beginRunActivity([][]string{{"go", "test", "./..."}})

	now := time.Now()
	model.appendEvent(agent.Event{Kind: agent.EventTurnStarted, At: now, Step: 3})
	model.appendEvent(agent.Event{Kind: agent.EventToolCalled, At: now, Step: 3, ToolCall: &agent.ToolCall{Name: "git_status", Arguments: json.RawMessage(`{}`)}})
	if model.activity.phase != activityInspecting {
		t.Fatalf("activity phase = %v, want inspecting", model.activity.phase)
	}
	view := model.View()
	for _, expected := range []string{"Activity", "inspecting the isolated worktree", "turn 3/24", "last activity", "Queued next · 1 prompt queued", "Add a regression test after this.", "Verification", "go test ./... · pending"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("running view omitted %q:\n%s", expected, view)
		}
	}

	verifier := &agent.ToolCall{Name: "run_command", Arguments: json.RawMessage(`{"argv":["go","test","./..."]}`)}
	model.appendEvent(agent.Event{Kind: agent.EventToolCalled, At: now, Step: 3, ToolCall: verifier})
	if model.activity.phase != activityVerifying || model.verificationStatus[0].phase != verificationRunning {
		t.Fatalf("verifier start = activity %#v, verifier %#v", model.activity, model.verificationStatus)
	}
	model.appendEvent(agent.Event{Kind: agent.EventToolFinished, At: now.Add(time.Second), Step: 3, ToolCall: verifier})
	if model.verificationStatus[0].phase != verificationPassed || model.verificationSummary() != "verification passed" {
		t.Fatalf("verifier finish = %#v", model.verificationStatus)
	}
	next, command := model.Update(activityTickMsg{})
	if next.(Model).screen != runningScreen || command == nil {
		t.Fatal("activity tick did not keep the live status refresh scheduled")
	}
}

func TestLatestRunViewShowsDiffFactsAndFailureRecovery(t *testing.T) {
	model := New(Config{})
	model.width = 120
	model.height = 60
	model.resizeInputs()
	model.outcome = &gatorrun.Outcome{}
	model.verificationStatus = []verificationStatus{{argv: []string{"go", "test", "./..."}, phase: verificationPassed}}
	model.diffStats = summarizeDiff("diff --git a/a.go b/a.go\n+one\n-two\ndiff --git a/b.go b/b.go\n+three\n")
	view := model.View()
	for _, expected := range []string{"Latest run", "verification passed", "2 file(s) · +2 −1"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("completed view omitted %q:\n%s", expected, view)
		}
	}

	model.runErr = errors.New("required verification failed: go test ./...")
	view = model.View()
	for _, expected := range []string{"Stopped: required verification failed", "Verification did not complete successfully"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("failure view omitted %q:\n%s", expected, view)
		}
	}

	model.lastRunCancelled = true
	if guidance := model.failureGuidance(); !strings.Contains(guidance, "Cancelled by you") {
		t.Fatalf("cancellation guidance = %q", guidance)
	}
}

func TestRunStatusViewsFitConstrainedTerminals(t *testing.T) {
	model := New(Config{Verification: [][]string{{"go", "test", "./..."}, {"go", "vet", "./..."}}})
	model.screen = runningScreen
	model.execution = &executionStream{steering: make(chan string, 1), steeringSupported: false}
	model.queue = []queuedInput{{kind: queuedPrompt, text: "Check the edge cases after the delegated run."}}
	model.beginRunActivity([][]string{{"go", "test", "./..."}, {"go", "vet", "./..."}})
	model.appendEvent(agent.Event{Kind: agent.EventHarnessStarted, Step: 1, Text: "codex"})

	for _, size := range []struct{ width, height int }{{44, 18}, {18, 10}} {
		next, _ := model.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
		assertViewFits(t, next.(Model), size.width, size.height)
	}
}

func TestQueueRejectsDirectShellCommandsAndHoldsAfterFailure(t *testing.T) {
	model := New(Config{})
	model.screen = runningScreen
	model.execution = &executionStream{steering: make(chan string, 1), steeringSupported: true}
	model.task.SetValue("!rm -rf build")
	updated := drive(t, model, tea.KeyMsg{Type: tea.KeyTab})
	if len(updated.queue) != 0 || !strings.Contains(strings.ToLower(updated.notice.text), "direct shell commands") {
		t.Fatalf("shell queue state = %#v", updated.notice)
	}
	updated.queue = []queuedInput{{kind: queuedPrompt, text: "Retry with a smaller change."}}
	next, command := updated.Update(executionDoneMsg{done: executionDone{err: errors.New("provider unavailable")}})
	if command != nil {
		t.Fatal("failed run unexpectedly dispatched queued work")
	}
	paused := next.(Model)
	if paused.screen != composeScreen || len(paused.queue) != 1 || !strings.Contains(paused.notice.text, "retained") {
		t.Fatalf("failed queue state = %#v", paused)
	}
}

func TestRunningQueueCommandsInspectAndRemoveQueuedWork(t *testing.T) {
	model := New(Config{})
	model.screen = runningScreen
	model.queue = []queuedInput{{kind: queuedPrompt, text: "Review the generated diff."}}
	model.task.SetValue("/queue")

	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil {
		t.Fatal("/queue unexpectedly started a command")
	}
	updated := next.(Model)
	if len(updated.queue) != 1 || !strings.Contains(updated.commandOutput, "Review the generated diff") {
		t.Fatalf("/queue state = queue %#v, output %q", updated.queue, updated.commandOutput)
	}
	updated.task.SetValue("/dequeue")
	next, command = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil {
		t.Fatal("/dequeue unexpectedly started a command")
	}
	updated = next.(Model)
	if len(updated.queue) != 0 || !strings.Contains(updated.notice.text, "Removed next") {
		t.Fatalf("/dequeue state = queue %#v, notice %#v", updated.queue, updated.notice)
	}
}

func TestCompletedRunAppliesQueuedCommandAndQueuedPromptStartsRun(t *testing.T) {
	model := New(Config{})
	model.screen = runningScreen
	model.queue = []queuedInput{{kind: queuedCommand, text: "/plan"}}
	next, command := model.Update(executionDoneMsg{done: executionDone{}})
	if command != nil {
		t.Fatal("queued local command unexpectedly returned a command")
	}
	updated := next.(Model)
	if updated.runMode != gatorrun.PlanMode || len(updated.queue) != 0 || !strings.Contains(updated.notice.text, "read-only") {
		t.Fatalf("queued command dispatch = %#v", updated)
	}

	repository := testRepository(t)
	agentModel := &testAgentModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{Text: "Queued plan complete."},
	}}
	queued := New(Config{
		RepositoryPath: repository,
		Model:          "test-model",
		StateDir:       t.TempDir(),
		NewExecutor: func(string, string, string) (gatorrun.Executor, error) {
			return gatorrun.Executor{Model: agentModel}, nil
		},
	})
	queued.runMode = gatorrun.PlanMode
	queued.queue = []queuedInput{{kind: queuedPrompt, text: "Plan the queued work."}}
	next, command, dispatched := queued.dispatchNextQueued()
	if !dispatched || command == nil || next.(Model).screen != runningScreen {
		t.Fatalf("queued prompt dispatch = dispatched %v, command %v, model %#v", dispatched, command != nil, next)
	}
	finished := runTeaCommand(t, next.(Model), command)
	if finished.runErr != nil || finished.screen != composeScreen || len(finished.queue) != 0 || len(agentModel.requests) != 2 {
		t.Fatalf("queued prompt finished = error %v, screen %v, queue %#v, requests %d", finished.runErr, finished.screen, finished.queue, len(agentModel.requests))
	}
}

func TestPlanCommandStartsWithoutVerifierAndUsesReadOnlyTools(t *testing.T) {
	repository := testRepository(t)
	agentModel := &testAgentModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{Text: "Plan the focused implementation and test changes."},
	}}
	model := New(Config{
		RepositoryPath: repository,
		Model:          "test-model",
		StateDir:       t.TempDir(),
		NewExecutor: func(string, string, string) (gatorrun.Executor, error) {
			return gatorrun.Executor{Model: agentModel}, nil
		},
	})
	model.width = 100
	model.height = 40
	model.resizeInputs()
	model.task.SetValue("/plan")
	updated := drive(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if updated.runMode != gatorrun.PlanMode || !strings.Contains(updated.notice.text, "read-only") {
		t.Fatalf("plan command state = %#v", updated)
	}
	updated.task.SetValue("/execute")
	updated = drive(t, updated, tea.KeyMsg{Type: tea.KeyEnter})
	if updated.runMode != gatorrun.ExecuteMode {
		t.Fatalf("execute command mode = %v", updated.runMode)
	}
	updated.task.SetValue("/plan")
	updated = drive(t, updated, tea.KeyMsg{Type: tea.KeyEnter})
	updated.task.SetValue("Plan the feature before implementation")
	updated = drive(t, updated, tea.KeyMsg{Type: tea.KeyCtrlR})
	if updated.screen != composeScreen || updated.runErr != nil || updated.outcome == nil {
		t.Fatalf("plan run = screen %v, error %v, outcome %#v", updated.screen, updated.runErr, updated.outcome)
	}
	if !strings.Contains(updated.View(), "Plan the focused implementation") || updated.outcome.ThreadID == "" {
		t.Fatalf("plan conversation = %s", updated.View())
	}
	for _, tool := range agentModel.requests[0].Tools {
		if tool.Name == "apply_patch" || tool.Name == "run_command" {
			t.Fatalf("plan tool surface included %q", tool.Name)
		}
	}
}

func TestSessionStatusHandlesValidVerifierConfiguration(t *testing.T) {
	model := New(Config{Verification: [][]string{{"go", "test", "./..."}}})
	if status := model.sessionStatus(); !strings.Contains(status, "go test ./...") || !strings.Contains(status, "mode: execute") {
		t.Fatalf("session status = %q", status)
	}
}

func TestReviewShowsExplicitPatchHandoffCommands(t *testing.T) {
	model := New(Config{})
	model.screen = reviewScreen
	model.outcome = &gatorrun.Outcome{StatePath: "/state/run-001"}
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if command != nil {
		t.Fatal("patch handoff returned an unexpected command")
	}
	updated := next.(Model)
	if !strings.Contains(updated.commandOutput, "gator export /state/run-001") || !strings.Contains(updated.commandOutput, "gator apply --check /state/run-001") {
		t.Fatalf("patch handoff = %q", updated.commandOutput)
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
	if !strings.Contains(model.View(), "[go]") {
		t.Fatalf("path type badge missing from view: %s", model.View())
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

func TestCtrlSpaceReopensDismissedPathSuggestions(t *testing.T) {
	repository := testRepository(t)
	model := New(Config{RepositoryPath: repository, StateDir: t.TempDir()})
	model.task.SetValue("Inspect @hel")
	model.normalizeContextSelection()
	model.contextClosed = true
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	if command != nil {
		t.Fatal("Ctrl+Space returned an unexpected command")
	}
	updated := next.(Model)
	if updated.contextClosed || !updated.contextCompletionVisible() {
		t.Fatalf("path completion was not reopened: %#v", updated)
	}
}

func TestDraftRestoresComposerState(t *testing.T) {
	repository := testRepository(t)
	stateDirectory := t.TempDir()
	first := New(Config{RepositoryPath: repository, StateDir: stateDirectory, Provider: "anthropic", Model: "claude-sonnet-5"})
	first.task.SetValue("Add draft recovery")
	first.verification.SetValue("go test ./...\ngo vet ./...")
	first.persistDraft()
	if first.draftErr != nil {
		t.Fatalf("persist draft: %v", first.draftErr)
	}
	restored := New(Config{RepositoryPath: repository, StateDir: stateDirectory})
	if restored.task.Value() != "Add draft recovery" || restored.verification.Value() != "go test ./...\ngo vet ./..." || restored.provider.Value() != "anthropic" || restored.model.Value() != "claude-sonnet-5" {
		t.Fatalf("restored composer = %#v", restored)
	}
}

func TestPreflightShowsFactoryConfigurationErrorBeforeRun(t *testing.T) {
	model := New(Config{
		RepositoryPath: "/tmp/example-repository",
		StateDir:       t.TempDir(),
		Verification:   [][]string{{"go", "test", "./..."}},
		NewExecutor: func(string, string, string) (gatorrun.Executor, error) {
			return gatorrun.Executor{}, errors.New("codex CLI is not installed or not on PATH")
		},
	})
	model.task.SetValue("Add a focused feature")
	model.refreshPreflight()
	if len(model.preflight) != 1 || !strings.Contains(model.preflight[0], "CLI is not installed") {
		t.Fatalf("preflight = %#v", model.preflight)
	}
	model.width = 100
	model.height = 40
	if !strings.Contains(model.preflightView(), "CLI is not installed") {
		t.Fatalf("preflight view = %s", model.preflightView())
	}
}

func TestF1HelpReturnsToPreviousScreen(t *testing.T) {
	model := New(Config{StateDir: t.TempDir()})
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyF1})
	if command != nil {
		t.Fatal("F1 returned an unexpected command")
	}
	updated := next.(Model)
	if updated.screen != helpScreen || !strings.Contains(updated.helpView(), "Ctrl+O") {
		t.Fatalf("help screen was not rendered")
	}
	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if next.(Model).screen != composeScreen {
		t.Fatalf("F1 help did not return to composer: %#v", next)
	}
}

func TestRecentRunsPickerLoadsASelectedContinuation(t *testing.T) {
	repository := testRepository(t)
	stateDirectory := t.TempDir()
	worktreePath := t.TempDir()
	runJournal, record, err := journal.Open(repository, "run-recent-001", worktreePath, stateDirectory, time.Now())
	if err != nil {
		t.Fatalf("open run journal: %v", err)
	}
	if err := runJournal.SaveSession(journal.Session{Version: 2, Repository: repository, WorktreePath: worktreePath, Provider: "openai", Model: "gpt-5.6", Task: "Retained task", Verification: [][]string{{"go", "test", "./..."}}}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if err := runJournal.Close(); err != nil {
		t.Fatalf("close journal: %v", err)
	}
	if record.StatePath == "" {
		t.Fatal("run record path is empty")
	}
	model := New(Config{
		RepositoryPath: repository,
		StateDir:       stateDirectory,
		NewExecutor: func(string, string, string) (gatorrun.Executor, error) {
			return gatorrun.Executor{}, errors.New("OPENAI_API_KEY is required")
		},
	})
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	if command != nil {
		t.Fatal("recent-run picker returned an unexpected command")
	}
	picker := next.(Model)
	pickerView := picker.recentRunsView()
	if picker.screen != recentScreen || !strings.Contains(pickerView, "Retained") || strings.Contains(pickerView, record.StatePath) {
		t.Fatalf("recent picker = %s", pickerView)
	}
	next, command = picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("recent run did not focus the continuation task")
	}
	continued := next.(Model)
	if continued.screen != composeScreen || continued.resumeStatePath != record.StatePath || continued.provider.Value() != "openai" || continued.model.Value() != "gpt-5.6" {
		t.Fatalf("continuation = %#v", continued)
	}
	continued.task.SetValue("Continue the retained patch")
	continued.refreshPreflight()
	if len(continued.preflight) != 1 || !strings.Contains(continued.preflight[0], "OPENAI_API_KEY") || !strings.Contains(continued.preflightView(), "Before continuing") {
		t.Fatalf("continuation preflight = %#v", continued.preflight)
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

func TestImageReferenceAttachesPixelsToNativeRun(t *testing.T) {
	repository := testRepository(t)
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00}
	if err := os.WriteFile(filepath.Join(repository, "screen.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	agentModel := &testAgentModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{Text: "Plan ready."},
	}}
	model := New(Config{
		RepositoryPath: repository,
		Model:          "test-model",
		StateDir:       t.TempDir(),
		NewExecutor: func(string, string, string) (gatorrun.Executor, error) {
			return gatorrun.Executor{Model: agentModel}, nil
		},
	})
	model.runMode = gatorrun.PlanMode
	model.task.SetValue("Review @screen.png")
	pending := drive(t, model, tea.KeyMsg{Type: tea.KeyCtrlR})
	if pending.screen != attachmentConfirmScreen || len(agentModel.requests) != 0 {
		t.Fatalf("image attachment confirmation = screen %v, requests %#v", pending.screen, agentModel.requests)
	}
	updated := drive(t, pending, tea.KeyMsg{Type: tea.KeyEnter})
	if updated.runErr != nil || len(agentModel.requests) == 0 {
		t.Fatalf("image run error = %v, requests = %#v", updated.runErr, agentModel.requests)
	}
	message := agentModel.requests[0].Messages[0]
	if len(message.Images) != 1 || message.Images[0].Name != "screen.png" || string(message.Images[0].Data) != string(png) || !strings.Contains(message.Content, "image attachment") {
		t.Fatalf("image attachment message = %#v", message)
	}
}

func TestPDFReferenceAttachesDocumentToNativeRun(t *testing.T) {
	repository := testRepository(t)
	pdf := []byte("%PDF-1.7\ncontent")
	if err := os.WriteFile(filepath.Join(repository, "report.pdf"), pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	agentModel := &testAgentModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{Text: "Plan ready."},
	}}
	model := New(Config{
		RepositoryPath: repository,
		Model:          "test-model",
		StateDir:       t.TempDir(),
		NewExecutor: func(string, string, string) (gatorrun.Executor, error) {
			return gatorrun.Executor{Model: agentModel}, nil
		},
	})
	model.runMode = gatorrun.PlanMode
	model.task.SetValue("Review @report.pdf")
	pending := drive(t, model, tea.KeyMsg{Type: tea.KeyCtrlR})
	if pending.screen != attachmentConfirmScreen || len(agentModel.requests) != 0 {
		t.Fatalf("PDF attachment confirmation = screen %v, requests %#v", pending.screen, agentModel.requests)
	}
	updated := drive(t, pending, tea.KeyMsg{Type: tea.KeyEnter})
	if updated.runErr != nil || len(agentModel.requests) == 0 {
		t.Fatalf("PDF run error = %v, requests = %#v", updated.runErr, agentModel.requests)
	}
	message := agentModel.requests[0].Messages[0]
	if len(message.Attachments) != 1 || message.Attachments[0].Name != "report.pdf" || message.Attachments[0].MediaType != "application/pdf" || string(message.Attachments[0].Data) != string(pdf) || !strings.Contains(message.Content, "document attachment") {
		t.Fatalf("PDF attachment message = %#v", message)
	}
}

func TestAttachmentConfirmationCanCancelWithoutSendingBytes(t *testing.T) {
	repository := testRepository(t)
	if err := os.WriteFile(filepath.Join(repository, "screen.png"), []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, 0o644); err != nil {
		t.Fatal(err)
	}
	agentModel := &testAgentModel{}
	model := New(Config{
		RepositoryPath: repository,
		Model:          "test-model",
		NewExecutor: func(string, string, string) (gatorrun.Executor, error) {
			return gatorrun.Executor{Model: agentModel}, nil
		},
	})
	model.width, model.height = 120, 48
	model.runMode = gatorrun.PlanMode
	model.task.SetValue("Review @screen.png")
	pending := drive(t, model, tea.KeyMsg{Type: tea.KeyCtrlR})
	if !strings.Contains(pending.View(), "Provider boundary") || !strings.Contains(pending.View(), "PDF byte limits") {
		t.Fatalf("attachment confirmation view = %s", pending.View())
	}
	cancelled := drive(t, pending, tea.KeyMsg{Type: tea.KeyEsc})
	if cancelled.screen != composeScreen || len(agentModel.requests) != 0 || !strings.Contains(cancelled.notice.text, "No file bytes were sent") {
		t.Fatalf("cancelled attachment confirmation = %#v, requests %#v", cancelled.notice, agentModel.requests)
	}
}

func TestAttachmentConfirmationRequiresFreshConsentWhenBytesChange(t *testing.T) {
	repository := testRepository(t)
	path := filepath.Join(repository, "screen.png")
	first := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x01}
	if err := os.WriteFile(path, first, 0o644); err != nil {
		t.Fatal(err)
	}
	agentModel := &testAgentModel{}
	model := New(Config{
		RepositoryPath: repository,
		Model:          "test-model",
		NewExecutor: func(string, string, string) (gatorrun.Executor, error) {
			return gatorrun.Executor{Model: agentModel}, nil
		},
	})
	model.runMode = gatorrun.PlanMode
	model.task.SetValue("Review @screen.png")
	pending := drive(t, model, tea.KeyMsg{Type: tea.KeyCtrlR})
	second := append([]byte(nil), first...)
	second[len(second)-1] = 0x02
	if err := os.WriteFile(path, second, 0o644); err != nil {
		t.Fatal(err)
	}
	rechecked := drive(t, pending, tea.KeyMsg{Type: tea.KeyEnter})
	if rechecked.screen != attachmentConfirmScreen || len(agentModel.requests) != 0 || !strings.Contains(rechecked.notice.text, "changed after preview") {
		t.Fatalf("changed attachment confirmation = screen %v, notice %#v, requests %#v", rechecked.screen, rechecked.notice, agentModel.requests)
	}
}

func TestTranscriptShowsPatchLinesAndReadOutput(t *testing.T) {
	patch := "--- a/example.go\n+++ b/example.go\n@@ -1 +1 @@\n-old\n+new\n"
	arguments, err := json.Marshal(struct {
		Patch string `json:"patch"`
	}{Patch: patch})
	if err != nil {
		t.Fatal(err)
	}
	called := renderEvent(agent.Event{Kind: agent.EventToolCalled, Step: 1, ToolCall: &agent.ToolCall{Name: "apply_patch", Arguments: arguments}})
	if !strings.Contains(called.detail, "-old") || !strings.Contains(called.detail, "+new") {
		t.Fatalf("patch transcript detail = %q", called.detail)
	}
	finished := renderEvent(agent.Event{Kind: agent.EventToolFinished, Step: 1, ToolCall: &agent.ToolCall{Name: "read_file"}, ToolResult: `{"ok":true,"result":{"content":"source"}}`})
	if !strings.Contains(finished.detail, "source") {
		t.Fatalf("read transcript detail = %q", finished.detail)
	}
	model := New(Config{})
	model.screen = reviewScreen
	model.events = []timelineEntry{called, finished}
	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if updated := next.(Model); updated.screen != transcriptScreen || !strings.Contains(updated.transcriptView(), "-old") {
		t.Fatalf("transcript view = %s", updated.transcriptView())
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

func TestInteractiveRunStreamsToConversation(t *testing.T) {
	repository := testRepository(t)
	agentModel := &testAgentModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "test", Name: "run_command", Arguments: json.RawMessage(`{"argv":["go","test","./..."]}`)}}},
		{Text: "The repository is verified and ready for review."},
		{ToolCalls: []agent.ToolCall{{ID: "follow-up-status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "follow-up-diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "follow-up-test", Name: "run_command", Arguments: json.RawMessage(`{"argv":["go","test","./..."]}`)}}},
		{Text: "The follow-up is complete."},
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
	model.focus = providerField
	_ = model.focusField()
	model.task.SetValue("Inspect @hello.go and verify this repository")

	updated := drive(t, model, tea.KeyMsg{Type: tea.KeyCtrlR})
	if updated.screen != composeScreen {
		t.Fatalf("screen = %v, want conversation", updated.screen)
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
	if !strings.Contains(updated.View(), "The repository is verified") || !strings.Contains(updated.View(), "Send a follow-up") {
		t.Fatalf("conversation omitted final result or follow-up prompt: %s", updated.View())
	}
	if updated.focus != taskField || !updated.task.Focused() {
		t.Fatalf("follow-up prompt was not focused: focus %v, task focused %t", updated.focus, updated.task.Focused())
	}

	updated.task.SetValue("Explain the implementation tradeoff.")
	continued := drive(t, updated, tea.KeyMsg{Type: tea.KeyCtrlR})
	if continued.screen != composeScreen || continued.runErr != nil || len(agentModel.requests) != 8 {
		t.Fatalf("follow-up conversation = screen %v, error %v, requests %d", continued.screen, continued.runErr, len(agentModel.requests))
	}
	if !strings.Contains(continued.View(), "The follow-up is complete") {
		t.Fatalf("conversation omitted follow-up result: %s", continued.View())
	}
}

func TestChatAggregatesStreamingTextAndKeepsToolActivityInOrder(t *testing.T) {
	model := New(Config{})
	model.width = 100
	model.height = 40
	model.resizeInputs()
	model.appendChat(chatEntry{author: chatUser, text: "Implement the feature."})
	model.appendEvent(agent.Event{Kind: agent.EventTextDelta, Step: 1, Text: "I will "})
	model.appendEvent(agent.Event{Kind: agent.EventTextDelta, Step: 1, Text: "inspect the repository."})
	model.appendEvent(agent.Event{Kind: agent.EventToolCalled, Step: 1, ToolCall: &agent.ToolCall{Name: "git_status", Arguments: json.RawMessage(`{}`)}})

	if len(model.chat) != 4 {
		t.Fatalf("chat entries = %#v", model.chat)
	}
	if got := model.chat[2].text; got != "I will inspect the repository." {
		t.Fatalf("streamed chat text = %q", got)
	}
	view := model.View()
	for _, expected := range []string{"Implement the feature.", "I will inspect the repository.", "tool -> git_status", "Message Gator"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("chat view omitted %q: %s", expected, view)
		}
	}
}

func TestChatPageKeysBrowseConversation(t *testing.T) {
	model := New(Config{})
	model.width = 100
	model.height = 40
	model.resizeInputs()
	for index := 0; index < 12; index++ {
		model.appendChat(chatEntry{author: chatAgent, text: fmt.Sprintf("message %d", index)})
	}
	last := model.chatIndex
	up, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	browsed := up.(Model)
	if browsed.chatIndex >= last {
		t.Fatalf("page up did not move through conversation: %d >= %d", browsed.chatIndex, last)
	}
	down, _ := browsed.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if got := down.(Model).chatIndex; got != last {
		t.Fatalf("page down index = %d, want %d", got, last)
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

func runTeaCommand(t *testing.T, model Model, command tea.Cmd) Model {
	t.Helper()
	for command != nil {
		current, nextCommand := model.Update(command())
		model = current.(Model)
		command = nextCommand
	}
	return model
}

func assertViewFits(t *testing.T, model Model, width, height int) {
	t.Helper()
	gotWidth, gotHeight := lipgloss.Size(model.View())
	if gotWidth > width || gotHeight > height {
		t.Fatalf("view is %dx%d, terminal is %dx%d:\n%s", gotWidth, gotHeight, width, height, model.View())
	}
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
