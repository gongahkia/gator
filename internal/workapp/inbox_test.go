package workapp

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/inbox"
)

func TestInboxOpeningIsReadOnlyAndExplicitActionsUseCurrentView(t *testing.T) {
	entry := inbox.Entry{ID: "result-one", Title: "Project update", Summary: "An exact retained update.", Status: "completed", CreatedAt: time.Now()}
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
