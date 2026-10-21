package worktui

import (
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/workhome"
)

func TestBridgeUsesLiveCommandAndMessageState(t *testing.T) {
	model := New(Config{CurrentFolder: t.TempDir(), Attention: workhome.Attention{UnreadResults: 1}})
	initial := model.BridgeState()
	if initial.Mode != "chat" || !initial.Home || len(initial.Actions) == 0 || initial.Actions[0].Command != "/inbox" {
		t.Fatalf("initial bridge state = %#v", initial)
	}
	model.conversation = "work-retained"
	if model.BridgeState().ConversationID != "work-retained" {
		t.Fatal("bridge lost the durable conversation identity")
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

func TestBridgeProjectsThemeChangesToReact(t *testing.T) {
	model := New(Config{CurrentFolder: t.TempDir(), Theme: "mono", SetTheme: func(string) error { return nil }})
	if got := model.BridgeState().Theme; got != "mono" {
		t.Fatalf("initial theme = %q", got)
	}
	next, _ := model.SubmitText("/theme contrast")
	if got := next.(Model).BridgeState().Theme; got != "contrast" {
		t.Fatalf("changed theme = %q", got)
	}
}

func TestDoctorResultIsMarkedForReactReport(t *testing.T) {
	model := New(Config{CurrentFolder: t.TempDir(), Inspect: func(topic string) (string, error) {
		if topic != "doctor" {
			t.Fatalf("unexpected inspection topic %q", topic)
		}
		return "Repository: detected", nil
	}})
	next, _ := model.SubmitText("/doctor")
	messages := next.(Model).BridgeState().Messages
	if len(messages) != 1 || messages[0].Role != "Doctor" || messages[0].Text != "Repository: detected" {
		t.Fatalf("doctor bridge message = %#v", messages)
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
		{command: "/learnings", title: "Saved guidance"},
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
	if state.Mode != "panel" || state.Panel == nil || state.Panel.Title != "Terminal status line" || len(state.Panel.Rows) == 0 {
		t.Fatalf("status line panel = %#v", state.Panel)
	}
}

func TestBridgeProjectsConfiguredStatusLineAndModelPanel(t *testing.T) {
	model := New(Config{CurrentFolder: t.TempDir()})
	model.statusLineEnabled = true
	model.statusLine = []string{"send", "current-dir"}
	state := model.BridgeState()
	if !state.StatusLineEnabled || len(state.StatusLine) != 2 || state.StatusLine[0] != "enter send" {
		t.Fatalf("configured status line = %#v", state.StatusLine)
	}
	model.models = &testModelPanel{}
	state = model.BridgeState()
	if state.Mode != "panel" || state.Panel == nil || state.Panel.Title != "Models" {
		t.Fatalf("model panel snapshot = %#v", state.Panel)
	}
}

func TestBridgeFlagsStoppedSupervisorForEnabledJobs(t *testing.T) {
	model := New(Config{CurrentFolder: t.TempDir(), Jobs: []jobs.Definition{{Enabled: true}}})
	if !model.BridgeState().SupervisorNeedsStart {
		t.Fatal("bridge hid stopped supervisor warning")
	}
	model.config.SupervisorRunning = true
	if model.BridgeState().SupervisorNeedsStart {
		t.Fatal("bridge showed stopped supervisor warning while running")
	}
}
