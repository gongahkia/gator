package workapp

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/inbox"
)

func TestChatUpdatePagesStayBoundedAndAnchoredAsNewResultsArrive(t *testing.T) {
	store, err := inbox.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC()
	for i := 0; i < 103; i++ {
		if _, err := store.Add(inbox.Entry{ID: fmt.Sprintf("inbox-%03d", i), Title: "Update", Status: "completed", OriginConversationID: "chat", CreatedAt: start.Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	m := New(Config{InboxStore: &store})
	m.conversation = "chat"
	m.refreshChatUpdates()
	if len(m.chatUpdates) != 50 || m.chatUpdates[0].ID != "inbox-102" || m.chatUpdatesTotal != 103 || !m.updatesHaveOlder {
		t.Fatal("initial page missing")
	}
	m = m.ApplyBridgeUpdate(BridgeUpdateAction{ConversationID: "chat", ID: "inbox-053", Action: "older"})
	if len(m.chatUpdates) != 50 || m.chatUpdates[0].ID != "inbox-052" {
		t.Fatal("second page wrong")
	}
	if _, err := store.Add(inbox.Entry{ID: "inbox-new", Title: "New result", Status: "completed", OriginConversationID: "chat", CreatedAt: start.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	m.refreshChatUpdates()
	if m.chatUpdates[0].ID != "inbox-052" || len(m.chatUpdates) != 50 {
		t.Fatal("new result shifted the history page")
	}
	m = m.ApplyBridgeUpdate(BridgeUpdateAction{ConversationID: "chat", ID: "inbox-003", Action: "older"})
	if len(m.chatUpdates) != 3 || m.updatesHaveOlder {
		t.Fatal("oldest page missing")
	}
	m = m.ApplyBridgeUpdate(BridgeUpdateAction{ConversationID: "chat", ID: "inbox-002", Action: "newer"})
	if len(m.chatUpdates) != 50 || m.chatUpdates[0].ID != "inbox-052" {
		t.Fatal("return page shifted")
	}
	m = m.ApplyBridgeUpdate(BridgeUpdateAction{ConversationID: "chat", ID: "inbox-052", Action: "newer"})
	if m.chatUpdates[0].ID != "inbox-new" || len(m.updatePageAnchors) != 0 {
		t.Fatal("newest result unavailable")
	}
}

func TestChatUpdateReplyAdmissionAndRecoveryKeepExactOwner(t *testing.T) {
	var captured RunOptions
	calls := 0
	config, sessions := requestConfig(t, func(_, _, _ string, options RunOptions) RunResult {
		calls++
		captured = options
		return RunResult{FinalText: "Reply retained"}
	})
	store, err := inbox.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config.InboxStore, config.ManagedSessions = &store, true
	m := New(config).FocusBridgeSession(true)
	entry, err := store.Add(inbox.Entry{Title: "Offsite", Summary: "Room changed", Status: "completed", OriginConversationID: m.conversation})
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.Add(inbox.Entry{Title: "Other", Summary: "Other chat", Status: "completed", OriginConversationID: "other-chat"})
	if err != nil {
		t.Fatal(err)
	}
	m = m.FocusBridgeSession(true)
	if got := m.BridgeState().ChatUpdates; len(got) != 1 || got[0].ID != entry.ID || got[0].Delivered || got[0].Read {
		t.Fatalf("initial projection: %+v", got)
	}
	m = m.ApplyBridgeUpdate(BridgeUpdateAction{ConversationID: m.conversation, ID: entry.ID, Action: "delivered"})
	if got := m.BridgeState().ChatUpdates[0]; !got.Delivered || got.Read || got.Resolved {
		t.Fatalf("display implied decision: %+v", got)
	}
	request := BridgeUploadSubmission{RequestID: "reply-one", Context: "conversation:" + m.conversation, Text: "Use the second option", ReplyToUpdateID: other.ID}
	next, run := m.SubmitBridgeUploads(request)
	if run != nil || next.(Model).bridgeUploadResult.Error == "" {
		t.Fatal("cross-chat reply accepted")
	}
	request.ReplyToUpdateID = entry.ID
	next, run = m.SubmitBridgeUploads(request)
	m = next.(Model)
	if run == nil || m.bridgeUploadResult.Error != "" {
		t.Fatal("reply not admitted", m.bridgeUploadResult)
	}
	request.RequestID, request.Text = "reply-two", "Also update the draft"
	next, queued := m.SubmitBridgeUploads(request)
	m = next.(Model)
	if queued != nil || len(m.queue) != 1 || m.queue[0].options.ReplyToUpdateID != entry.ID {
		t.Fatal("queued reply lost target")
	}
	journal, err := sessions.LoadRequests(m.conversation)
	if err != nil || len(journal.Requests) != 2 {
		t.Fatal("admission missing", err)
	}
	for _, retained := range journal.Requests {
		var options RunOptions
		if json.Unmarshal(retained.Options, &options) != nil || options.ReplyToUpdateID != entry.ID {
			t.Fatal("durable request lost reply target")
		}
	}
	next, retry := m.SubmitBridgeUploads(request)
	if retry != nil || len(next.(Model).queue) != 1 {
		t.Fatal("transport retry duplicated reply")
	}
	request.ReplyToUpdateID = ""
	next, retry = m.SubmitBridgeUploads(request)
	if retry != nil || next.(Model).bridgeUploadResult.Error == "" {
		t.Fatal("transport identity allowed changing reply target")
	}
	_ = run()
	if calls != 1 || captured.ReplyToUpdateID != entry.ID {
		t.Fatalf("execution target: %+v", captured)
	}
	config.StartConversationID = m.conversation
	restored := New(config)
	if len(restored.queue) != 2 || restored.queue[1].options.ReplyToUpdateID != entry.ID {
		t.Fatal("recovery lost origin")
	}
	if retained, _ := store.Load(entry.ID); !retained.ReadAt.IsZero() || !retained.ResolvedAt.IsZero() {
		t.Fatal("reply implied read or resolution")
	}
}
