package tui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/journal"
	gatorrun "github.com/gongahkia/gator/internal/run"
)

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
			return gatorrun.Executor{}, errors.New("OPENAI_API_KEY is required")
		},
	})
	model.task.SetValue("Add a focused feature")
	model.refreshPreflight()
	if len(model.preflight) != 1 || !strings.Contains(model.preflight[0], "OPENAI_API_KEY") {
		t.Fatalf("preflight = %#v", model.preflight)
	}
	model.width = 100
	model.height = 40
	if !strings.Contains(model.preflightView(), "OPENAI_API_KEY") {
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

func TestThreadTreeShowsRetainedLineageWithoutChangingContinuation(t *testing.T) {
	repository := testRepository(t)
	stateDirectory := t.TempDir()
	worktreePath := t.TempDir()
	create := func(runID, parent, instruction, status string) journal.Record {
		t.Helper()
		entry, record, err := journal.Open(repository, runID, worktreePath, stateDirectory, time.Now())
		if err != nil {
			t.Fatalf("open %s: %v", runID, err)
		}
		messages := []agent.Message{{Role: agent.RoleUser, Content: "Implement retained thread view"}}
		if instruction != "" {
			messages = append(messages, agent.Message{Role: agent.RoleUser, Content: "Continue the original task with this developer instruction:\n" + instruction})
		}
		if err := entry.SaveSession(journal.Session{
			Version:         2,
			Repository:      repository,
			WorktreePath:    worktreePath,
			Provider:        "openai",
			Model:           "gpt-5.6",
			Task:            "Implement retained thread view",
			ThreadID:        "thread-tree-001",
			Mode:            "execute",
			Messages:        messages,
			ParentStatePath: parent,
		}); err != nil {
			t.Fatalf("save %s: %v", runID, err)
		}
		if err := entry.Finish(status, "Turn result", time.Now()); err != nil {
			t.Fatalf("finish %s: %v", runID, err)
		}
		return record
	}
	first := create("run-tree-root", "", "", "completed")
	second := create("run-tree-head", first.StatePath, "Inspect the retained history", "failed")
	if err := journal.SaveThread(stateDirectory, journal.Thread{
		Version: 1, ID: "forked-tree-001", Repository: repository, WorktreePath: worktreePath,
		Provider: "openai", Model: "gpt-5.6", Task: "Try the alternate approach", HeadStatePath: "/state/forked-tree-001",
		ForkedFrom: first.StatePath, TurnCount: 1, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("save forked thread: %v", err)
	}

	model := New(Config{RepositoryPath: repository, StateDir: stateDirectory})
	next, _ := model.beginContinuation(second.StatePath)
	continued := next.(Model)
	continued.task.SetValue("/tree")
	next, command := continued.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil {
		t.Fatal("thread tree returned an unexpected command")
	}
	tree := next.(Model)
	if tree.screen != threadScreen || tree.threadIndex != 1 || tree.resumeStatePath != second.StatePath {
		t.Fatalf("thread tree = %#v", tree)
	}
	next, _ = tree.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	tree = next.(Model)
	view := tree.threadTreeView()
	if !strings.Contains(view, "press f to fork") || !strings.Contains(view, "Inspect the retained history") || !strings.Contains(view, "fork forked-tree") || strings.Contains(view, second.StatePath) {
		t.Fatalf("thread tree view = %s", view)
	}
	next, _ = tree.Update(tea.KeyMsg{Type: tea.KeyUp})
	if next.(Model).threadIndex != 0 {
		t.Fatalf("thread tree did not select the prior turn: %#v", next)
	}
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	returned := next.(Model)
	if returned.screen != composeScreen || returned.resumeStatePath != second.StatePath {
		t.Fatalf("thread tree return = %#v", returned)
	}
}

func TestThreadTreeRequiresARetainedThread(t *testing.T) {
	model := New(Config{})
	model.task.SetValue("/tree")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil {
		t.Fatal("missing thread returned an unexpected command")
	}
	updated := next.(Model)
	if updated.screen != composeScreen || updated.notice.kind != noticeInfo || !strings.Contains(updated.notice.text, "No retained thread") {
		t.Fatalf("missing thread notice = %#v", updated.notice)
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
	entry := updated.chat[len(updated.chat)-1]
	if !entry.isError || !strings.Contains(entry.text, "escapes the workspace") || updated.notice.text != "" {
		t.Fatalf("workspace failure = entry %#v, notice %#v", entry, updated.notice)
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
