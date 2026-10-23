package workapp

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAppearanceSelectionPersistsAndKeepsOldThemeOnFailure(t *testing.T) {
	var saved string
	var saveErr error
	m := New(Config{SetTheme: func(theme string) error {
		if saveErr != nil {
			return saveErr
		}
		saved = theme
		return nil
	}})
	next, _ := m.NavigateBridge("/appearance")
	m = next.(Model)
	panel := m.BridgeState().Panel
	if panel == nil || panel.Title != "Appearance" || len(panel.Rows) != 4 {
		t.Fatal("appearance settings unavailable", panel)
	}
	row := 1
	next, _ = m.ApplyBridgePanel(BridgePanelAction{ID: panel.ID, Action: "activate", Row: &row}, tea.KeyMsg{})
	m = next.(Model)
	if saved != "light" || m.BridgeState().Theme != "light" || !m.BridgeState().Panel.Rows[1].Checked {
		t.Fatal("light mode not saved or projected")
	}
	saveErr = errors.New("disk unavailable")
	row = 0
	panel = m.BridgeState().Panel
	next, _ = m.ApplyBridgePanel(BridgePanelAction{ID: panel.ID, Action: "activate", Row: &row}, tea.KeyMsg{})
	m = next.(Model)
	if m.theme != "light" || saved != "light" || m.BridgeState().Panel.Notice == "" {
		t.Fatal("failed save changed visible preference")
	}
}
func TestAppearanceRefreshesAcrossCachedChats(t *testing.T) {
	m := New(Config{ManagedSessions: true, Theme: "gator", ReadTheme: func() (string, error) { return "light", nil }})
	m = m.FocusBridgeSession(true)
	if m.BridgeState().Theme != "light" {
		t.Fatal("cached chat restored an earlier theme")
	}
}
