package workapp

import tea "github.com/charmbracelet/bubbletea"

func (m Model) openModels() (tea.Model, tea.Cmd) {
	if m.recallBusy {
		m.sectionNotice = "Finish or cancel the recall model download before managing models."
		return m, nil
	}
	if m.running {
		m.status = "Finish or cancel the current work before managing models."
		return m, nil
	}
	if m.config.Models == nil {
		m.status = "Model management is unavailable in this build."
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
	return m, tea.Sequence(tea.ClearScreen, resize, m.models.Init())
}

// Close releases a model operation if the terminal closes while its panel is open.
func (m Model) Close() {
	if m.controlCancel != nil {
		m.controlCancel()
	}
	if m.recallCancel != nil {
		m.recallCancel()
	}
	if m.models != nil {
		m.models.Close()
	}
	if m.cancel != nil {
		m.cancel()
	}
}

// closeModelPanel reconciles selection on every exit, including desktop navigation.
// Returning from setup restores a pending draft without submitting it.
func (m *Model) closeModelPanel() {
	m.models.Close()
	m.models = nil
	if m.config.SelectedModel != nil {
		provider, model, err := m.config.SelectedModel()
		if err != nil {
			m.status = "Read model selection: " + err.Error()
		} else {
			m.firstRun = provider == ""
			m.status = "Selected model: " + provider + " / " + model
			if m.firstRun {
				m.status = "Choose a model with /model before starting work."
			}
		}
	}
	m.refreshModelStatus()
	if m.pendingPrompt != "" {
		m.input, m.pendingPrompt = m.pendingPrompt, ""
	}
}
