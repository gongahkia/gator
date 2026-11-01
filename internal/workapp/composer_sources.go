package workapp

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"
)

// ComposerSource is display metadata. Binding fingerprints host-only account
// configuration/credentials and never leaves Go, including through this struct.
type ComposerSource struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Service     string `json:"service"`
	Status      string `json:"status"`
	Access      string `json:"access"`
	CheckedAt   string `json:"checked_at,omitempty"`
	Unavailable string `json:"unavailable,omitempty"`
	Binding     string `json:"-"`
}
type ComposerSources struct {
	ID       string           `json:"id"`
	Choices  []ComposerSource `json:"choices"`
	Selected []string         `json:"selected"`
	Issue    string           `json:"issue,omitempty"`
	Disabled string           `json:"disabled,omitempty"`
}
type SourceAction struct {
	ID        string   `json:"id"`
	RequestID string   `json:"request_id"`
	Action    string   `json:"action"`
	IDs       []string `json:"ids,omitempty"`
}

func (m *Model) refreshComposerSources() {
	m.sourceChoices, m.sourcesIssue = nil, ""
	if m.config.ComposerSources == nil {
		return
	}
	choices, err := m.config.ComposerSources()
	if err != nil {
		m.sourcesIssue = "Sources could not load. Open Connections or refresh to try again."
		return
	}
	if len(choices) > 128 {
		choices = choices[:128]
		m.sourcesIssue = "Showing 128 configured sources. Manage connections to review the rest."
	}
	m.sourceChoices = append([]ComposerSource{}, choices...)
}
func (m Model) composerSourceSettings() ComposerSources {
	result := ComposerSources{Choices: append([]ComposerSource{}, m.sourceChoices...), Selected: append([]string{}, m.options.ConnectorIDs...), Issue: m.sourcesIssue}
	if m.section != "" || m.launcher || m.models != nil || m.controlBusy || m.bridgeExternal != nil || m.pendingBundleAction != nil {
		result.Disabled = "Return to the conversation before selecting sources."
	}
	bindings := make([]string, 0, len(m.sourceChoices))
	for _, choice := range m.sourceChoices {
		bindings = append(bindings, choice.Binding)
	}
	data, _ := json.Marshal([]any{m.source, m.conversation, result, bindings})
	result.ID = fmt.Sprintf("%x", sha256.Sum256(data))
	return result
}

// ApplySourceSelection changes subsequent request options only. Running work and
// already-admitted queue entries own immutable copies of their original options.
func (m Model) ApplySourceSelection(action SourceAction) (tea.Model, tea.Cmd) {
	m.sourcesResult = &ComposerResult{RequestID: action.RequestID}
	fail := func(text string) (tea.Model, tea.Cmd) { m.sourcesResult.Error = text; return m, nil }
	if action.RequestID == "" || len(action.RequestID) > 64 {
		return fail("Invalid source selection request.")
	}
	m.refreshComposerSources()
	current := m.composerSourceSettings()
	if current.Disabled != "" {
		return fail(current.Disabled)
	}
	if action.Action == "refresh" {
		if len(action.IDs) > 0 {
			return fail("Refresh cannot select sources.")
		}
		return m, nil
	}
	if action.Action != "select" || action.ID != current.ID || action.IDs == nil || len(action.IDs) > 32 {
		return fail("Sources changed. Review the current list and try again.")
	}
	seen := map[string]bool{}
	for _, id := range action.IDs {
		found := false
		for _, choice := range current.Choices {
			if choice.ID == id && choice.Unavailable == "" {
				found = true
			}
		}
		if !found || seen[id] {
			return fail("A selected source is unavailable, removed or repeated. Review Connections.")
		}
		seen[id] = true
	}
	if m.conversation != "" {
		if m.config.SelectComposerSources == nil {
			return fail("Source selection is unavailable.")
		}
		if err := m.config.SelectComposerSources(m.conversation, slices.Clone(m.options.ConnectorIDs), slices.Clone(action.IDs)); err != nil {
			return fail("Could not save sources: " + singleLine(err.Error()))
		}
	}
	m.options.ConnectorIDs = append([]string{}, action.IDs...)
	return m, nil
}
