package worktui

import (
	"context"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/gongahkia/gator/internal/jobs"
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

func TestConversationSessionControlsReachEachRun(t *testing.T) {
	var got RunOptions
	model := New(Config{
		CurrentFolder:    "/work",
		ConnectorChoices: func() []string { return []string{"notion"} },
		Run: func(_, _, _ string, options RunOptions) RunResult {
			got = options
			return RunResult{FinalText: "done"}
		},
	})
	for _, command := range []string{
		"/mode draft",
		"/artifact brief.md",
		"/connector notion",
		"/web-origin https://example.com",
		"/source ignore AGENTS.md",
		"/source-refresh",
	} {
		model.input = command
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model = updated.(Model)
	}
	model.input = "write it"
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command == nil {
		t.Fatal("run did not start")
	}
	updated, _ = model.Update(command())
	model = updated.(Model)
	if got.Mode != "draft" || !got.RefreshSource ||
		len(got.Artifacts) != 1 || got.Artifacts[0] != "brief.md" ||
		len(got.ConnectorIDs) != 1 || got.ConnectorIDs[0] != "notion" ||
		len(got.WebOrigins) != 1 || got.WebOrigins[0] != "https://example.com" ||
		len(got.IgnoredInstructionPaths) != 1 || got.IgnoredInstructionPaths[0] != "AGENTS.md" {
		t.Fatalf("run options = %#v", got)
	}
	if model.options.RefreshSource {
		t.Fatal("source refresh was not one-shot")
	}
}

func TestCodeModeRequiresTheSharedWorkCodeSpecialist(t *testing.T) {
	var got RunOptions
	model := New(Config{CurrentFolder: "/work", Run: func(_, _, _ string, options RunOptions) RunResult {
		got = options
		return RunResult{FinalText: "done"}
	}})
	model.input = "/code on"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if !model.options.RequireCode {
		t.Fatal("/code on did not enable the Work Code requirement")
	}
	model.input = "implement the patch"
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command == nil {
		t.Fatal("Code Work did not start")
	}
	updated, _ = model.Update(command())
	model = updated.(Model)
	if !got.RequireCode || !strings.Contains(model.View(), "done") {
		t.Fatalf("Code Work options = %#v\n%s", got, model.View())
	}
}

func TestSourceIgnoreCommandsManageProjectInstructionPaths(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	for _, command := range []string{"/source ignore AGENTS.md", "/source ignored"} {
		model.input = command
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model = updated.(Model)
	}
	if len(model.options.IgnoredInstructionPaths) != 1 || model.options.IgnoredInstructionPaths[0] != "AGENTS.md" || !strings.Contains(model.View(), "Ignored project instruction files") {
		t.Fatalf("ignored instructions = %#v\n%s", model.options.IgnoredInstructionPaths, model.View())
	}
	model.input = "/source unignore AGENTS.md"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if len(model.options.IgnoredInstructionPaths) != 0 {
		t.Fatalf("ignored instructions were not restored: %#v", model.options.IgnoredInstructionPaths)
	}
}

func TestJobsUseTheConfiguredSharedServiceAndRefreshTheSection(t *testing.T) {
	var received []string
	model := New(Config{
		CurrentFolder: "/work",
		JobAction: func(arguments []string) (string, error) {
			received = append([]string(nil), arguments...)
			return "Job added.", nil
		},
		RefreshJobs: func() ([]jobs.Definition, error) {
			return []jobs.Definition{{ID: "job-daily", Name: "Daily review", Enabled: true, Schedule: "0 9 * * *", Timezone: "UTC"}}, nil
		},
	})
	model.input = "/jobs add daily --schedule 0 9 * * * -- review changes"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if !reflect.DeepEqual(received, []string{"add", "daily", "--schedule", "0", "9", "*", "*", "*", "--", "review", "changes"}) || len(model.config.Jobs) != 1 || !strings.Contains(model.View(), "Job added.") {
		t.Fatalf("job action = %#v jobs=%#v\n%s", received, model.config.Jobs, model.View())
	}
	model.input = "/jobs"
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.section != "jobs" || !strings.Contains(model.View(), "Daily review") {
		t.Fatalf("jobs section = %#v\n%s", model, model.View())
	}
}

func TestFeedbackUsesTheConfiguredSharedServiceForCurrentRevision(t *testing.T) {
	var workID string
	var received []string
	model := New(Config{CurrentFolder: "/work", FeedbackAction: func(id string, arguments []string) (string, error) {
		workID = id
		received = append([]string(nil), arguments...)
		return "feedback recorded", nil
	}})
	model.revision = "work-feedback"
	model.input = "/feedback accept"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if workID != "work-feedback" || !reflect.DeepEqual(received, []string{"accept"}) || !strings.Contains(model.View(), "feedback recorded") {
		t.Fatalf("feedback action work=%q args=%#v\n%s", workID, received, model.View())
	}
	for _, item := range commandPaletteEntries() {
		if item.command == "/feedback" {
			return
		}
	}
	t.Fatal("command palette omitted feedback")
}

func TestVerifiedDeliverablesRenderAndSaveWithOneConfirmation(t *testing.T) {
	var requests []BundleActionRequest
	model := New(Config{
		CurrentFolder: "/work",
		BundleAction: func(request BundleActionRequest) (string, error) {
			requests = append(requests, request)
			if request.Execute {
				return "saved", nil
			}
			return "create brief.md", nil
		},
	})
	model.home = false
	updated, _ := model.Update(runDone(RunResult{
		FinalText: "Done",
		Bundle: BundleSummary{
			Path: "/state/run",
			Artifacts: []ArtifactSummary{{
				Path: "brief.md", MediaType: "text/markdown", Bytes: 42, Valid: true,
			}},
		},
	}))
	model = updated.(Model)
	if view := ansi.Strip(model.View()); !strings.Contains(view, "Verified deliverables") || !strings.Contains(view, "brief.md") {
		t.Fatalf("bundle card = %q", view)
	}
	model.input = "/save /destination"
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command != nil || model.pendingBundleAction == nil || !strings.Contains(ansi.Strip(model.View()), "Review transfer") {
		t.Fatal("save did not stop for one explicit confirmation")
	}
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command == nil {
		t.Fatal("confirmation did not start save")
	}
	updated, _ = model.Update(command())
	model = updated.(Model)
	if len(requests) != 2 || requests[0].Execute || !requests[1].Execute || !strings.Contains(model.View(), "saved") {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestRetryUsesTheSameConfirmedBundleActionPath(t *testing.T) {
	var requests []BundleActionRequest
	model := New(Config{
		CurrentFolder: "/work",
		BundleAction: func(request BundleActionRequest) (string, error) {
			requests = append(requests, request)
			if request.Execute {
				return "retry finished", nil
			}
			return "report.md · failed", nil
		},
	})
	model.home = false
	model.lastBundle = BundleSummary{Path: "/state/run"}
	model.input = "/retry delivery-one"
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command != nil || model.pendingBundleAction == nil || model.pendingBundleAction.request.Action != "retry" || model.pendingBundleAction.request.DeliveryID != "delivery-one" {
		t.Fatalf("retry did not prepare a confirmed bundle action: %#v", model.pendingBundleAction)
	}
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command == nil {
		t.Fatal("retry confirmation did not start delivery")
	}
	updated, _ = model.Update(command())
	model = updated.(Model)
	if len(requests) != 2 || requests[0].Action != "retry" || requests[0].Execute || requests[1].Action != "retry" || !requests[1].Execute || !strings.Contains(model.View(), "retry finished") {
		t.Fatalf("retry requests = %#v", requests)
	}
}

func TestRevisionCommandsStayInConversation(t *testing.T) {
	model := New(Config{CurrentFolder: "/work", Conversations: nil, MoveBack: func(id string) (string, error) { return "moved " + id, nil }})
	model.launcher = false
	model.home = false
	model.conversation = "work-one"
	model.title = "Work"
	model.input = "/revision-back"
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
	model.input = "/revision-forward revision-two"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if called != "work-one/revision-two" || !strings.Contains(updated.(Model).View(), "moved") {
		t.Fatalf("called = %q, view = %s", called, updated.(Model).View())
	}
}

func TestRequestedConversationRestoresTranscriptCardsAndSelectedRevision(t *testing.T) {
	bundle := BundleSummary{Path: "/retained/rev-two", Status: "completed", Artifacts: []ArtifactSummary{{
		Path: "report.docx", MediaType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", Bytes: 128, Valid: true,
	}}}
	model := New(Config{
		CurrentFolder:       "/work",
		StartConversationID: "work-one",
		Conversations:       []worksession.Conversation{{ID: "work-one", Title: "Quarterly report", SourcePath: "/source"}},
		LoadConversation: func(id string) (ConversationState, error) {
			if id != "work-one" {
				t.Fatalf("conversation ID = %q", id)
			}
			return ConversationState{
				Title: "Quarterly report", SourcePath: "/source", RevisionID: "rev-two", SnapshotID: "snap-two",
				OutputPath: "/retained/rev-two/output", LastBundle: bundle,
				Messages: []TranscriptMessage{
					{Role: "You", Text: "Draft the report"},
					{Role: "Gator", Text: "The first draft is ready."},
					{Role: "Deliverables", Bundle: &BundleSummary{Path: "/retained/rev-one", Artifacts: []ArtifactSummary{{Path: "report.md", Valid: true}}}},
					{Role: "You", Text: "Revise the executive summary"},
					{Role: "Gator", Text: "The revised report is ready."},
					{Role: "Deliverables", Bundle: &bundle},
				},
			}, nil
		},
		LoadConversationOptions: func(string) (RunOptions, error) {
			return RunOptions{Context: context.Background(), Mode: "draft", PreviousArtifacts: []string{"report.docx"}}, nil
		},
	})
	model.width, model.height = 120, 80
	view := ansi.Strip(model.View())
	for _, want := range []string{
		"Draft the report", "The first draft is ready.", "Revise the executive summary", "The revised report is ready.",
		"report.md", "report.docx", "Revision rev-two", "snapshot snap-two",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("restored view is missing %q:\n%s", want, view)
		}
	}
	if model.lastBundle.Path != bundle.Path || model.lastOutput != "/retained/rev-two/output" || model.options.Context != nil || model.options.Mode != "draft" {
		t.Fatalf("restored state = %#v", model)
	}
}

func TestRevisionNavigationReloadsSelectedThreadAndBundle(t *testing.T) {
	head := "rev-two"
	stateFor := func(revision string) ConversationState {
		bundle := BundleSummary{Path: "/retained/" + revision, Artifacts: []ArtifactSummary{{Path: revision + ".md", Valid: true}}}
		return ConversationState{
			Title: "Report", SourcePath: "/source", RevisionID: revision, SnapshotID: "snap-" + revision,
			OutputPath: bundle.Path + "/output", LastBundle: bundle,
			Messages: []TranscriptMessage{{Role: "You", Text: "Prompt " + revision}, {Role: "Gator", Text: "Answer " + revision}, {Role: "Deliverables", Bundle: &bundle}},
		}
	}
	model := New(Config{
		CurrentFolder:       "/work",
		StartConversationID: "work-one",
		Conversations:       []worksession.Conversation{{ID: "work-one", Title: "Report", SourcePath: "/source"}},
		LoadConversation: func(id string) (ConversationState, error) {
			if id != "work-one" {
				t.Fatalf("conversation ID = %q", id)
			}
			return stateFor(head), nil
		},
		MoveBack: func(id string) (string, error) {
			if id != "work-one" {
				t.Fatalf("move ID = %q", id)
			}
			head = "rev-one"
			return "Moved back to rev-one.", nil
		},
		MoveForward: func(id string) (string, error) {
			if id != "work-one" {
				t.Fatalf("move ID = %q", id)
			}
			head = "rev-two"
			return "Moved forward to rev-two.", nil
		},
	})
	model.height = 60
	model.input = "/revision-back"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	view := ansi.Strip(model.View())
	if strings.Contains(view, "Answer rev-two") || strings.Contains(view, "rev-two.md") ||
		!strings.Contains(view, "Answer rev-one") || !strings.Contains(view, "rev-one.md") ||
		!strings.Contains(view, "Moved back to rev-one.") || model.lastBundle.Path != "/retained/rev-one" {
		t.Fatalf("navigation did not replace selected thread:\n%s\nstate=%#v", view, model)
	}
	model.input = "/revision-forward"
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	view = ansi.Strip(model.View())
	if strings.Contains(view, "Answer rev-one") || strings.Contains(view, "rev-one.md") ||
		!strings.Contains(view, "Answer rev-two") || !strings.Contains(view, "rev-two.md") ||
		!strings.Contains(view, "Moved forward to rev-two.") || model.lastBundle.Path != "/retained/rev-two" {
		t.Fatalf("forward navigation did not replace selected thread:\n%s\nstate=%#v", view, model)
	}
}

func TestConversationPickerRefreshesRetainedConversationList(t *testing.T) {
	model := New(Config{
		CurrentFolder: "/work",
		ListConversations: func() ([]worksession.Conversation, error) {
			return []worksession.Conversation{{ID: "work-new", Title: "Newly retained", SourcePath: "/source"}}, nil
		},
		LoadConversation: func(id string) (ConversationState, error) {
			return ConversationState{Title: "Newly retained", SourcePath: "/source", RevisionID: "rev-new", Messages: []TranscriptMessage{{Role: "Gator", Text: id}}}, nil
		},
	})
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	model = updated.(Model)
	if !strings.Contains(model.View(), "Newly retained") {
		t.Fatalf("picker did not refresh conversations: %s", model.View())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.conversation != "work-new" || !strings.Contains(model.View(), "work-new") {
		t.Fatalf("picker did not restore selected conversation: %#v\n%s", model, model.View())
	}
}

func TestInitialViewIsADeclutteredBottomComposer(t *testing.T) {
	model := New(Config{CurrentFolder: "/work/reports"})
	model.width = 100
	model.height = 30
	view := model.View()
	if !model.home || model.launcher || !strings.Contains(view, gatorWordmark) || strings.Contains(view, "What do you want to accomplish?") || !strings.Contains(view, "Ask Gator to work on something") {
		t.Fatalf("initial view = %q", view)
	}
	initialLines := strings.Split(ansi.Strip(view), "\n")
	if !strings.HasPrefix(initialLines[0], gatorWordmark) {
		t.Fatalf("initial logo is not top-left: %q", initialLines[0])
	}
	for _, line := range initialLines {
		if strings.Contains(line, "Ask Gator to work on something") && !strings.HasPrefix(line, "│") {
			t.Fatalf("initial composer is not left-aligned: %q", line)
		}
	}
	if strings.Contains(view, "Inbox") || strings.Contains(view, "Scheduled jobs") || strings.Contains(view, "local-first work") {
		t.Fatalf("initial view exposes launcher clutter: %q", view)
	}
	for _, shortcut := range []string{"ctrl+g editor", "ctrl+i workspace", "ctrl+p commands", "ctrl+x conversations", "ctrl+b inbox", "ctrl+j jobs", "model ", "mode auto"} {
		if strings.Contains(view, shortcut) {
			t.Fatalf("initial view exposes status-line clutter %q: %q", shortcut, view)
		}
	}
	typing, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	typingModel := typing.(Model)
	typingView := typingModel.View()
	if !typingModel.home || strings.Contains(typingView, "What do you want to accomplish?") || !strings.Contains(typingView, gatorWordmark) || strings.Contains(typingView, "Work in reports") {
		t.Fatalf("composer did not stay bottom-anchored while drafting: %q", typingView)
	}
	submitted, _ := typingModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	submittedModel := submitted.(Model)
	submittedView := ansi.Strip(submittedModel.View())
	if submittedModel.home || strings.Contains(submittedView, "What do you want to accomplish?") || !strings.Contains(submittedView, "Work in reports") {
		t.Fatalf("composer did not enter the Work view after submission: %q", submittedView)
	}
	composerRow := -1
	for row, line := range strings.Split(submittedView, "\n") {
		if strings.Contains(line, "Ask Gator to work on something") {
			composerRow = row
			break
		}
	}
	if composerRow < submittedModel.height-3 {
		t.Fatalf("composer row = %d, want the bottom of %d rows:\n%s", composerRow, submittedModel.height, submittedView)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	view = updated.(Model).View()
	if !strings.Contains(view, "Commands") || !strings.Contains(view, "/jobs") || !strings.Contains(view, "/learnings") {
		t.Fatalf("palette did not reveal the product commands: %q", view)
	}
	if strings.Contains(view, "/attach") || strings.Contains(view, "/status") {
		t.Fatalf("palette retained advanced command bloat: %q", view)
	}
	if strings.Contains(view, "Work in reports") || strings.Contains(view, "Inbox") || strings.Contains(view, "Scheduled jobs") {
		t.Fatalf("command palette still mixes destinations with actions: %q", view)
	}
	if strings.Contains(view, "Open the isolated coding workflow") {
		t.Fatalf("palette still exposes the retired Code frontend: %q", view)
	}
}

func TestBottomHomeComposerGrowsForWrappedDrafts(t *testing.T) {
	model := New(Config{CurrentFolder: "/work/reports"})
	model.width, model.height = 60, 30
	model.input = strings.Repeat("draft ", 30)
	view := ansi.Strip(model.View())
	if strings.Contains(view, "What do you want to accomplish?") || strings.Count(view, "│") <= 2 {
		t.Fatalf("bottom composer did not grow across wrapped rows: %q", view)
	}
}

func TestRunningWorkUsesRattlesLoadingFrames(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	model.home = false
	model.running = true
	model.loadingRun = 7
	model.runStarted = time.Now().Add(-20 * time.Second)

	if view := ansi.Strip(model.View()); !strings.Contains(view, "⠋ Working…") {
		t.Fatalf("initial loading view = %q", view)
	}
	if view := ansi.Strip(model.View()); !strings.Contains(view, "20s · esc to interrupt") {
		t.Fatalf("initial loading view omitted elapsed work time: %q", view)
	}
	updated, command := model.Update(loadingTickMsg{run: 7})
	model = updated.(Model)
	if command == nil || model.loadingFrame != 1 {
		t.Fatalf("loading tick was not scheduled: frame=%d command=%v", model.loadingFrame, command != nil)
	}
	if view := ansi.Strip(model.View()); !strings.Contains(view, "⠙ Working…") {
		t.Fatalf("updated loading view = %q", view)
	}
	updated, command = model.Update(loadingTickMsg{run: 6})
	if command != nil || updated.(Model).loadingFrame != 1 {
		t.Fatal("a stale run tick must not animate a later run")
	}
}

func TestCommandPaletteFiltersAndFillsCommandsThatNeedArguments(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jobs")})
	model = updated.(Model)
	view := model.View()
	if !strings.Contains(view, "/jobs") || strings.Contains(view, "/help") {
		t.Fatalf("filtered palette = %q", view)
	}
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command != nil || model.launcher || model.input != "/jobs " {
		t.Fatalf("selected command state = %#v", model)
	}
}

func TestCommandPaletteContainsCurrentCommands(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	model.openCommandPalette()
	expected := []string{
		"/help", "/new", "/model", "/source", "/mode", "/settings", "/settings statusline", "/theme", "/history", "/jobs", "/learnings", "/feedback", "/review", "/save", "/apply", "/retry", "/copy", "exit",
	}
	if len(model.entries) != len(expected) {
		t.Fatalf("command palette has %d entries, want %d", len(model.entries), len(expected))
	}
	for index, item := range model.entries {
		if item.kind != "command" && item.kind != "command-input" {
			t.Fatalf("non-command entry in palette: %#v", item)
		}
		if item.title != expected[index] {
			t.Fatalf("command %d = %q, want %q", index, item.title, expected[index])
		}
	}
}

func TestSourceMenuIncludesWorkspaceSelectionAndRefresh(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	model.input = "/source"
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command != nil || !model.launcher || model.launcherMode != "source" || !strings.Contains(model.View(), "Current: /work") {
		t.Fatalf("source menu state = %#v\n%s", model, model.View())
	}
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command != nil || model.launcher || model.input != "/source " {
		t.Fatalf("source selection input state = %#v", model)
	}

	model.openSourceMenu()
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command != nil || model.launcher || !model.options.RefreshSource {
		t.Fatalf("source refresh state = %#v", model)
	}
}

func TestCopyPromptsForTheResponseOrDeliverableSummary(t *testing.T) {
	var copied []string
	model := New(Config{
		CurrentFolder: "/work",
		Copy:          func(text string) error { copied = append(copied, text); return nil },
	})
	model.messages = []message{{role: "Gator", text: "First response"}, {role: "Gator", text: "Latest response"}}
	model.lastBundle = BundleSummary{Path: "/state/bundle", Artifacts: []ArtifactSummary{{Path: "brief.md", Valid: true}}}
	model.input = "/copy"
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command != nil || !model.launcher || model.launcherMode != "copy" || len(model.entries) != 3 || !strings.Contains(model.View(), "Verified deliverables") {
		t.Fatalf("copy picker state = %#v\n%s", model, model.View())
	}
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command != nil || model.launcher || !reflect.DeepEqual(copied, []string{"Latest response"}) {
		t.Fatalf("selected copy = %#v, state = %#v", copied, model)
	}
}

func TestExitLeavesTheTUI(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	model.input = "exit"
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatalf("exit did not produce an exit command: %#v", updated.(Model))
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("exit command = %T, want tea.QuitMsg", command())
	}
}

func TestRemovedTUIAliasesAreUnknown(t *testing.T) {
	for _, input := range []string{"/learning list", "/status-line", "/quit"} {
		model := New(Config{CurrentFolder: "/work"})
		model.input = input
		updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if command != nil || !strings.Contains(updated.(Model).View(), "unknown command") {
			t.Fatalf("%s still routes: %#v", input, updated.(Model))
		}
	}
}

func TestProviderSlashCommandsAreNotDispatched(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	for _, command := range []string{"/connect openai", "/login openai", "/logout openai"} {
		model.input = command
		updated, run := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model = updated.(Model)
		if run != nil || !strings.Contains(model.View(), "unknown command") {
			t.Fatalf("%s was still dispatched: %#v\n%s", command, model, model.View())
		}
	}
}

func TestConnectorSetupAndManagementRunInsideTUI(t *testing.T) {
	var calls [][]string
	available := []string(nil)
	model := New(Config{
		CurrentFolder: "/work",
		ConnectorChoices: func() []string {
			return append([]string(nil), available...)
		},
		ConnectorAction: func(arguments []string) (string, error) {
			calls = append(calls, append([]string(nil), arguments...))
			if arguments[0] == "add" {
				available = []string{"google-work"}
				return "Added connector google-work.", nil
			}
			return "done", nil
		},
	})
	model.input = "/connector setup google-work desktop-client-id"
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command == nil {
		t.Fatal("connector setup did not start")
	}
	updated, _ = model.Update(command())
	model = updated.(Model)
	if len(calls) != 1 || strings.Join(calls[0], " ") != "add google-work --kind google --oauth-client-id desktop-client-id" {
		t.Fatalf("connector setup arguments = %#v", calls)
	}
	if view := model.View(); !strings.Contains(view, "Added connector google-work") || !strings.Contains(view, "/connector login google-work prompt") {
		t.Fatalf("connector setup result = %s", view)
	}
	model.input = "/connector google-work"
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if len(model.options.ConnectorIDs) != 1 || model.options.ConnectorIDs[0] != "google-work" {
		t.Fatalf("new connector choices were not refreshed: %#v", model.options.ConnectorIDs)
	}
}

func TestConnectorLoginUsesInteractiveCommandAndSelectsOnSuccess(t *testing.T) {
	var arguments []string
	model := New(Config{
		CurrentFolder: "/work",
		ConnectorCommand: func(values []string) *exec.Cmd {
			arguments = append([]string(nil), values...)
			return exec.Command("true")
		},
	})
	model.input = "/connector login google-work prompt"
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command == nil || strings.Join(arguments, " ") != "login google-work --prompt-client-secret" {
		t.Fatalf("connector login command = %#v", arguments)
	}
	updated, _ = model.Update(connectorActionDone{action: "login", id: "google-work"})
	model = updated.(Model)
	if len(model.options.ConnectorIDs) != 1 || model.options.ConnectorIDs[0] != "google-work" ||
		!strings.Contains(model.View(), "authenticated and selected") {
		t.Fatalf("connector login result = %#v\n%s", model.options.ConnectorIDs, model.View())
	}
}

func TestCommandPaletteRowsShareOneLeftColumn(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	model.width, model.height = 100, 60
	model.openCommandPalette()
	lines := strings.Split(ansi.Strip(model.View()), "\n")
	position := -1
	for _, command := range []string{"/help", "/new", "/jobs", "/retry", "exit"} {
		found := false
		for _, line := range lines {
			if index := strings.Index(line, command); index >= 0 {
				column := ansi.StringWidth(line[:index])
				if position < 0 {
					position = column
				} else if column != position {
					t.Fatalf("%s starts at column %d, want %d\n%s", command, column, position, ansi.Strip(model.View()))
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

func TestDirectShortcutsOpenConversationsInboxAndJobs(t *testing.T) {
	model := New(Config{
		CurrentFolder: "/work",
		Conversations: []worksession.Conversation{{ID: "work-one", Title: "Quarterly plan", SourcePath: "/source"}},
	})
	press := func(key tea.KeyMsg) {
		updated, _ := model.Update(key)
		model = updated.(Model)
	}
	press(tea.KeyMsg{Type: tea.KeyCtrlI})
	if !model.launcher || model.launcherMode != "source" || !strings.Contains(model.View(), "Change workspace") {
		t.Fatalf("workspace shortcut state = %#v", model)
	}
	press(tea.KeyMsg{Type: tea.KeyEnter})
	if model.launcher || model.input != "/source " {
		t.Fatalf("workspace selection input state = %#v", model)
	}
	press(tea.KeyMsg{Type: tea.KeyCtrlB})
	if model.section != "inbox" || !strings.Contains(model.View(), "Inbox") || model.launcher {
		t.Fatalf("inbox shortcut state = %#v", model)
	}
	press(tea.KeyMsg{Type: tea.KeyEsc})
	press(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if model.section != "jobs" || !strings.Contains(model.View(), "Scheduled jobs") {
		t.Fatalf("jobs shortcut state = %#v", model)
	}
	press(tea.KeyMsg{Type: tea.KeyCtrlX})
	if !model.launcher || model.launcherMode != "conversations" || !strings.Contains(model.View(), "Quarterly plan") {
		t.Fatalf("conversation shortcut state = %#v", model)
	}
	press(tea.KeyMsg{Type: tea.KeyEnter})
	if model.launcher || model.section != "" || model.conversation != "work-one" || model.source != "/source" {
		t.Fatalf("selected conversation state = %#v", model)
	}
}

func TestMainComposerOwnsEffortAndAttachments(t *testing.T) {
	var received RunOptions
	model := New(Config{CurrentFolder: "/work", Run: func(_, _, _ string, options RunOptions) RunResult {
		received = options
		return RunResult{FinalText: "done"}
	}})
	for _, command := range []string{"/effort high", "/attach design.png"} {
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
	if received.MaxSteps != 48 || received.Code.MaxSteps != 32 || len(received.Attachments) != 1 {
		t.Fatalf("received options = %#v", received)
	}
	if len(model.options.Attachments) != 0 {
		t.Fatalf("per-send attachments were retained: %#v", model.options.Attachments)
	}
}

func TestComposerCommandsAcceptPathsWithFlexibleWhitespace(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	for _, command := range []string{"/attach product briefs/q3 plan.pdf"} {
		model.input = command
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model = updated.(Model)
	}
	if len(model.options.Attachments) != 1 || model.options.Attachments[0] != "product briefs/q3 plan.pdf" {
		t.Fatalf("attachments = %#v", model.options.Attachments)
	}
	model.input = "/detach product briefs/q3 plan.pdf"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(updated.(Model).options.Attachments) != 0 {
		t.Fatalf("attachment was not removed: %#v", updated.(Model).options.Attachments)
	}
}

func TestWorkTUIRejectsUnsupportedCodeCommandOptions(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	initial := cloneRunOptions(model.options)
	model.input = "/code verify go test ./..."
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command != nil || !strings.Contains(model.View(), "usage: /code on|off") {
		t.Fatalf("Code command validation = %#v\n%s", model, model.View())
	}
	if model.options.RequireCode || !reflect.DeepEqual(model.options.Code, initial.Code) {
		t.Fatalf("Code settings changed from %#v to %#v", initial.Code, model.options.Code)
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

func TestFirstRunRetainsInitialTaskInModelManagement(t *testing.T) {
	panel := &testModelPanel{}
	model := New(Config{
		CurrentFolder: "/work", FirstRun: true,
		Models:        func() (ModelPanel, error) { return panel, nil },
		SelectedModel: func() (string, string, error) { return "openai", "gpt-5", nil },
	})
	model.input = "prepare the brief"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.models == nil || model.pendingPrompt != "prepare the brief" {
		t.Fatalf("model setup state = %#v", model)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if model.models != nil || model.firstRun || model.input != "prepare the brief" || !strings.Contains(model.status, "Selected model: openai / gpt-5") {
		t.Fatalf("post-model state = %#v", model)
	}
}

func TestFirstRunStillOpensModelManagementAfterLocalCommand(t *testing.T) {
	panel := &testModelPanel{}
	model := New(Config{CurrentFolder: "/work", FirstRun: true, Models: func() (ModelPanel, error) { return panel, nil }})
	model.input = "/help"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	model.input = "prepare the brief"
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.models == nil || model.pendingPrompt != "prepare the brief" {
		t.Fatalf("model setup state = %#v", model)
	}
}
