package worktui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNativeFollowupFieldsKeepReviewAndRejectInvalidChoicesAtomically(t *testing.T) {
	newForm := func() Model {
		m := New(Config{CurrentFolder: t.TempDir(), ConnectorChoices: func() []string { return []string{"calendar"} }})
		m.newFollowup()
		return m
	}
	for name, fields := range map[string]map[int]string{
		"authority":        {0: "must not be applied", 8: "act"},
		"unknown source":   {0: "must not be applied", 3: `["other-account"]`},
		"duplicate source": {3: `["calendar","calendar"]`},
		"calendar range":   {5: "31"},
		"attempt range":    {9: "16,10,120"},
		"frequency":        {6: "1"},
	} {
		t.Run(name, func(t *testing.T) {
			m := newForm()
			before := m.followupForm.Draft
			next, cmd := m.ApplyBridgePanel(BridgePanelAction{ID: m.BridgeState().Panel.ID, Action: "key", Fields: fields}, tea.KeyMsg{Type: tea.KeyCtrlR})
			m = next.(Model)
			if cmd != nil || m.followupForm.Review || m.followupForm.Draft.Title != before.Title || m.followupForm.Draft.Mode != before.Mode || len(m.followupForm.Draft.ConnectorIDs) != 0 {
				t.Fatal("invalid native fields partly changed or saved an agreement")
			}
		})
	}
	m := newForm()
	next, cmd := m.ApplyBridgePanel(BridgePanelAction{ID: m.BridgeState().Panel.ID, Action: "key", Fields: map[int]string{
		0: "Watch the project", 3: `["calendar"]`, 5: "14", 6: "30", 8: "inspect", 9: "16,50000,120", 10: "6",
	}}, tea.KeyMsg{Type: tea.KeyCtrlR})
	m = next.(Model)
	d := m.followupForm.Draft
	if cmd != nil || !m.followupForm.Review || d.Title != "Watch the project" || d.IntervalMinutes != 30 || d.CalendarDays != 14 || d.Mode != "inspect" || d.MaxRequests != 16 || d.MaxTokens != 50000 || d.MaxSeconds != 120 || d.MaxChecksPerDay != 6 || strings.Join(d.ConnectorIDs, ",") != "calendar" {
		t.Fatal("native controls did not reach the existing complete agreement review")
	}
	if m.followupSaving {
		t.Fatal("native values saved without a separate confirmation")
	}
}

func TestDesktopListFilterAcceptsOnlyProjectedChoices(t *testing.T) {
	m := New(Config{})
	m.section = "history"
	next, _ := m.ApplyBridgePanel(BridgePanelAction{ID: m.BridgeState().Panel.ID, Action: "filter", Text: "delete"}, tea.KeyMsg{})
	m = next.(Model)
	if m.historyFilter != "" {
		t.Fatal("an unprojected filter changed the view")
	}
	next, _ = m.ApplyBridgePanel(BridgePanelAction{ID: m.BridgeState().Panel.ID, Action: "filter", Text: "failed"}, tea.KeyMsg{})
	if next.(Model).historyFilter != "failed" {
		t.Fatal("a projected filter was not selected directly")
	}
}
