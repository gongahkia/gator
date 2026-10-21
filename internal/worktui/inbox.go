package worktui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/uisnapshot"
)

func (m Model) selectedInboxEntry() *inbox.Entry {
	for _, entry := range m.config.Inbox {
		if entry.ID == m.inboxDetail {
			return &entry
		}
	}
	return nil
}

// Opening retained content never marks it read. Explicit actions use the same
// local inbox service as /inbox, and bridge actions retain their view-ID guard.
func (m Model) updateInboxKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.String() == "esc" {
		if m.inboxDetail != "" {
			m.inboxDetail, m.sectionNotice = "", ""
		} else {
			m.section = ""
		}
		return m, nil
	}
	if key.String() == "r" {
		if err := m.refreshInbox(); err != nil {
			m.sectionNotice = "Could not refresh Inbox: " + err.Error()
		}
		return m, nil
	}
	if m.inboxDetail == "" {
		switch key.String() {
		case "up":
			m.selected = max(0, m.selected-1)
		case "down":
			m.selected = min(max(0, len(m.config.Inbox)-1), m.selected+1)
		case "enter":
			if m.selected >= 0 && m.selected < len(m.config.Inbox) {
				m.inboxDetail, m.sectionNotice = m.config.Inbox[m.selected].ID, ""
			}
		}
		return m, nil
	}
	entry := m.selectedInboxEntry()
	if entry == nil || m.config.InboxAction == nil {
		return m, nil
	}
	var args []string
	message := ""
	switch key.String() {
	case "m":
		args, message = []string{"read", entry.ID}, "Marked as read."
	case "u", "n", "w", "c":
		rating := map[string]string{"u": "useful", "n": "noise", "w": "wrong", "c": "clear"}[key.String()]
		args, message = []string{"rate", entry.ID, rating}, "Rating saved."
	default:
		return m, nil
	}
	if _, err := m.config.InboxAction(args); err != nil {
		m.sectionNotice = "Could not update this result: " + err.Error()
		return m, nil
	}
	m.sectionNotice = message
	if err := m.refreshInbox(); err != nil {
		m.sectionNotice += " Refresh failed: " + err.Error()
	}
	m.refreshAttention()
	return m, nil
}

func (m Model) inboxDetailPanel() uisnapshot.Panel {
	p := uisnapshot.Panel{Kind: "inbox-detail", Title: "Inbox result", Notice: m.sectionNotice,
		Hints: []uisnapshot.Hint{{Shortcut: "Esc", Action: "back to Inbox"}, {Shortcut: "R", Action: "refresh"}}}
	entry := m.selectedInboxEntry()
	if entry == nil {
		p.Notice = "This result is no longer in the current Inbox. Go back and refresh the list."
		return p
	}
	p.Title = safeInboxText(entry.Title)
	p.Subtitle = strings.Join([]string{safeInboxText(entry.Status), entry.CreatedAt.Local().Format("2 Jan, 15:04")}, " · ")
	read := "Unread"
	if !entry.ReadAt.IsZero() {
		read = "Read"
	}
	p.Blocks = []uisnapshot.Block{{Title: "Retained update", Lines: []string{safeInboxText(entry.Summary)}},
		{Title: "Result details", Lines: []string{read, "Rating: " + valueOrDefault(entry.Rating, "Not rated"),
			fmt.Sprintf("Urgent: %t", entry.Urgent), "Result ID: " + entry.ID,
			"Goal: " + valueOrNone(entry.GoalID), "Check: " + valueOrNone(entry.JobID)}}}
	if m.config.InboxAction != nil {
		if entry.ReadAt.IsZero() {
			p.Hints = append(p.Hints, uisnapshot.Hint{Shortcut: "M", Action: "mark as read"})
		}
		p.Hints = append(p.Hints, uisnapshot.Hint{Shortcut: "U", Action: "useful"},
			uisnapshot.Hint{Shortcut: "N", Action: "noise"}, uisnapshot.Hint{Shortcut: "W", Action: "wrong"})
		if entry.Rating != "" {
			p.Hints = append(p.Hints, uisnapshot.Hint{Shortcut: "C", Action: "clear rating"})
		}
	}
	return p
}
