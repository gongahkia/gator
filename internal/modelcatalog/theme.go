package modelcatalog

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	labelStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	keyStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
)

func applyWorkSurfaceTheme(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "contrast":
		headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("46"))
		labelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
		dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
		keyStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("46"))
		errorStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
		okStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("46"))
		return "contrast"
	case "mono":
		headerStyle = lipgloss.NewStyle().Bold(true)
		labelStyle = lipgloss.NewStyle().Bold(true)
		dimStyle = lipgloss.NewStyle().Faint(true)
		keyStyle = lipgloss.NewStyle().Bold(true)
		errorStyle = lipgloss.NewStyle().Bold(true)
		okStyle = lipgloss.NewStyle().Bold(true)
		return "mono"
	default:
		headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
		labelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
		dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
		keyStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
		errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
		okStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
		return "gator"
	}
}
