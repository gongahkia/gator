package tui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// effortLevel is an intent-level control. It deliberately changes Gator's
// bounded agent-turn budget rather than silently substituting a cloud model;
// /model remains the explicit expert control for provider, credentials, and
// exact model selection.
type effortLevel uint8

const (
	effortFast effortLevel = iota
	effortStandard
	effortThorough
	effortMaximum
)

func parseEffort(value string) effortLevel {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "fast":
		return effortFast
	case "thorough":
		return effortThorough
	case "maximum":
		return effortMaximum
	default:
		return effortStandard
	}
}

func (e effortLevel) label() string {
	return [...]string{"Fast", "Standard", "Thorough", "Maximum"}[e]
}

func (e effortLevel) description() string {
	return [...]string{
		"fewer agent turns for a small, well-scoped change",
		"the normal balanced turn budget",
		"more room for investigation, implementation, and verification",
		"the largest bounded turn budget for difficult work",
	}[e]
}

func (e effortLevel) maxSteps(base int) int {
	if base < 1 {
		base = defaultMaxSteps
	}
	switch e {
	case effortFast:
		return max(1, base/2)
	case effortThorough:
		return max(base+base/2, 36)
	case effortMaximum:
		return max(base*2, 48)
	default:
		return base
	}
}

func (m Model) openEffortPicker() (tea.Model, tea.Cmd) {
	m.effortIndex = int(m.effort)
	m.screen = effortScreen
	m.notice = notice{text: "Choose how much Gator should investigate and verify. /model remains available for exact model control.", kind: noticeInfo}
	return m, nil
}

func (m Model) updateEffort(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc", "q":
		m.screen = composeScreen
		return m, m.focusField()
	case "up", "k", "ctrl+p":
		m.effortIndex = (m.effortIndex - 1 + 4) % 4
	case "down", "j", "ctrl+n":
		m.effortIndex = (m.effortIndex + 1) % 4
	case "1", "2", "3", "4":
		m.effortIndex = int(message.String()[0] - '1')
		fallthrough
	case "enter":
		m.effort = effortLevel(m.effortIndex)
		m.screen = composeScreen
		m.notice = notice{text: m.effort.label() + " effort selected: " + m.effort.description() + ".", kind: noticeSuccess}
		return m, m.focusField()
	case "m":
		return m.openModelCatalog()
	}
	return m, nil
}

func (m Model) effortView() string {
	levels := []effortLevel{effortFast, effortStandard, effortThorough, effortMaximum}
	lines := make([]string, 0, len(levels))
	for index, level := range levels {
		prefix := "  "
		if index == m.effortIndex {
			prefix = "> "
		}
		line := prefix + level.label() + " · " + level.description() + " · up to " + strconv.Itoa(level.maxSteps(m.config.MaxSteps)) + " turns"
		if index == m.effortIndex {
			line = keyStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return m.header("effort") + "\n" + m.panel(strings.Join(lines, "\n")) + "\n" + m.noticeView() + "\n" + m.footer("up/down choose", "enter apply", "m /model", "esc composer")
}
