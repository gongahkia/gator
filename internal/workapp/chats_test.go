package workapp

import (
	"errors"
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

func TestNewChatCreationFailureKeepsCurrentChatAndFolderChangesGetIdentity(t *testing.T) {
	sessions, _ := worksession.Open(t.TempDir())
	source, other := t.TempDir(), t.TempDir()
	cfg := Config{CurrentFolder: source, CreateConversation: func(path string) (worksession.Conversation, error) { return sessions.CreateChat(path, time.Now()) }, RememberConversation: sessions.SetActive, ResolveSource: func(path string) (string, error) { return path, nil }}
	m := New(cfg)
	first := m.conversation
	m.messages = []message{{role: "You", text: "Keep this conversation"}}
	m.options.ConnectorIDs = []string{"personal"}
	m.config.CreateConversation = func(string) (worksession.Conversation, error) {
		return worksession.Conversation{}, errors.New("disk unavailable")
	}
	next, _ := m.runLocalCommand("/new")
	failed := next.(Model)
	if failed.conversation != first || len(failed.messages) != 1 || failed.bridgeNotice == "" {
		t.Fatal("creation failure replaced the current chat")
	}
	m.config.CreateConversation = cfg.CreateConversation
	next, _ = m.runLocalCommand("/source " + other)
	changed := next.(Model)
	if changed.conversation == "" || changed.conversation == first || changed.source != other || len(changed.options.ConnectorIDs) != 0 || changed.lastWorkID != "" {
		t.Fatal("folder switch inherited old chat state")
	}
	retained, err := sessions.Load(changed.conversation)
	if err != nil || retained.SourcePath != other {
		t.Fatal("folder chat identity not persisted", err)
	}
}
