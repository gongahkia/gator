package worktui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/gongahkia/gator/internal/worksession"
)

func TestHomeComposerStartsConversationAndRetainsResult(t *testing.T) {
	model := New(Config{CurrentFolder: "/work/reports", Run: func(source, conversation, prompt string, _ RunOptions) RunResult {
		if source != "/work/reports" || conversation != "" || prompt != "draft" {
			t.Fatalf("run input = %q %q %q", source, conversation, prompt)
		}
		return RunResult{ConversationID: "work-one", RevisionID: "rev-one", SnapshotID: "snap-one", FinalText: "Done", OutputPath: "/output"}
	}})
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("draft")})
	model = updated.(Model)
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command == nil || !model.running || model.home {
		t.Fatal("conversation did not start")
	}
	updated, _ = model.Update(command())
	model = updated.(Model)
	if model.conversation != "work-one" || model.running || !strings.Contains(model.View(), "Done") {
		t.Fatalf("model = %#v\n%s", model, model.View())
	}
}

func TestRevisionCommandsStayInConversation(t *testing.T) {
	model := New(Config{CurrentFolder: "/work", Conversations: nil, MoveBack: func(id string) (string, error) { return "moved " + id, nil }})
	model.launcher = false
	model.home = false
	model.conversation = "work-one"
	model.title = "Work"
	model.input = "/back"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if !strings.Contains(model.View(), "moved work-one") {
		t.Fatalf("view = %s", model.View())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if !updated.(Model).launcher {
		t.Fatal("ctrl+p did not reopen launcher")
	}
}

func TestStartsRequestedConversationAndSelectsForwardBranch(t *testing.T) {
	called := ""
	model := New(Config{
		CurrentFolder:       "/work",
		StartConversationID: "work-one",
		Conversations:       []worksession.Conversation{{ID: "work-one", Title: "Report", SourcePath: "/source"}},
		MoveToRevision: func(conversationID, revisionID string) (string, error) {
			called = conversationID + "/" + revisionID
			return "moved", nil
		},
	})
	if model.launcher || model.home || model.conversation != "work-one" || model.source != "/source" {
		t.Fatalf("model did not open requested conversation: %#v", model)
	}
	model.input = "/forward revision-two"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if called != "work-one/revision-two" || !strings.Contains(updated.(Model).View(), "moved") {
		t.Fatalf("called = %q, view = %s", called, updated.(Model).View())
	}
}

func TestInitialViewIsADeclutteredCenteredComposer(t *testing.T) {
	model := New(Config{CurrentFolder: "/work/reports"})
	model.width = 100
	model.height = 30
	view := model.View()
	if !model.home || model.launcher || !strings.Contains(view, gatorWordmark) || !strings.Contains(view, "What do you want to accomplish?") || !strings.Contains(view, "Ask Gator to work on something") {
		t.Fatalf("initial view = %q", view)
	}
	if strings.Contains(view, "Inbox") || strings.Contains(view, "Scheduled jobs") || strings.Contains(view, "local-first work") {
		t.Fatalf("initial view exposes launcher clutter: %q", view)
	}
	if !strings.Contains(view, "ctrl+p commands") || !strings.Contains(view, "ctrl+x  l conversations") || !strings.Contains(view, "i inbox") || !strings.Contains(view, "j jobs") {
		t.Fatalf("initial view omits direct navigation: %q", view)
	}
	typing, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	typingView := typing.(Model).View()
	if strings.Contains(typingView, "What do you want to accomplish?") || !strings.Contains(typingView, gatorWordmark) || !strings.Contains(typingView, "Work in reports") {
		t.Fatalf("composer did not dock after typing: %q", typingView)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	view = updated.(Model).View()
	if !strings.Contains(view, "Commands") || !strings.Contains(view, "/attach") || !strings.Contains(view, "/status") {
		t.Fatalf("palette did not reveal universal commands: %q", view)
	}
	if strings.Contains(view, "Work in reports") || strings.Contains(view, "Inbox") || strings.Contains(view, "Scheduled jobs") {
		t.Fatalf("command palette still mixes destinations with actions: %q", view)
	}
	if strings.Contains(view, "Open the isolated coding workflow") {
		t.Fatalf("palette still exposes the retired Code frontend: %q", view)
	}
}

func TestCommandPaletteFiltersAndFillsCommandsThatNeedArguments(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("sandbox")})
	model = updated.(Model)
	view := model.View()
	if !strings.Contains(view, "/code sandbox") || strings.Contains(view, "/help") {
		t.Fatalf("filtered palette = %q", view)
	}
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command != nil || model.launcher || model.input != "/code sandbox " {
		t.Fatalf("selected command state = %#v", model)
	}
}

