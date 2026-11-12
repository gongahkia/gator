package workapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/uisnapshot"
)

func TestComputedControlPreviewRetainsFieldsAndRequiresFreshExactReview(t *testing.T) {
	saves, previews := 0, 0
	m := New(Config{BridgeMode: true, CurrentFolder: t.TempDir(), Controls: func(_ context.Context, family, action string, args []string) (ControlScreen, error) {
		if family != "check-timing" {
			t.Fatal(family)
		}
		if action == "preview" {
			previews++
			if args[2] != "09:30" {
				return ControlScreen{}, errors.New("use HH:MM")
			}
			return ControlScreen{Panel: uisnapshot.Panel{Kind: "confirmation", Title: "Computed review", ActionIdentity: strings.Join(args, ":")}, Actions: []ControlAction{{Key: "Enter", Label: "Save timing", Action: "save", Args: args}}}, nil
		}
		if action == "save" {
			saves++
		}
		return ControlScreen{Panel: uisnapshot.Panel{Kind: "controls", Title: "Check timing", ActionIdentity: "canonical-policy"}, Actions: []ControlAction{{Label: "Daily", Action: "preview", Args: []string{"job-one", "digest-one"}, Fields: []ControlField{{Label: "Time", Value: "09:00"}}, Preview: true}}}, nil
	}})
	m.conversation = "work-original"
	next, _ := m.Update(m.openControls("check-timing", []string{"job-one", "digest-one"})())
	m = next.(Model)
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	formID := m.BridgeState().Panel.ID
	next, cmd := m.ApplyBridgePanel(BridgePanelAction{ID: formID, Action: "key", Fields: map[int]string{0: "invalid"}}, tea.KeyMsg{Type: tea.KeyCtrlR})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.controlForm == nil || m.controlForm.Fields[0].Value != "invalid" || m.sectionNotice != "use HH:MM" || saves != 0 {
		t.Fatal("invalid preview discarded fields or saved")
	}
	next, cmd = m.ApplyBridgePanel(BridgePanelAction{ID: m.BridgeState().Panel.ID, Action: "key", Fields: map[int]string{0: "09:30"}}, tea.KeyMsg{Type: tea.KeyCtrlR})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	reviewID := m.BridgeState().Panel.ID
	if m.controlForm != nil || m.BridgeState().Panel.Kind != "confirmation" || saves != 0 {
		t.Fatal("preview saved or retained form")
	}
	next, cmd = m.ApplyBridgePanel(BridgePanelAction{ID: formID, Action: "key"}, tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil || saves != 0 {
		t.Fatal("old form identity saved the computed review")
	}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.controlForm == nil || m.controlForm.Fields[0].Value != "09:30" || m.controlReview || m.sectionNotice != "" {
		t.Fatal("Back did not restore editable values")
	}
	next, cmd = m.ApplyBridgePanel(BridgePanelAction{ID: reviewID, Action: "key"}, tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("cancelled review activated the restored form")
	}
	m, cmd = productKey(m, tea.KeyMsg{Type: tea.KeyCtrlR})
	next, _ = m.Update(cmd())
	m = next.(Model)
	m, cmd = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if saves != 1 || previews != 3 || m.controlPreviewReturn != nil || m.controlForm != nil || m.conversation != "work-original" || m.BridgeState().Panel.Navigation != "following" {
		t.Fatal("save lost owner or retained obsolete review")
	}
}
