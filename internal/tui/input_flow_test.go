package tui

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/tools"
)

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
	model := New(Config{LocalModels: &fakeLocalModelManager{}})
	model.task.SetValue("/mod")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	if command != nil {
		t.Fatal("Tab executed the model command")
	}
	updated := next.(Model)
	if updated.focus != taskField || updated.task.Value() != "/model" {
		t.Fatalf("Tab completion = %#v", updated)
	}
	next, command = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("Enter did not execute the completed model command")
	}
	updated = next.(Model)
	if updated.screen != localModelsScreen || updated.task.Value() != "" {
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
	entry := updated.chat[len(updated.chat)-1]
	if updated.vim != vimOff || updated.task.Value() != "Explain this repository." || !entry.isError || !strings.Contains(entry.text, "No model provider") {
		t.Fatalf("Enter did not record its startup failure: %#v", entry)
	}
}

func TestCtrlCClearsComposerWithoutCancellingActiveRun(t *testing.T) {
	model := New(Config{})
	model.task.SetValue("Draft a focused implementation plan.")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if command == nil {
		t.Fatal("Ctrl+C did not restore focus to the cleared composer")
	}
	cleared := next.(Model)
	if cleared.task.Value() != "" || !strings.Contains(cleared.notice.text, "Message cleared") {
		t.Fatalf("Ctrl+C composer state = task %q, notice %#v", cleared.task.Value(), cleared.notice)
	}

	cancelled := false
	cleared.screen = runningScreen
	cleared.execution = &executionStream{cancel: func() { cancelled = true }, steering: make(chan string, 1)}
	cleared.task.SetValue("Check the failing test as well.")
	next, command = cleared.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if command == nil {
		t.Fatal("Ctrl+C did not restore focus to the running composer")
	}
	updated := next.(Model)
	if updated.task.Value() != "" || cancelled || updated.cancelling {
		t.Fatalf("Ctrl+C changed the active run: task %q cancelled=%t state=%t", updated.task.Value(), cancelled, updated.cancelling)
	}

	next, command = updated.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if command != nil || !next.(Model).cancelling || !cancelled {
		t.Fatalf("Ctrl+X did not request run cancellation: command=%v state=%#v", command, next.(Model))
	}
}

