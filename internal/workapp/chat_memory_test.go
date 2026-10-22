package workapp

import (
	"errors"
	"github.com/gongahkia/gator/internal/memory"
	"testing"
	"time"
)

func TestMemoryReceiptProjectsOwnedSavedStateAndAcknowledgesRejection(t *testing.T) {
	now := time.Now().UTC()
	records := []memory.Record{{ID: "mem-1", Text: "Short replies", CreatedAt: now, UpdatedAt: now, Origin: &memory.ChatOrigin{ConversationID: "work-chat", RunID: "run-1", Quote: "Short replies", Basis: "user-stated"}}, {ID: "mem-foreign", Text: "Not this chat", Origin: &memory.ChatOrigin{ConversationID: "work-other"}}}
	calls := 0
	m := New(Config{ReadChatMemories: func(string, string) ([]memory.Record, error) { return records, nil }, ReviewChatMemory: func(a ChatMemoryAction, goal string) error { calls++; return errors.New("stale revision") }})
	m.conversation = "work-chat"
	m.refreshChatMemories()
	s := m.BridgeState()
	if len(s.ChatMemories) != 1 || s.ChatMemories[0].RunID != "run-1" || !s.ChatMemories[0].CanUndo {
		t.Fatal("invalid receipt projection", s.ChatMemories)
	}
	m = m.ApplyChatMemory(ChatMemoryAction{ConversationID: "work-other", ID: "mem-1", RequestID: "other", Action: "undo"})
	if calls != 0 {
		t.Fatal("cross-chat action executed")
	}
	m = m.ApplyChatMemory(ChatMemoryAction{ConversationID: "work-chat", ID: "mem-1", RequestID: "current", Action: "undo"})
	if calls != 1 || m.chatMemoryResult.RequestID != "current" || m.chatMemoryResult.Error != "stale revision" {
		t.Fatal("failed save acknowledged as accepted")
	}
	records = nil
	m.refreshChatMemories()
	if len(m.BridgeState().ChatMemories) != 0 {
		t.Fatal("forgotten record kept as saved")
	}
}

func TestMemoryReceiptRefreshFailureRetainsOnlySameChatAndGoal(t *testing.T) {
	var readErr error
	records := []memory.Record{{ID: "mem-1", Text: "Short replies", Origin: &memory.ChatOrigin{ConversationID: "work-chat", RunID: "run-1"}}}
	m := New(Config{ReadChatMemories: func(string, string) ([]memory.Record, error) { return records, readErr }})
	m.conversation = "work-chat"
	m.refreshChatMemories()
	readErr = errors.New("temporary read failure")
	m.refreshChatMemories()
	if len(m.chatMemories) != 1 || m.chatMemoryIssue == "" {
		t.Fatal("refresh failure discarded the current receipt or hid its stale state")
	}
	m.goalID = "goal-other"
	m.refreshChatMemories()
	if len(m.chatMemories) != 0 {
		t.Fatal("cached receipt crossed goal scope")
	}
	m.goalID, readErr = "", nil
	m.refreshChatMemories()
	if len(m.chatMemories) != 1 || m.chatMemoryIssue != "" {
		t.Fatal("successful refresh did not restore the current projection")
	}
	m.conversation, readErr = "work-other", errors.New("temporary read failure")
	m.refreshChatMemories()
	if len(m.chatMemories) != 0 {
		t.Fatal("cached receipt crossed conversation scope")
	}
}
