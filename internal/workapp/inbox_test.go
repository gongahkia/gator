package workapp

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/inbox"
)

func TestInboxOpeningIsReadOnlyAndExplicitActionsUseCurrentView(t *testing.T) {
	entry := inbox.Entry{Kind: "job", ID: "result-one", Title: "Project update", Summary: "An exact retained update.", Status: "completed", CreatedAt: time.Now()}
	var calls []string
	m := New(Config{Inbox: []inbox.Entry{entry}, RefreshInbox: func() ([]inbox.Entry, error) { return []inbox.Entry{entry}, nil },
		InboxAction: func(args []string) (string, error) {
			calls = append(calls, strings.Join(args, " "))
			if args[0] == "read" {
				entry.ReadAt = time.Now()
			} else {
				entry.Rating = args[2]
			}
			return "saved", nil
		}})
	m.section = "inbox"
	list := m.BridgeState().Panel
	row := 0
	next, _ := m.ApplyBridgePanel(BridgePanelAction{ID: list.ID, Action: "activate", Row: &row}, tea.KeyMsg{})
	m = next.(Model)
	if m.inboxDetail != entry.ID || len(calls) != 0 || m.BridgeState().Panel.Kind != "inbox-detail" {
		t.Fatal("opening a result failed or changed its read/rating state")
	}
	detail := m.BridgeState().Panel
	next, _ = m.ApplyBridgePanel(BridgePanelAction{ID: list.ID, Action: "key"}, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m = next.(Model)
	if len(calls) != 0 {
		t.Fatal("stale list action mutated a result")
	}
	next, _ = m.ApplyBridgePanel(BridgePanelAction{ID: detail.ID, Action: "key"}, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m = next.(Model)
	if len(calls) != 1 || calls[0] != "read result-one" || m.selectedInboxEntry().ReadAt.IsZero() {
		t.Fatal("explicit read action did not use the canonical Inbox service")
	}
	next, _ = m.ApplyBridgePanel(BridgePanelAction{ID: m.BridgeState().Panel.ID, Action: "key"}, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = next.(Model)
	if len(calls) != 2 || calls[1] != "rate result-one useful" {
		t.Fatal("rating used the wrong entry or operation")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if next.(Model).section != "inbox" || next.(Model).inboxDetail != "" {
		t.Fatal("back did not return to Inbox")
	}
}

func TestInboxFeedbackViewBindsRevisionEvenWhenAssessmentLooksIdentical(t *testing.T) {
	entry := inbox.Entry{ID: "result-one", Kind: "job", Title: "Update", Status: "completed", CreatedAt: time.Now()}
	calls := 0
	m := New(Config{Inbox: []inbox.Entry{entry}, InboxAction: func([]string) (string, error) { calls++; return "saved", nil }})
	m.section, m.inboxDetail = "inbox", entry.ID
	old := m.BridgeState().Panel.ID
	m.config.Inbox[0].RatingVersion = 2
	if m.BridgeState().Panel.ID == old {
		t.Fatal("feedback revision absent from view identity")
	}
	next, _ := m.ApplyBridgePanel(BridgePanelAction{ID: old, Action: "key"}, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	_ = next
	if calls != 0 {
		t.Fatal("stale assessment changed feedback")
	}
}

func TestInboxNotificationStatusDoesNotImplyDeliveredOrRead(t *testing.T) {
	now := time.Now().UTC()
	e := inbox.Entry{ID: "result-one", Kind: "job", Title: "Update", Status: "completed", CreatedAt: now,
		DesktopDelivery: &inbox.DesktopDelivery{State: "failed", Attempts: 1, AttemptedAt: now, NextAttemptAt: now.Add(time.Minute)}}
	m := New(Config{Inbox: []inbox.Entry{e}})
	m.section, m.inboxDetail = "inbox", e.ID
	panel := m.BridgeState().Panel
	status := ""
	for _, field := range panel.Summary {
		if field.Label == "Desktop notification" {
			status = field.Value
		}
	}
	if !strings.Contains(status, "unconfirmed") || !strings.Contains(status, "retry") || !m.config.Inbox[0].ReadAt.IsZero() || !m.config.Inbox[0].DeliveredAt.IsZero() {
		t.Fatal("failed handoff presented as delivery", status)
	}
}