func TestCommandPaletteContainsOnlySlashCommands(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	model.openCommandPalette()
	expected := []string{
		"/help", "/new", "/model", "/effort", "/attach", "/detach", "/status", "/permissions",
		"/doctor", "/agents", "/settings", "/theme", "/history", "/back", "/forward", "/review",
		"/copy", "/queue", "/dequeue", "/clear-queue", "/code status", "/code verify", "/code scope",
		"/code profile", "/code setup", "/code allow", "/code allow-prefix", "/code sandbox",
		"/code network", "/code max-steps", "/code grant", "/code revoke", "/code browser",
		"/code reset", "/quit",
	}
	if len(model.entries) != len(expected) {
		t.Fatalf("command palette has %d entries, want %d", len(model.entries), len(expected))
	}
	for index, item := range model.entries {
		if !strings.HasPrefix(item.title, "/") || item.kind != "command" && item.kind != "command-input" {
			t.Fatalf("non-command entry in palette: %#v", item)
		}
		if item.title != expected[index] {
			t.Fatalf("command %d = %q, want %q", index, item.title, expected[index])
		}
	}
}

func TestCommandPaletteRowsShareOneLeftColumn(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	model.width, model.height = 100, 60
	model.openCommandPalette()
	lines := strings.Split(ansi.Strip(model.View()), "\n")
	position := -1
	for _, command := range []string{"/help", "/new", "/permissions", "/code grant", "/quit"} {
		found := false
		for _, line := range lines {
			if index := strings.Index(line, command); index >= 0 {
				if position < 0 {
					position = index
				} else if index != position {
					t.Fatalf("%s starts at column %d, want %d\n%s", command, index, position, ansi.Strip(model.View()))
				}
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("palette omitted %s", command)
		}
	}
}

func TestLeaderShortcutsOpenConversationsInboxAndJobs(t *testing.T) {
	model := New(Config{
		CurrentFolder: "/work",
		Conversations: []worksession.Conversation{{ID: "work-one", Title: "Quarterly plan", SourcePath: "/source"}},
	})
	press := func(key tea.KeyMsg) {
		updated, _ := model.Update(key)
		model = updated.(Model)
	}
	press(tea.KeyMsg{Type: tea.KeyCtrlX})
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if model.section != "inbox" || !strings.Contains(model.View(), "Inbox") || model.launcher {
		t.Fatalf("inbox shortcut state = %#v", model)
	}
	press(tea.KeyMsg{Type: tea.KeyEsc})
	press(tea.KeyMsg{Type: tea.KeyCtrlX})
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if model.section != "jobs" || !strings.Contains(model.View(), "Scheduled jobs") {
		t.Fatalf("jobs shortcut state = %#v", model)
	}
	press(tea.KeyMsg{Type: tea.KeyCtrlX})
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if !model.launcher || model.launcherMode != "conversations" || !strings.Contains(model.View(), "Quarterly plan") {
		t.Fatalf("conversation shortcut state = %#v", model)
	}
	press(tea.KeyMsg{Type: tea.KeyEnter})
	if model.launcher || model.section != "" || model.conversation != "work-one" || model.source != "/source" {
		t.Fatalf("selected conversation state = %#v", model)
	}
}

func TestMainComposerOwnsEffortAttachmentsAndCodePolicy(t *testing.T) {
	var received RunOptions
	model := New(Config{CurrentFolder: "/work", Run: func(_, _, _ string, options RunOptions) RunResult {
		received = options
		return RunResult{FinalText: "done"}
	}})
	for _, command := range []string{
		"/effort high", "/attach design.png", "/code verify go test ./...", "/code scope internal/parser",
		"/code profile implementer", "/code allow go test ./...", "/code grant lsp",
	} {
		model.input = command
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model = updated.(Model)
	}
	model.input = "implement the parser"
	updated, run := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if run == nil {
		t.Fatal("configured prompt did not start")
	}
	updated, _ = model.Update(run())
	model = updated.(Model)
	if received.MaxSteps != 48 || received.Code.MaxSteps != 32 || received.Code.Profile != "implementer" || len(received.Attachments) != 1 || len(received.Code.Verification) != 1 || len(received.Code.Scopes) != 1 || len(received.Code.AllowedCommands) != 1 || !sliceContains(received.Code.Capabilities, "lsp") {
		t.Fatalf("received options = %#v", received)
	}
	if len(model.options.Attachments) != 0 {
		t.Fatalf("per-send attachments were retained: %#v", model.options.Attachments)
	}
}

func TestComposerCommandsAcceptPathsAndValuesWithFlexibleWhitespace(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	for _, command := range []string{
		"/attach product briefs/q3 plan.pdf",
		"/code   verify   go test ./...",
	} {
		model.input = command
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model = updated.(Model)
	}
	if len(model.options.Attachments) != 1 || model.options.Attachments[0] != "product briefs/q3 plan.pdf" {
		t.Fatalf("attachments = %#v", model.options.Attachments)
	}
	if len(model.options.Code.Verification) != 1 || model.options.Code.Verification[0] != "go test ./..." {
		t.Fatalf("verification = %#v", model.options.Code.Verification)
	}
	model.input = "/detach product briefs/q3 plan.pdf"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(updated.(Model).options.Attachments) != 0 {
		t.Fatalf("attachment was not removed: %#v", updated.(Model).options.Attachments)
	}
}

func TestPromptsTypedDuringRunExecuteSequentially(t *testing.T) {
	var prompts []string
	model := New(Config{CurrentFolder: "/work", Run: func(_, _, prompt string, _ RunOptions) RunResult {
		prompts = append(prompts, prompt)
		return RunResult{ConversationID: "conversation-one", FinalText: "done " + prompt}
	}})
	model.input = "first"
	updated, first := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("second")})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if len(model.queue) != 1 {
		t.Fatalf("queue = %#v", model.queue)
	}
	updated, second := model.Update(first())
	model = updated.(Model)
	if second == nil || !model.running {
		t.Fatalf("queued run did not start: %#v", model)
	}
	updated, _ = model.Update(second())
	model = updated.(Model)
	if strings.Join(prompts, ",") != "first,second" || model.running || len(model.queue) != 0 {
		t.Fatalf("prompts=%#v model=%#v", prompts, model)
	}
}

