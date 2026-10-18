package worktui

import (
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/workhome"
)

func TestBridgeUsesLiveCommandAndMessageState(t *testing.T) {
	model := New(Config{CurrentFolder: t.TempDir(), Attention: workhome.Attention{UnreadResults: 1}})
	initial := model.BridgeState()
	if initial.Mode != "chat" || !initial.Home || len(initial.Actions) == 0 || initial.Actions[0].Command != "/inbox" {
		t.Fatalf("initial bridge state = %#v", initial)
	}
	foundHelp := false
	for _, command := range initial.Commands {
		foundHelp = foundHelp || command.Name == "help"
	}
	if !foundHelp {
		t.Fatalf("bridge command list omits /help: %#v", initial.Commands)
	}
	next, _ := model.SubmitText("/help")
	state := next.(Model).BridgeState()
	if state.Home || len(state.Messages) != 1 || state.Messages[0].Role != "Gator" || !strings.Contains(state.Messages[0].Text, "/model") {
		t.Fatalf("submitted command bridge state = %#v", state)
	}
	model.openCommandPalette()
	modal := model.BridgeState()
	if modal.Mode != "panel" || modal.Panel == nil || modal.Panel.Title != "Commands" {
		t.Fatalf("modal bridge state = %#v", modal)
	}
}

func TestBridgeProjectsCommandPanelsWithoutGoView(t *testing.T) {
	for _, test := range []struct {
		command string
		title   string
	}{
		{command: "/inbox", title: "Inbox"},
		{command: "/jobs", title: "Scheduled jobs"},
		{command: "/history", title: "History"},
		{command: "/learnings", title: "Learnings"},
		{command: "/goals", title: "Ongoing goals"},
	} {
		t.Run(test.command, func(t *testing.T) {
			model := New(Config{CurrentFolder: t.TempDir()})
			next, _ := model.SubmitText(test.command)
			state := next.(Model).BridgeState()
			if state.Mode != "panel" || state.Panel == nil || state.Panel.Title != test.title {
				t.Fatalf("%s panel = %#v", test.command, state.Panel)
			}
		})
	}
	model := New(Config{CurrentFolder: t.TempDir()})
	model.pendingBundleAction = &pendingBundleAction{summary: "Save one draft"}
	state := model.BridgeState()
	if state.Mode != "panel" || state.Panel == nil || state.Panel.Title != "Review transfer" || len(state.Panel.Blocks) != 1 {
		t.Fatalf("confirmation panel = %#v", state.Panel)
	}
	model.pendingBundleAction = nil
	model.openStatusLineEditor()
	state = model.BridgeState()
	if state.Mode != "panel" || state.Panel == nil || state.Panel.Title != "Configure status line" || len(state.Panel.Rows) == 0 {
		t.Fatalf("status line panel = %#v", state.Panel)
	}
}
