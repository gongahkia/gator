package workapp

import (
	"testing"

	"github.com/gongahkia/gator/internal/worksession"
)

func TestManagedChatNavigationKeepsExecutionAndLastOpenedOwnership(t *testing.T) {
	config, store := requestConfig(t, func(_, _, _ string, _ RunOptions) RunResult { return RunResult{FinalText: "done"} })
	config.ManagedSessions = true
	config.LastConversation, config.RememberConversation = store.Active, store.SetActive
	m := New(config).FocusBridgeSession(true)
	id := m.conversation
	m.requestSession(BridgeSessionRequest{View: "/followup"})
	if m.sessionRequest.OriginConversationID != id {
		t.Fatal("view navigation lost originating chat")
	}
	next, run := m.SubmitText("Keep working here")
	m = next.(Model)
	next, _ = m.NavigateBridge("/new")
	m = next.(Model)
	if !m.running || m.conversation != id || !m.sessionRequest.New {
		t.Fatal("navigation replaced active execution")
	}
	m = m.FocusBridgeSession(false)
	config.StartNew = true
	other := New(config).FocusBridgeSession(true)
	if other.conversation == id {
		t.Fatal("new chat restored the previous chat")
	}
	next, _ = m.Update(run())
	m = next.(Model)
	active, err := store.Active()
	if err != nil || active != other.conversation {
		t.Fatal("background completion stole the active chat", active, err)
	}
	if len(m.messages) == 0 || m.conversation != id || m.running {
		t.Fatal("completion lost its origin")
	}
	m = m.FocusBridgeSession(true)
	active, _ = store.Active()
	if active != id {
		t.Fatal("returning to chat did not retain its selection")
	}
}

func TestManagedReadOnlyViewDoesNotCreateChatsAndSubmissionCapturesModel(t *testing.T) {
	var options RunOptions
	config, store := requestConfig(t, func(_, _, _ string, selected RunOptions) RunResult {
		options = selected
		return RunResult{FinalText: "done"}
	})
	config.ManagedSessions, config.StartView = true, true
	config.ViewOriginConversationID = "work-requesting-chat"
	config.SelectedModel = func() (string, string, error) { return "chosen-provider", "chosen-model", nil }
	m := New(config).FocusBridgeSession(true)
	m.newFollowup()
	if m.followupForm.Draft.OriginConversationID != config.ViewOriginConversationID {
		t.Fatal("follow-up from a view lost its origin")
	}
	m.followupForm, m.section = nil, ""
	chats, err := store.List(50)
	if err != nil || len(chats) != 0 || m.conversation != "" {
		t.Fatal("browsing created an empty chat")
	}
	m.options.Uploads = []string{"retained-input"}
	m.options.MaxSteps = 12
	next, run := m.SubmitText("Start from this view")
	m = next.(Model)
	if run == nil || m.conversation == "" {
		t.Fatal("first message did not create its own chat")
	}
	_ = run()
	if options.ModelProvider != "chosen-provider" || options.ModelName != "chosen-model" || options.MaxSteps != 12 || len(options.Uploads) != 1 {
		t.Fatalf("submission lost captured settings: %+v", options)
	}
	journal, err := store.LoadRequests(m.conversation)
	if err != nil || len(journal.Requests) != 1 {
		t.Fatal("first message was not admitted", err)
	}
}

func TestManagedRecentSelectionStillRejectsStaleLists(t *testing.T) {
	config, _ := requestConfig(t, nil)
	config.ManagedSessions = true
	config.Conversations = []worksession.Conversation{{ID: "work-selected", Title: "Selected"}}
	m := New(config)
	_, identity := m.bridgeRecentConversations()
	m.running = true
	next, _ := m.OpenBridgeConversation(BridgeConversationAction{ID: "work-selected", ListID: "stale"})
	if next.(Model).sessionRequest != nil {
		t.Fatal("stale selection opened a chat")
	}
	next, _ = m.OpenBridgeConversation(BridgeConversationAction{ID: "work-selected", ListID: identity})
	selected := next.(Model)
	if selected.sessionRequest == nil || selected.sessionRequest.ConversationID != "work-selected" || !selected.running || selected.conversation != m.conversation {
		t.Fatal("selection failed or changed running work")
	}
}
