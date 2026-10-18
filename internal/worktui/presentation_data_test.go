package worktui

import (
	"strings"
	"testing"
)

// snapshotText exposes the display-independent state used by the React UI.
// Tests use it to check content and workflow state without coupling to layout.
func (m Model) snapshotText() string {
	state := m.BridgeState()
	var parts []string
	parts = append(parts, state.Title, state.Source, state.Status, state.Input)
	if state.StatusLineEnabled {
		parts = append(parts, state.StatusLine...)
	}
	for _, action := range state.Actions {
		parts = append(parts, action.Label, action.Command)
	}
	for _, message := range state.Messages {
		parts = append(parts, message.Role, message.Text)
		if message.Bundle != nil {
			parts = append(parts, formatBundleSummary(*message.Bundle))
		}
	}
	if panel := state.Panel; panel != nil {
		parts = append(parts, panel.Kind, panel.Title, panel.Subtitle, panel.Query, panel.Notice, panel.Footer)
		for _, row := range panel.Rows {
			parts = append(parts, row.Label, row.Detail, row.State)
		}
		for _, block := range panel.Blocks {
			parts = append(parts, block.Title)
			parts = append(parts, block.Lines...)
		}
	}
	return strings.Join(parts, "\n")
}

func TestBridgeKeepsUserAndGatorTurnsDistinct(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	model.messages = []message{{role: "You", text: "Draft a brief"}, {role: "Gator", text: "Here is the brief."}}
	state := model.BridgeState()
	if len(state.Messages) != 2 || state.Messages[0].Role != "You" || state.Messages[0].Text != "Draft a brief" || state.Messages[1].Role != "Gator" || state.Messages[1].Text != "Here is the brief." {
		t.Fatalf("bridge messages = %#v", state.Messages)
	}
}
