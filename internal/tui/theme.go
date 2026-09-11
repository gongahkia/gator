package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	labelStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	keyStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
	panelStyle  = lipgloss.NewStyle().Padding(0, 1)
)

// applyTheme keeps terminal customization intentionally recognizable: every
// saved choice has a stable name and no hidden style file is evaluated.
func applyTheme(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "contrast":
		headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
		labelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
		dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
		keyStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("226"))
		errorStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
		okStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("46"))
		panelStyle = lipgloss.NewStyle().Padding(0, 1)
		return "contrast"
	case "mono":
		headerStyle = lipgloss.NewStyle().Bold(true)
		labelStyle = lipgloss.NewStyle().Bold(true)
		dimStyle = lipgloss.NewStyle().Faint(true)
		keyStyle = lipgloss.NewStyle().Bold(true)
		errorStyle = lipgloss.NewStyle().Bold(true)
		okStyle = lipgloss.NewStyle().Bold(true)
		panelStyle = lipgloss.NewStyle().Padding(0, 1)
		return "mono"
	default:
		headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
		labelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
		dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
		keyStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
		errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
		okStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
		panelStyle = lipgloss.NewStyle().Padding(0, 1)
		return "gator"
	}
}

// applyWorkSurfaceTheme restyles legacy controls embedded in the Work product
// without changing the standalone Code TUI's retained themes.
func applyWorkSurfaceTheme(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "contrast":
		headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("46"))
		labelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
		dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
		keyStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("46"))
		errorStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
		okStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("46"))
		panelStyle = lipgloss.NewStyle()
		return "contrast"
	case "mono":
		headerStyle = lipgloss.NewStyle().Bold(true)
		labelStyle = lipgloss.NewStyle().Bold(true)
		dimStyle = lipgloss.NewStyle().Faint(true)
		keyStyle = lipgloss.NewStyle().Bold(true)
		errorStyle = lipgloss.NewStyle().Bold(true)
		okStyle = lipgloss.NewStyle().Bold(true)
		panelStyle = lipgloss.NewStyle()
		return "mono"
	default:
		headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
		labelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
		dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
		keyStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
		errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
		okStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
		panelStyle = lipgloss.NewStyle()
		return "gator"
	}
}

// New creates a terminal model in task-composition mode.
