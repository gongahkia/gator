package workapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestDesktopPanelClickCannotActivateChangedView(t *testing.T) {
	m := New(Config{CurrentFolder: t.TempDir()})
	m.openCommandPalette()
	old := m.BridgeState().Panel
	row := 0
	next, _ := m.ApplyBridgePanel(BridgePanelAction{ID: old.ID, Action: "query", Text: "help"}, tea.KeyMsg{})
	m = next.(Model)
	if m.BridgeState().Panel.ID == old.ID {
		t.Fatal("changing rows did not invalidate the old view")
	}
	next, command := m.ApplyBridgePanel(BridgePanelAction{ID: old.ID, Action: "activate", Row: &row}, tea.KeyMsg{})
	m = next.(Model)
	if command != nil || !m.launcher || len(m.messages) != 0 || !strings.Contains(m.status, "view changed") {
		t.Fatal("stale click activated a new row")
	}
	next, _ = m.ApplyBridgePanel(BridgePanelAction{ID: m.BridgeState().Panel.ID, Action: "activate", Row: &row}, tea.KeyMsg{})
	if next.(Model).launcher || len(next.(Model).messages) != 1 {
		t.Fatal("current view did not route the command through Go")
	}
}

func TestDesktopFormKeepsSecretsPrivateAndRequiresSeparateReview(t *testing.T) {
	saved := 0
	m := New(Config{Controls: func(_ context.Context, _, action string, args []string) (ControlScreen, error) {
		if action == "save" {
			saved++
			if len(args) != 1 || args[0] != "desktop-secret" {
				t.Fatal("wrong reviewed secret")
			}
		}
		return ControlScreen{Panel: uisnapshot.Panel{Title: "Test", Kind: "recovery"}, Actions: []ControlAction{{Key: "T", Label: "Credential", Action: "save", Fields: []ControlField{{Label: "Token", Secret: true}}, Review: true}}}, nil
	}})
	next, _ := m.Update(m.openControls("recovery", nil)())
	m = next.(Model)
	row := 0
	next, _ = m.ApplyBridgePanel(BridgePanelAction{ID: m.BridgeState().Panel.ID, Action: "activate", Row: &row}, tea.KeyMsg{})
	m = next.(Model)
	panel := m.BridgeState().Panel
	if !panel.Rows[0].Input.Secret || panel.Rows[0].Input.Value != "" {
		t.Fatal("credential form was not projected as a private replacement")
	}
	next, command := m.ApplyBridgePanel(BridgePanelAction{ID: panel.ID, Action: "key", Fields: map[int]string{0: "desktop-secret"}}, tea.KeyMsg{Type: tea.KeyCtrlR})
	m = next.(Model)
	encoded, _ := json.Marshal(m.BridgeState())
	if saved != 0 || command != nil || m.BridgeState().Panel.Kind != "confirmation" || strings.Contains(string(encoded), "desktop-secret") {
		t.Fatal("form skipped review or leaked its secret")
	}
	next, command = m.ApplyBridgePanel(BridgePanelAction{ID: panel.ID, Action: "key"}, tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil || saved != 0 {
		t.Fatal("old form identity confirmed a later review")
	}
	m = next.(Model)
	next, command = m.ApplyBridgePanel(BridgePanelAction{ID: m.BridgeState().Panel.ID, Action: "key"}, tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil || saved != 0 {
		t.Fatal("review did not schedule its existing Go action")
	}
	_, _ = next.(Model).Update(command())
	if saved != 1 {
		t.Fatal("reviewed action did not save exactly once")
	}
}

func TestDesktopRejectsFormInjectionBeforeAnyFieldChanges(t *testing.T) {
	m := New(Config{})
	m.section = "controls"
	m.controlForm = &ControlAction{Fields: []ControlField{{Label: "Name", Value: "before"}}}
	for _, fields := range []map[int]string{{0: "changed", 1: "unknown"}, {0: strings.Repeat("x", 8193)}, {0: "bad\nvalue"}} {
		next, _ := m.ApplyBridgePanel(BridgePanelAction{ID: m.BridgeState().Panel.ID, Action: "key", Fields: fields}, tea.KeyMsg{Type: tea.KeyCtrlR})
		m = next.(Model)
		if m.controlForm.Fields[0].Value != "before" || m.controlReview {
			t.Fatal("invalid form partially changed the draft")
		}
	}
}

func TestDesktopApprovalCannotRespondToAnotherLiveRequest(t *testing.T) {
	m := New(Config{})
	m.interaction = &workrun.Interaction{ID: 17, Kind: "test"}
	if m.BridgeState().PendingApproval.ID != 17 {
		t.Fatal("live request identity was not projected")
	}
	next, command := m.RespondBridgeApproval(16, true)
	m = next.(Model)
	if command != nil || m.interaction.ID != 17 || !strings.Contains(m.status, "no longer pending") {
		t.Fatal("stale approval reached the live request")
	}
}

func TestDesktopNavigationDoesNotAcceptPromptsOrInterruptWork(t *testing.T) {
	m := New(Config{})
	m.openCommandPalette()
	next, _ := m.NavigateBridge("Please run something")
	if !next.(Model).launcher || len(next.(Model).messages) != 0 {
		t.Fatal("navigation accepted an arbitrary prompt")
	}
	m.running = true
	next, _ = m.NavigateBridge("/new")
	if !next.(Model).running || !next.(Model).launcher {
		t.Fatal("navigation discarded active Work")
	}
	next, _ = m.NavigateBridge("/chat")
	if !next.(Model).running || next.(Model).launcher {
		t.Fatal("returning to the live conversation stopped Work or kept the view open")
	}
}
