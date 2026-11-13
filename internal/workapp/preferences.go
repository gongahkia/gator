package workapp

import (
	"fmt"
	"slices"

	"github.com/gongahkia/gator/internal/worksession"
)

func (m *Model) refreshConversationPreferences() error {
	if !m.retainsRequests() {
		return nil
	}
	p, err := m.config.SessionStore.LoadPreferences(m.conversation)
	if err != nil {
		m.preferencesIssue = "Could not load saved request preferences: " + err.Error()
		return fmt.Errorf("%s", m.preferencesIssue)
	}
	m.preferencesIssue = ""
	if p.ConnectorIDs != nil {
		m.options.ConnectorIDs = slices.Clone(*p.ConnectorIDs)
	}
	if p.WorkEffort != nil {
		budget, err := worksession.WorkEffortBudget(*p.WorkEffort)
		if err != nil {
			return err
		}
		m.options.MaxSteps, m.options.Code.MaxSteps = budget.MaxSteps, budget.CodeMaxSteps
	}
	return nil
}

// RefreshDesktopPreferences handles the existing private host notification for
// saved preferences. It refreshes future defaults without changing live work.
func (m Model) RefreshDesktopPreferences() Model {
	if m.config.ReadDesktop != nil {
		if preferences, err := m.config.ReadDesktop(); err == nil {
			m.desktop = preferences
		} else {
			m.bridgeNotice = "Could not reload desktop preferences: " + err.Error()
		}
	}
	m.refreshTerminalPreferences()
	if err := m.refreshConversationPreferences(); err != nil {
		m.bridgeNotice = err.Error()
	}
	// This is the model for future submissions. Active operations, retained
	// queues and reviewed plans already own their immutable RunOptions.
	// Embedders can supply an already configured Run callback without model
	// settings. Absence of a settings reader does not mean setup is missing.
	if m.config.ModelStatus != nil || m.config.SelectedModel != nil {
		m.refreshModelStatus()
		m.firstRun = m.modelStatus.Provider == ""
	}
	return m
}
