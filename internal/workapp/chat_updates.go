package workapp

import (
	"fmt"
	"time"
)

type BridgeChatUpdate struct {
	NextStep  string    `json:"next_step,omitempty"`
	Evidence  []string  `json:"evidence,omitempty"`
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	Status    string    `json:"status"`
	JobID     string    `json:"job_id,omitempty"`
	GoalID    string    `json:"goal_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Delivered bool      `json:"delivered"`
	Read      bool      `json:"read"`
	Resolved  bool      `json:"resolved"`
	Version   uint64    `json:"version"`
}

type BridgeUpdateAction struct {
	ConversationID string `json:"conversation_id"`
	ID             string `json:"id"`
	Action         string `json:"action"`
	Version        uint64 `json:"version"`
}

type BridgeUpdateConversation struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Count int    `json:"count"`
}

func (m *Model) refreshChatUpdates() {
	m.chatUpdates, m.chatUpdatesIssue, m.chatUpdatesTotal = nil, "", 0
	if m.updatePageChat != m.conversation {
		m.updatePageChat, m.updatePageAnchors = m.conversation, nil
	}
	m.updatesHaveOlder = false
	m.updateConversations = nil
	if m.config.InboxStore == nil {
		return
	}
	entries, err := m.config.InboxStore.List(false, 0)
	if err != nil {
		m.chatUpdatesIssue = "Follow-up updates unavailable: " + err.Error()
		return
	}
	byConversation := map[string]int{}
	anchor := ""
	if len(m.updatePageAnchors) > 0 {
		anchor = m.updatePageAnchors[len(m.updatePageAnchors)-1]
	}
	pageStarted := anchor == ""
	for _, entry := range entries {
		if m.conversation != "" && entry.OriginConversationID == m.conversation {
			m.chatUpdatesTotal++
			if pageStarted && len(m.chatUpdates) < 50 {
				m.chatUpdates = append(m.chatUpdates, entry)
			} else if pageStarted {
				m.updatesHaveOlder = true
			}
			if entry.ID == anchor {
				pageStarted = true
			}
		}
		if entry.OriginConversationID == "" || !entry.ReadAt.IsZero() || !entry.ResolvedAt.IsZero() || m.config.SessionStore == nil {
			continue
		}
		if index, found := byConversation[entry.OriginConversationID]; found {
			m.updateConversations[index].Count++
			continue
		}
		if len(m.updateConversations) == 50 {
			continue
		}
		conversation, err := m.config.SessionStore.Load(entry.OriginConversationID)
		if err != nil {
			continue
		} // Deleted chats are still visible in the inbox.
		byConversation[entry.OriginConversationID] = len(m.updateConversations)
		m.updateConversations = append(m.updateConversations, BridgeUpdateConversation{ID: conversation.ID, Title: conversation.Title, Count: 1})
	}
	if !pageStarted || len(m.chatUpdates) == 0 && len(m.updatePageAnchors) > 0 {
		m.updatePageAnchors = nil
		m.refreshChatUpdates()
	}
}

func (m Model) bridgeChatUpdates() []BridgeChatUpdate {
	result := make([]BridgeChatUpdate, 0, len(m.chatUpdates))
	for i := len(m.chatUpdates) - 1; i >= 0; i-- {
		e := m.chatUpdates[i]
		result = append(result, BridgeChatUpdate{ID: e.ID, Title: e.Title, Summary: e.Summary, NextStep: e.NextStep, Evidence: append([]string(nil), e.Evidence...), Status: e.Status, JobID: e.JobID, GoalID: e.GoalID, CreatedAt: e.CreatedAt, Delivered: !e.DeliveredAt.IsZero(), Read: !e.ReadAt.IsZero(), Resolved: !e.ResolvedAt.IsZero(), Version: e.ResolutionVersion})
	}
	return result
}

func (m Model) ApplyBridgeUpdate(action BridgeUpdateAction) Model {
	if m.config.InboxStore == nil || m.conversation == "" || action.ConversationID != m.conversation {
		m.chatUpdatesIssue = "Return to the update’s conversation before changing it."
		return m
	}
	if action.Action == "older" || action.Action == "newer" {
		if len(m.chatUpdates) == 0 {
			return m
		}
		if action.Action == "older" && m.updatesHaveOlder && action.ID == m.chatUpdates[len(m.chatUpdates)-1].ID {
			m.updatePageAnchors = append(m.updatePageAnchors, action.ID)
		} else if action.Action == "newer" && len(m.updatePageAnchors) > 0 && action.ID == m.chatUpdates[0].ID {
			m.updatePageAnchors = m.updatePageAnchors[:len(m.updatePageAnchors)-1]
		} else {
			m.chatUpdatesIssue = "Updates changed; use the current page controls."
			return m
		}
		m.refreshChatUpdates()
		return m
	}
	_, err := m.config.InboxStore.Receipt(m.conversation, action.ID, action.Action, action.Version, time.Now())
	m.refreshChatUpdates()
	if err != nil {
		m.chatUpdatesIssue = err.Error()
	}
	return m
}

func (m Model) validateUpdateReply(id string) error {
	if id == "" {
		return nil
	}
	if m.config.InboxStore == nil || m.conversation == "" {
		return fmt.Errorf("this update is unavailable")
	}
	entry, err := m.config.InboxStore.Load(id)
	if err != nil {
		return fmt.Errorf("this update is unavailable: %w", err)
	}
	if entry.OriginConversationID != m.conversation {
		return fmt.Errorf("this update belongs to another conversation")
	}
	return nil
}
