package tui

import (
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
)

// attachedTerminalKeyInput turns Bubble Tea's decoded key event back into the
// conventional bytes expected by a VT-compatible pseudo-terminal. It keeps
// the small, explicit raw-input surface separate from display rendering.
func attachedTerminalKeyInput(message tea.KeyMsg) ([]byte, bool) {
	if message.Type == tea.KeyRunes {
		return terminalMetaInput([]byte(string(message.Runes)), message.Alt), len(message.Runes) > 0
	}
	key := int(message.Type)
	if (key >= 0 && key <= 31) || key == 127 {
		return terminalMetaInput([]byte{byte(key)}, message.Alt), true
	}
	if final, modifier, found := terminalArrowKey(message); found {
		if modifier == 1 {
			return []byte("\x1b[" + string(final)), true
		}
		return []byte("\x1b[1;" + strconv.Itoa(modifier) + string(final)), true
	}
	if final, modifier, found := terminalHomeEndKey(message); found {
		if modifier == 1 {
			return []byte("\x1b[" + string(final)), true
		}
		return []byte("\x1b[1;" + strconv.Itoa(modifier) + string(final)), true
	}
	var input []byte
	switch message.Type {
	case tea.KeyShiftTab:
		input = []byte("\x1b[Z")
	case tea.KeyPgUp:
		input = []byte("\x1b[5~")
	case tea.KeyPgDown:
		input = []byte("\x1b[6~")
	case tea.KeyCtrlPgUp:
		input = []byte("\x1b[5;5~")
	case tea.KeyCtrlPgDown:
		input = []byte("\x1b[6;5~")
	case tea.KeyInsert:
		input = []byte("\x1b[2~")
	case tea.KeyDelete:
		input = []byte("\x1b[3~")
	case tea.KeySpace:
		input = []byte(" ")
	case tea.KeyF1:
		input = []byte("\x1bOP")
	case tea.KeyF2:
		input = []byte("\x1bOQ")
	case tea.KeyF3:
		input = []byte("\x1bOR")
	case tea.KeyF4:
		input = []byte("\x1bOS")
	case tea.KeyF5:
		input = []byte("\x1b[15~")
	case tea.KeyF6:
		input = []byte("\x1b[17~")
	case tea.KeyF7:
		input = []byte("\x1b[18~")
	case tea.KeyF8:
		input = []byte("\x1b[19~")
	case tea.KeyF9:
		input = []byte("\x1b[20~")
	case tea.KeyF10:
		input = []byte("\x1b[21~")
	case tea.KeyF11:
		input = []byte("\x1b[23~")
	case tea.KeyF12:
		input = []byte("\x1b[24~")
	default:
		return nil, false
	}
	return terminalMetaInput(input, message.Alt), true
}

func terminalArrowKey(message tea.KeyMsg) (rune, int, bool) {
	shift, control := false, false
	var final rune
	switch message.Type {
	case tea.KeyUp:
		final = 'A'
	case tea.KeyDown:
		final = 'B'
	case tea.KeyRight:
		final = 'C'
	case tea.KeyLeft:
		final = 'D'
	case tea.KeyCtrlUp:
		final, control = 'A', true
	case tea.KeyCtrlDown:
		final, control = 'B', true
	case tea.KeyCtrlRight:
		final, control = 'C', true
	case tea.KeyCtrlLeft:
		final, control = 'D', true
	case tea.KeyShiftUp:
		final, shift = 'A', true
	case tea.KeyShiftDown:
		final, shift = 'B', true
	case tea.KeyShiftRight:
		final, shift = 'C', true
	case tea.KeyShiftLeft:
		final, shift = 'D', true
	case tea.KeyCtrlShiftUp:
		final, shift, control = 'A', true, true
	case tea.KeyCtrlShiftDown:
		final, shift, control = 'B', true, true
	case tea.KeyCtrlShiftRight:
		final, shift, control = 'C', true, true
	case tea.KeyCtrlShiftLeft:
		final, shift, control = 'D', true, true
	default:
		return 0, 0, false
	}
	return final, terminalKeyModifier(shift, message.Alt, control), true
}

func terminalHomeEndKey(message tea.KeyMsg) (rune, int, bool) {
	shift, control := false, false
	var final rune
	switch message.Type {
	case tea.KeyHome:
		final = 'H'
	case tea.KeyEnd:
		final = 'F'
	case tea.KeyCtrlHome:
		final, control = 'H', true
	case tea.KeyCtrlEnd:
		final, control = 'F', true
	case tea.KeyShiftHome:
		final, shift = 'H', true
	case tea.KeyShiftEnd:
		final, shift = 'F', true
	case tea.KeyCtrlShiftHome:
		final, shift, control = 'H', true, true
	case tea.KeyCtrlShiftEnd:
		final, shift, control = 'F', true, true
	default:
		return 0, 0, false
	}
	return final, terminalKeyModifier(shift, message.Alt, control), true
}

func terminalKeyModifier(shift, alternate, control bool) int {
	modifier := 1
	if shift {
		modifier++
	}
	if alternate {
		modifier += 2
	}
	if control {
		modifier += 4
	}
	return modifier
}

func terminalMetaInput(input []byte, alternate bool) []byte {
	if !alternate {
		return input
	}
	return append([]byte{'\x1b'}, input...)
}
