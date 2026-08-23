package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAttachedTerminalKeyInputUsesConventionalTerminalSequences(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyMsg
		want string
	}{
		{name: "runes", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("é")}, want: "é"},
		{name: "alt rune", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x"), Alt: true}, want: "\x1bx"},
		{name: "control", key: tea.KeyMsg{Type: tea.KeyCtrlC}, want: "\x03"},
		{name: "up", key: tea.KeyMsg{Type: tea.KeyUp}, want: "\x1b[A"},
		{name: "alt control right", key: tea.KeyMsg{Type: tea.KeyCtrlRight, Alt: true}, want: "\x1b[1;7C"},
		{name: "shift end", key: tea.KeyMsg{Type: tea.KeyShiftEnd}, want: "\x1b[1;2F"},
		{name: "delete", key: tea.KeyMsg{Type: tea.KeyDelete}, want: "\x1b[3~"},
		{name: "function", key: tea.KeyMsg{Type: tea.KeyF5}, want: "\x1b[15~"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, found := attachedTerminalKeyInput(test.key)
			if !found || string(got) != test.want {
				t.Fatalf("attachedTerminalKeyInput(%#v) = %q, %t; want %q, true", test.key, got, found, test.want)
			}
		})
	}
	if got, found := attachedTerminalKeyInput(tea.KeyMsg{Type: tea.KeyF13}); found || got != nil {
		t.Fatalf("unsupported F13 input = %q, %t", got, found)
	}
}
