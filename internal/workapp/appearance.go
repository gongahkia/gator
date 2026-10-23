package workapp

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/uisnapshot"
)

var appearanceThemes = []struct{ id, label, detail string }{
	{"gator", "Dark", "Gator’s dark canvas and soft green accent."},
	{"light", "Light", "A soft white canvas and deep green accent."},
	{"contrast", "High contrast", "Brighter text and stronger outlines on a dark canvas."},
	{"mono", "Monochrome", "A dark canvas with a neutral accent."},
}

func (m Model) appearancePanel() uisnapshot.Panel {
	panel := uisnapshot.Panel{Kind: "appearance", Title: "Appearance", Subtitle: "Choose how Gator looks on this device.", Selected: m.selected, Notice: m.sectionNotice, Hints: []uisnapshot.Hint{{Shortcut: "↑/↓", Action: "choose"}, {Shortcut: "Enter", Action: "use"}, {Shortcut: "Esc", Action: "back"}}}
	for _, theme := range appearanceThemes {
		state := ""
		if theme.id == m.theme {
			state = "current"
		}
		panel.Rows = append(panel.Rows, uisnapshot.Row{Label: theme.label, Detail: theme.detail, State: state, Checked: theme.id == m.theme})
	}
	return panel
}
func (m Model) updateAppearanceKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.section = ""
	case "up":
		if m.selected > 0 {
			m.selected--
		}
	case "down":
		if m.selected < len(appearanceThemes)-1 {
			m.selected++
		}
	case "enter":
		if m.selected < 0 || m.selected >= len(appearanceThemes) || m.config.SetTheme == nil {
			return m, nil
		}
		selected := appearanceThemes[m.selected]
		if err := m.config.SetTheme(selected.id); err != nil {
			m.sectionNotice = "Couldn’t save appearance: " + err.Error()
			return m, nil
		}
		m.theme = selected.id
		m.sectionNotice = ""
	}
	return m, nil
}
