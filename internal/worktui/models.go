package worktui

import tea "github.com/charmbracelet/bubbletea"

func (m Model) openModels() (tea.Model, tea.Cmd) {
	if m.running {
		m.status = "Finish or cancel the current work before managing models."
		return m, nil
	}
	panel, err := m.config.Models()
	if err != nil {
		m.status = "Open model management: " + err.Error()
		return m, nil
	}
	m.models = panel
	m.launcher = false
	next, resize := panel.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	m.models = next.(ModelPanel)
	return m, tea.Batch(resize, m.models.Init())
}

// Close releases a model operation if the terminal closes while its panel is open.
func (m Model) Close() {
	if m.models != nil {
		m.models.Close()
	}
	if m.cancel != nil {
		m.cancel()
	}
}
