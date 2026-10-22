package workapp

import (
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/memory"
)

type ChatMemoryReceipt struct {
	ID        string `json:"id"`
	RunID     string `json:"run_id"`
	Text      string `json:"text"`
	Quote     string `json:"quote,omitempty"`
	Updated   string `json:"updated"`
	Scope     string `json:"scope"`
	Corrected bool   `json:"corrected,omitempty"`
	CanUndo   bool   `json:"can_undo,omitempty"`
}
type ChatMemoryAction struct {
	ConversationID string `json:"conversation_id"`
	ID             string `json:"id"`
	Updated        string `json:"updated"`
	Action         string `json:"action"`
	Text           string `json:"text,omitempty"`
	RequestID      string `json:"request_id"`
}
type ChatMemoryResult struct {
	RequestID string `json:"request_id"`
	Error     string `json:"error,omitempty"`
}

func (m *Model) refreshChatMemories() {
	owner := m.conversation + "\x00" + m.goalID
	if m.chatMemoriesOwner != owner {
		m.chatMemories, m.chatMemoriesOwner = nil, owner
	}
	m.chatMemoryIssue = ""
	if m.config.ReadChatMemories == nil || m.conversation == "" {
		m.chatMemories = nil
		return
	}
	records, err := m.config.ReadChatMemories(m.conversation, m.goalID)
	if err != nil {
		m.chatMemoryIssue = "Saved memories could not refresh. Shown memories may be out of date; your open edits are kept."
		return
	}
	m.chatMemories = nil
	for _, record := range records {
		if record.Origin == nil || record.Origin.ConversationID != m.conversation || record.GoalID != "" && record.GoalID != m.goalID {
			continue
		}
		scope := "Across your chats"
		if record.GoalID != "" {
			scope = "This goal"
		}
		m.chatMemories = append(m.chatMemories, ChatMemoryReceipt{ID: record.ID, RunID: record.Origin.RunID, Text: record.Text, Quote: record.Origin.Quote, Updated: record.UpdatedAt.Format(time.RFC3339Nano), Scope: scope, Corrected: record.Origin.Basis == "user-corrected", CanUndo: record.CreatedAt.Equal(record.UpdatedAt) && record.Origin.Basis == "user-stated"})
		if len(m.chatMemories) == 64 {
			break
		}
	}
}
func (m Model) ApplyChatMemory(action ChatMemoryAction) Model {
	m.chatMemoryResult = &ChatMemoryResult{RequestID: action.RequestID}
	if action.ConversationID != m.conversation || m.conversation == "" || action.RequestID == "" || len(action.RequestID) > 64 || len(action.Text) > 4096 || action.Action == "correct" && strings.TrimSpace(action.Text) == "" {
		m.chatMemoryResult.Error = "The chat or memory changed. Reopen the saved memory and try again."
		return m
	}
	if m.config.ReviewChatMemory == nil {
		m.chatMemoryResult.Error = "Memory review is unavailable."
		return m
	}
	if err := m.config.ReviewChatMemory(action, m.goalID); err != nil {
		m.chatMemoryResult.Error = err.Error()
	}
	m.refreshChatMemories()
	return m
}

// Kept as a typed adapter so desktop projection never reads arbitrary files.
type ReadChatMemories func(conversationID, goalID string) ([]memory.Record, error)
