package workapp

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/config"
)

type TerminalPreferencesState struct {
	ID          string                     `json:"id"`
	Initialized bool                       `json:"initialized"`
	Values      config.TerminalPreferences `json:"values"`
	Issue       string                     `json:"issue,omitempty"`
}

type TerminalPreferencesAction struct {
	ID        string                           `json:"id"`
	RequestID string                           `json:"request_id"`
	Action    string                           `json:"action"`
	Patch     *config.TerminalPreferencesPatch `json:"patch,omitempty"`
	Legacy    *config.TerminalPreferences      `json:"legacy,omitempty"`
}

type TerminalPreferencesResult struct {
	RequestID string                      `json:"request_id"`
	Values    *config.TerminalPreferences `json:"values,omitempty"`
	Error     string                      `json:"error,omitempty"`
}

func (m *Model) refreshTerminalPreferences() {
	state := TerminalPreferencesState{Values: config.DefaultTerminalPreferences()}
	if m.config.ReadTerminal == nil {
		state.Issue = "Terminal preferences are unavailable in this runtime."
	} else {
		values, initialized, err := m.config.ReadTerminal()
		if err != nil {
			state.Issue = "Terminal preferences could not be read. Inspect configuration and retry."
		} else {
			state.Values, state.Initialized = values, initialized
			m.theme = normalizeTheme(values.Theme)
		}
	}
	data, _ := json.Marshal([]any{state.Values, state.Initialized, state.Issue})
	state.ID = fmt.Sprintf("%x", sha256.Sum256(data))
	m.terminal = state
}

// Display changes do not change a task's model, scope, approvals or draft.
func (m Model) ApplyTerminalPreferences(action TerminalPreferencesAction) (tea.Model, tea.Cmd) {
	m.terminalResult = &TerminalPreferencesResult{RequestID: action.RequestID}
	fail := func(text string) (tea.Model, tea.Cmd) { m.terminalResult.Error = text; return m, nil }
	before := m.terminal
	if action.RequestID == "" || len(action.RequestID) > 64 {
		return fail("Invalid terminal preference request.")
	}
	if before.Issue != "" {
		return fail(before.Issue)
	}
	if action.ID != before.ID && !(action.Action == "migrate" && before.Initialized) {
		return fail("Terminal preferences changed. Inspect them and try again.")
	}
	var values config.TerminalPreferences
	var err error
	switch action.Action {
	case "migrate":
		if action.Legacy == nil || action.Patch != nil || m.config.ImportTerminal == nil {
			return fail("Invalid terminal preference import.")
		}
		values, err = m.config.ImportTerminal(*action.Legacy)
	case "configure":
		if action.Patch == nil || action.Legacy != nil || m.config.SaveTerminal == nil {
			return fail("Invalid terminal preference change.")
		}
		values, err = m.config.SaveTerminal(*action.Patch, &before.Values)
	default:
		return fail("Unknown terminal preference action.")
	}
	m.refreshTerminalPreferences()
	if err != nil {
		return fail("Could not save terminal preferences: " + singleLine(err.Error()))
	}
	m.terminalResult.Values = &values
	return m, nil
}
