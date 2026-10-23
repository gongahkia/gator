package workapp

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/worksession"
)

// Only display-safe metadata leaves Go; no replay state, provider settings,
// captured files, or agent history is loaded to populate the sidebar.
type BridgeConversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Source    string    `json:"source"`
	UpdatedAt time.Time `json:"updated_at"`
	Pinned    bool      `json:"pinned,omitempty"`
}
type BridgeConversationAction struct {
	ID     string `json:"id"`
	ListID string `json:"list_id"`
}

func (m Model) bridgeRecentConversations() ([]BridgeConversation, string) {
	if m.bridgeRecentIssue != "" {
		return nil, ""
	}
	var items []worksession.Conversation
	for _, item := range m.config.Conversations {
		if !item.Archived {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Pinned != items[j].Pinned {
			return items[i].Pinned
		}
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	items = items[:min(len(items), 24)]
	recent := make([]BridgeConversation, 0, len(items))
	for _, item := range items {
		recent = append(recent, BridgeConversation{ID: item.ID, Title: truncate(singleLine(item.Title), 256), Source: item.SourcePath, UpdatedAt: item.UpdatedAt, Pinned: item.Pinned})
	}
	encoded, _ := json.Marshal(recent)
	return recent, fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func (m *Model) refreshBridgeConversations() {
	if !m.config.BridgeMode || m.config.ListConversations == nil {
		return
	}
	items, err := m.config.ListConversations()
	if err != nil {
		m.bridgeRecentIssue = "Recent conversations are unavailable. Open Conversations to retry."
		return
	}
	m.config.Conversations, m.bridgeRecentIssue = items, ""
	if m.config.LastConversation != nil && m.title == "New chat" {
		for _, item := range items {
			if item.ID == m.conversation {
				m.title = item.Title
				break
			}
		}
	}
}

func (m *Model) refreshBridgeFollowing() {
	if !m.config.BridgeMode || m.config.ListGoals == nil {
		return
	}
	items, err := m.config.ListGoals()
	if err != nil {
		m.bridgeFollowingIssue = "Ongoing goals are unavailable. Open goals to retry."
		return
	}
	m.config.Goals, m.bridgeFollowingIssue = items, ""
}

// OpenBridgeConversation reuses the existing conversation picker restore path.
// A stale sidebar click cannot select a different row or bypass active work.
func (m Model) OpenBridgeConversation(action BridgeConversationAction) (tea.Model, tea.Cmd) {
	m.bridgeNotice = ""
	if (m.running && !m.config.ManagedSessions) || m.controlBusy || m.followupSaving || m.recallBusy || m.bridgeExternal != nil {
		m.bridgeNotice = "Finish or stop the current operation before switching conversations."
		return m, nil
	}
	m.refreshBridgeConversations()
	items, identity := m.bridgeRecentConversations()
	if action.ListID == "" || action.ListID != identity {
		m.bridgeNotice = "Recent conversations changed. Choose the conversation again."
		return m, nil
	}
	found := false
	for _, item := range items {
		found = found || item.ID == action.ID
	}
	if !found {
		m.bridgeNotice = "This conversation is no longer in the recent list."
		return m, nil
	}
	if m.config.ManagedSessions {
		m.requestSession(BridgeSessionRequest{ConversationID: action.ID})
		return m, nil
	}
	if m.config.QueryConversations != nil {
		for _, item := range items {
			if item.ID == action.ID {
				m.cancelCatalog()
				m.launcher, m.launcherMode, m.paletteQuery, m.selected = true, "conversations", "", 0
				m.entries = []entry{{kind: "conversation", id: item.ID, title: item.Title, source: item.Source, subtitle: item.Source}}
				return m.updateLauncher(tea.KeyMsg{Type: tea.KeyEnter})
			}
		}
	}
	next, _ := m.NavigateBridge("/conversations")
	model := next.(Model)
	for index, item := range model.entries {
		if item.id == action.ID {
			model.selected = index
			return model.updateLauncher(tea.KeyMsg{Type: tea.KeyEnter})
		}
	}
	model.bridgeNotice = "This conversation is no longer available."
	return model, nil
}