func TestCtrlQQuitsAndCloseCancelsNativeRun(t *testing.T) {
	model := New(Config{})
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if next.(Model).screen != composeScreen || command == nil {
		t.Fatalf("Ctrl+Q = model %#v, command %v", next.(Model), command)
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("Ctrl+Q command = %T, want tea.QuitMsg", command())
	}

	cancelled := false
	model.execution = &executionStream{cancel: func() { cancelled = true }}
	model.Close()
	if !cancelled {
		t.Fatal("Close did not cancel the active native run")
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
	entry := updated.chat[len(updated.chat)-1]
	if updated.vimCommand != "" || updated.task.Value() != "Explain this repository." || !entry.isError || !strings.Contains(entry.text, "No model provider") {
		t.Fatalf(":w state = command %q, task %q, entry %#v", updated.vimCommand, updated.task.Value(), entry)
	}

	updated.screen = runningScreen
	updated.execution = &executionStream{steering: make(chan string, 1)}
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
	model.execution = &executionStream{steering: make(chan string, 1)}
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

func TestCommandApprovalKeysDoNotSteer(t *testing.T) {
	reply := make(chan tools.CommandDecision, 1)
	model := New(Config{})
	model.screen = runningScreen
	model.execution = &executionStream{steering: make(chan string, 1)}
	model.pendingApproval = &pendingCommandApproval{argv: []string{"go", "env"}, reply: reply}
	model.task.SetValue("do not steer this")

	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil {
		t.Fatal("Enter started an unexpected command during command approval")
	}
	updated := next.(Model)
	if updated.pendingApproval != nil {
		t.Fatal("pending approval was not consumed")
	}
	select {
	case decision := <-reply:
		if decision != tools.CommandAllowOnce {
			t.Fatalf("decision = %s, want allow_once", decision)
		}
	default:
		t.Fatal("allow-once decision was not sent")
	}
	select {
	case instruction := <-updated.execution.steering:
		t.Fatalf("Enter steered active run with %q", instruction)
	default:
	}
}

func TestCommandApprovalDeny(t *testing.T) {
	reply := make(chan tools.CommandDecision, 1)
	model := New(Config{})
	model.screen = runningScreen
	model.pendingApproval = &pendingCommandApproval{argv: []string{"rm", "-rf", "/"}, reply: reply}

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if next.(Model).pendingApproval != nil {
		t.Fatal("deny left a pending approval")
	}
	select {
	case decision := <-reply:
		if decision != tools.CommandDeny {
			t.Fatalf("decision = %s, want deny", decision)
		}
	default:
		t.Fatal("deny decision was not sent")
	}
}

func TestRunningTabCompletesPartialSlashCommandBeforeQueueingIt(t *testing.T) {
	model := New(Config{})
	model.screen = runningScreen
	model.execution = &executionStream{steering: make(chan string, 1)}
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
	model.execution = &executionStream{steering: make(chan string, 1)}
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

func TestRunningViewShowsEventBackedActivityVerificationAndQueue(t *testing.T) {
	model := New(Config{Verification: [][]string{{"go", "test", "./..."}}})
	model.width = 120
	model.height = 60
	model.resizeInputs()
	model.screen = runningScreen
	model.execution = &executionStream{steering: make(chan string, 1)}
	model.queue = []queuedInput{{kind: queuedPrompt, text: "Add a regression test after this."}}
	model.beginRunActivity([][]string{{"go", "test", "./..."}})

	now := time.Now()
	model.appendEvent(agent.Event{Kind: agent.EventTurnStarted, At: now, Step: 3})
	model.appendEvent(agent.Event{Kind: agent.EventToolCalled, At: now, Step: 3, ToolCall: &agent.ToolCall{Name: "git_status", Arguments: json.RawMessage(`{}`)}})
	if model.activity.phase != activityInspecting {
		t.Fatalf("activity phase = %v, want inspecting", model.activity.phase)
	}
	view := model.View()
	for _, expected := range []string{"Tool", "tool -> git_status", "ctrl+b controls"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("running view omitted %q:\n%s", expected, view)
		}
	}
	model.drawerOpen = true
	model.drawerSection = drawerActivity
	model.resizeInputs()
	view = model.View()
	for _, expected := range []string{"Activity", "inspecting the isolated", "1 prompt queued", "Add a regression test", "Verification", "pending · go test ./..."} {
		if !strings.Contains(view, expected) {
			t.Fatalf("activity drawer omitted %q:\n%s", expected, view)
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
	model.drawerOpen = true
	model.drawerSection = drawerReview
	model.resizeInputs()
	view := model.View()
	for _, expected := range []string{"Review", "verification passed", "2 files · +2 −1"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("completed view omitted %q:\n%s", expected, view)
		}
	}

	model.runErr = errors.New("required verification failed: go test ./...")
	view = model.View()
	for _, expected := range []string{"Stopped: required verification failed", "Verification did not complete"} {
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
	model.execution = &executionStream{steering: make(chan string, 1)}
	model.queue = []queuedInput{{kind: queuedPrompt, text: "Check the edge cases after the active run."}}
	model.beginRunActivity([][]string{{"go", "test", "./..."}, {"go", "vet", "./..."}})
	model.appendEvent(agent.Event{Kind: agent.EventTurnStarted, Step: 1})

	for _, size := range []struct{ width, height int }{{44, 18}, {18, 10}} {
		next, _ := model.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
		assertViewFits(t, next.(Model), size.width, size.height)
	}
}

func TestQueueRejectsDirectShellCommandsAndHoldsAfterFailure(t *testing.T) {
	model := New(Config{})
	model.screen = runningScreen
	model.execution = &executionStream{steering: make(chan string, 1)}
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
	entry := paused.chat[len(paused.chat)-1]
	if entry.text != "Run stopped: provider unavailable" || !entry.isError {
		t.Fatalf("failed run transcript entry = %#v", entry)
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