func TestQueuedPromptsPauseAfterFailure(t *testing.T) {
	var prompts []string
	model := New(Config{CurrentFolder: "/work", Run: func(_, _, prompt string, _ RunOptions) RunResult {
		prompts = append(prompts, prompt)
		return RunResult{Error: "failed"}
	}})
	model.input = "first"
	updated, first := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	model.input = "second"
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, next := model.Update(first())
	model = updated.(Model)
	if next != nil || model.running || len(model.queue) != 1 || strings.Join(prompts, ",") != "first" || !strings.Contains(model.status, "paused") {
		t.Fatalf("prompts=%#v model=%#v", prompts, model)
	}
}

func TestFirstRunRetainsInitialTaskThroughGuidedSetup(t *testing.T) {
	called := ""
	selectedProvider := ""
	model := New(Config{
		CurrentFolder: "/work", FirstRun: true,
		CompleteSetup: func(provider string) error { selectedProvider = provider; return nil },
		Run: func(_, _, prompt string, _ RunOptions) RunResult {
			called = prompt
			return RunResult{FinalText: "done"}
		},
	})
	model.input = "prepare the brief"
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command != nil || !model.onboarding || model.pendingPrompt != "prepare the brief" || called != "" {
		t.Fatalf("setup state = %#v called=%q", model, called)
	}
	updated, command = model.Update(setupDone{provider: "openai"})
	model = updated.(Model)
	if command == nil || !model.running || model.firstRun || selectedProvider != "openai" || len(model.entries) == 0 || model.entries[0].kind == "onboarding" {
		t.Fatalf("post-setup state = %#v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(Model)
	if called != "prepare the brief" || model.running || !strings.Contains(model.View(), "done") {
		t.Fatalf("called=%q state=%#v", called, model)
	}
}

func TestFirstRunStillStartsSetupAfterLocalCommand(t *testing.T) {
	model := New(Config{CurrentFolder: "/work", FirstRun: true})
	model.input = "/help"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	model.input = "prepare the brief"
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command != nil || !model.onboarding || model.pendingPrompt != "prepare the brief" {
		t.Fatalf("setup state = %#v", model)
	}
}

func TestFirstRunRetainsSetupStateWhenProviderSelectionCannotBeSaved(t *testing.T) {
	model := New(Config{
		CurrentFolder: "/work", FirstRun: true,
		CompleteSetup: func(string) error { return errors.New("save failed") },
	})
	model.home = false
	model.onboarding = true
	model.messages = []message{{role: "You", text: "openai"}}
	updated, command := model.Update(setupDone{provider: "openai"})
	model = updated.(Model)
	if command != nil || !model.onboarding || !model.firstRun || !strings.Contains(model.View(), "save failed") {
		t.Fatalf("failed setup state = %#v", model)
	}
}
