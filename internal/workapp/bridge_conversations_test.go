package workapp

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/worksession"
)

func TestBridgeRecentsBoundSortedDisplayMetadata(t *testing.T) {
	var records []worksession.Conversation
	for index := 0; index < 30; index++ {
		records = append(records, worksession.Conversation{ID: fmt.Sprintf("work-%02d", index), Title: "Title\nline", SourcePath: "/workspace", SnapshotID: "private-snapshot", UpdatedAt: time.Unix(int64(index), 0)})
	}
	model := New(Config{Conversations: records})
	state := model.BridgeState()
	if len(state.RecentConversations) != 24 || state.RecentConversations[0].ID != "work-29" || len(state.RecentID) != 64 || state.RecentConversations[0].Title != "Title line" {
		t.Fatalf("recent projection = %#v", state)
	}
	if records[0].ID != "work-00" {
		t.Fatal("projection mutated the authoritative order")
	}
}

func TestBridgeRecentSelectionUsesExistingRestoreAndRejectsStaleOrBusy(t *testing.T) {
	records := []worksession.Conversation{{ID: "work-one", Title: "One", SourcePath: "/one", UpdatedAt: time.Unix(1, 0)}}
	loaded := 0
	model := New(Config{BridgeMode: true, CurrentFolder: "/new", Conversations: records, ListConversations: func() ([]worksession.Conversation, error) { return records, nil }, LoadConversation: func(id string) (ConversationState, error) {
		loaded++
		return ConversationState{Title: "One", SourcePath: "/one", RevisionID: "rev-one", Messages: []TranscriptMessage{{Role: "Gator", Text: "Retained response"}}}, nil
	}})
	action := BridgeConversationAction{ID: "work-one", ListID: model.BridgeState().RecentID}
	model.running = true
	next, _ := model.OpenBridgeConversation(action)
	if loaded != 0 || !strings.Contains(next.(Model).bridgeNotice, "Finish") {
		t.Fatal("active work was bypassed")
	}
	model.running = false
	stale := action
	stale.ListID = strings.Repeat("a", 64)
	next, _ = model.OpenBridgeConversation(stale)
	if loaded != 0 || next.(Model).conversation != "" {
		t.Fatal("stale selection opened work")
	}
	next, _ = model.OpenBridgeConversation(action)
	state := next.(Model).BridgeState()
	if loaded != 1 || state.ConversationID != "work-one" || state.Source != "/one" || state.Messages[0].Text != "Retained response" {
		t.Fatalf("restored = %#v", state)
	}
	records = nil
	next, _ = model.OpenBridgeConversation(action)
	if loaded != 1 || next.(Model).conversation != "" {
		t.Fatal("removed conversation was restored")
	}
}

func TestBridgeRecentsRefreshFailureDoesNotInventEmptySuccess(t *testing.T) {
	model := New(Config{BridgeMode: true, ListConversations: func() ([]worksession.Conversation, error) { return nil, errors.New("unavailable") }})
	model.refreshBridgeConversations()
	state := model.BridgeState()
	if state.RecentIssue == "" || state.RecentID != "" {
		t.Fatalf("failed list = %#v", state)
	}
}
