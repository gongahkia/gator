package worktui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/worksession"
)

func TestHomeComposerStartsConversationAndRetainsResult(t *testing.T) {
	model := New(Config{CurrentFolder: "/work/reports", Run: func(source, conversation, prompt string) RunResult {
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
	if !model.home || model.launcher || !strings.Contains(view, "What do you want to accomplish?") || !strings.Contains(view, "Ask Gator to work on something") {
		t.Fatalf("initial view = %q", view)
	}
	if strings.Contains(view, "Inbox") || strings.Contains(view, "Scheduled jobs") || strings.Contains(view, "local-first work") {
		t.Fatalf("initial view exposes launcher clutter: %q", view)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	view = updated.(Model).View()
	if !strings.Contains(view, "Inbox") || !strings.Contains(view, "Scheduled jobs") {
		t.Fatalf("palette did not reveal secondary navigation: %q", view)
	}
}

func TestFirstRunRetainsInitialTaskThroughGuidedSetup(t *testing.T) {
	called := ""
	model := New(Config{CurrentFolder: "/work", FirstRun: true, Run: func(_, _, prompt string) RunResult {
		called = prompt
		return RunResult{FinalText: "done"}
	}})
	model.input = "prepare the brief"
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command != nil || !model.onboarding || model.pendingPrompt != "prepare the brief" || called != "" {
		t.Fatalf("setup state = %#v called=%q", model, called)
	}
	updated, command = model.Update(setupDone{provider: "openai"})
	model = updated.(Model)
	if command == nil || !model.running || model.firstRun {
		t.Fatalf("post-setup state = %#v", model)
	}
	updated, _ = model.Update(command())
	model = updated.(Model)
	if called != "prepare the brief" || model.running || !strings.Contains(model.View(), "done") {
		t.Fatalf("called=%q state=%#v", called, model)
	}
}
