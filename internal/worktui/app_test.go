package worktui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLauncherStartsConversationAndRetainsResult(t *testing.T) {
	model := New(Config{CurrentFolder: "/work/reports", Run: func(source, conversation, prompt string) RunResult {
		if source != "/work/reports" || conversation != "" || prompt != "draft" {
			t.Fatalf("run input = %q %q %q", source, conversation, prompt)
		}
		return RunResult{ConversationID: "work-one", RevisionID: "rev-one", SnapshotID: "snap-one", FinalText: "Done", OutputPath: "/output"}
	}})
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("draft")})
	model = updated.(Model)
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if command == nil || !model.running {
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
