package workapp

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ComposerModel contains display metadata, never endpoint or credential values.
type ComposerModel struct {
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Label       string `json:"label"`
	Unavailable string `json:"unavailable,omitempty"`
}

type ComposerSettings struct {
	ID       string          `json:"id"`
	Effort   string          `json:"effort"`
	Models   []ComposerModel `json:"models"`
	Issue    string          `json:"issue,omitempty"`
	Disabled string          `json:"disabled,omitempty"`
}

type ComposerAction struct {
	ID        string `json:"id"`
	RequestID string `json:"request_id"`
	Action    string `json:"action"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	Effort    string `json:"effort,omitempty"`
}

type ComposerResult struct {
	RequestID string `json:"request_id"`
	Error     string `json:"error,omitempty"`
}

func (m Model) composerSettings() ComposerSettings {
	state := ComposerSettings{Effort: effortName(m.options.MaxSteps), Models: append([]ComposerModel{}, m.composerModels...), Issue: m.composerModelsIssue}
	if m.running || len(m.queue) > 0 {
		state.Disabled = "Available after current and queued work finishes."
	} else if m.models != nil || m.launcher || m.section != "" || m.bridgeExternal != nil || m.pendingBundleAction != nil || m.controlBusy || m.recallBusy {
		state.Disabled = "Return to the conversation to change these settings."
	}
	data, _ := json.Marshal([]any{m.source, m.conversation, m.modelStatus.Provider, m.modelStatus.Model, state.Effort, state.Models, state.Issue, state.Disabled})
	state.ID = fmt.Sprintf("%x", sha256.Sum256(data))
	return state
}

// ApplyComposerSettings reuses canonical defaults and task budgets. It never
// alters a running request, queued work, conversation identity or draft.
func (m Model) ApplyComposerSettings(action ComposerAction) (tea.Model, tea.Cmd) {
	m.composerResult = &ComposerResult{RequestID: action.RequestID}
	fail := func(text string) (tea.Model, tea.Cmd) { m.composerResult.Error = text; return m, nil }
	state := m.composerSettings()
	if action.RequestID == "" || len(action.RequestID) > 64 || action.ID != state.ID {
		return fail("These settings changed. Review the current selection and try again.")
	}
	if state.Disabled != "" {
		return fail(state.Disabled)
	}
	switch action.Action {
	case "model":
		if action.Effort != "" || m.config.SelectComposerModel == nil {
			return fail("Model selection is unavailable.")
		}
		// Re-read configuration at the action boundary so removed providers and
		// credentials cannot be selected using an old display snapshot.
		m.refreshModelStatus()
		if m.composerSettings().ID != action.ID {
			return fail("Available models changed. Review the current list and try again.")
		}
		found := false
		for _, choice := range m.composerModels {
			if choice.Provider == action.Provider && choice.Model == action.Model && choice.Unavailable == "" {
				found = true
				break
			}
		}
		if !found {
			return fail("This model is no longer available. Open Manage models to configure it.")
		}
		if err := m.config.SelectComposerModel(action.Provider, action.Model); err != nil {
			return fail("Could not select model: " + singleLine(err.Error()))
		}
		m.refreshModelStatus()
		m.firstRun = m.modelStatus.Provider == ""
	case "effort":
		if action.Provider != "" || action.Model != "" {
			return fail("Invalid work effort selection.")
		}
		if err := m.setWorkEffort(action.Effort); err != nil {
			return fail(err.Error())
		}
	default:
		return fail("Unknown composer setting.")
	}
	return m, nil
}

func (m *Model) setWorkEffort(value string) error {
	switch strings.ToLower(value) {
	case "low":
		m.options.MaxSteps, m.options.Code.MaxSteps = 12, 8
	case "standard":
		m.options.MaxSteps, m.options.Code.MaxSteps = 24, 16
	case "high":
		m.options.MaxSteps, m.options.Code.MaxSteps = 48, 32
	default:
		return fmt.Errorf("unknown effort %q; use low, standard, or high", value)
	}
	return nil
}
