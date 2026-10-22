package workapp

import (
	"github.com/gongahkia/gator/internal/worksession"
	"testing"
	"time"
)

func TestNewChatsResumeLastAndKeepIndependentScope(t *testing.T) {
	s, _ := worksession.Open(t.TempDir())
	source := t.TempDir()
	create := func(path string) (worksession.Conversation, error) { return s.CreateChat(path, time.Now()) }
	load := func(id string) (ConversationState, error) {
		c, e := s.Load(id)
		return ConversationState{Title: c.Title, SourcePath: c.SourcePath}, e
	}
	cfg := Config{CurrentFolder: source, LastConversation: s.Active, RememberConversation: s.SetActive, CreateConversation: create, LoadConversation: load}
	first := New(cfg)
	id := first.conversation
	if id == "" || !first.home || first.primary {
		t.Fatal("initial empty chat lacks its own identity")
	}
	first.options.ConnectorIDs = []string{"private-connector"}
	first.messages = []message{{role: "You", text: "Stay in this chat"}}
	next, _ := first.NavigateBridge("/new")
	second := next.(Model)
	if second.conversation == id || second.source != source || len(second.messages) != 0 || len(second.options.ConnectorIDs) != 0 {
		t.Fatal("new chat inherited old transcript or connector selections")
	}
	reopened := New(cfg)
	if reopened.conversation != second.conversation {
		t.Fatal("restart did not reopen last blank chat")
	}
	reopened.restoreConversation(id, "")
	if current, _ := s.Active(); current != id {
		t.Fatal("opening an old chat did not update navigation state")
	}
	reopened.running = true
	next, _ = reopened.NavigateBridge("/new")
	if next.(Model).conversation != id {
		t.Fatal("new chat abandoned running work")
	}
}
