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
	if modal.Mode != "legacy" || modal.LegacyView == "" {
		t.Fatalf("modal bridge state = %#v", modal)
	}
}
