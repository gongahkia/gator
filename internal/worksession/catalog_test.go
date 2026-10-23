package worksession

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConversationCatalogPagesHundredsOfChatsWithStableAnchors(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 310; i++ {
		c := Conversation{Version: Version, ID: fmt.Sprintf("work-%03d", i), Title: fmt.Sprintf("Conversation %d", i), SourcePath: "/project", CreatedAt: at, UpdatedAt: at.Add(time.Duration(i) * time.Minute)}
		if err := s.writeConversation(c); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.PageConversations(ConversationQuery{})
	if err != nil || len(first.Items) != 50 || first.Next == "" || first.Items[0].ID != "work-309" || first.Incomplete {
		t.Fatal(first, err)
	}
	c := Conversation{Version: Version, ID: "work-new", Title: "New arrival", SourcePath: "/project", CreatedAt: at, UpdatedAt: at.Add(24 * time.Hour)}
	if err := s.writeConversation(c); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, item := range first.Items {
		seen[item.ID] = true
	}
	cursor := first.Next
	for cursor != "" {
		page, err := s.PageConversations(ConversationQuery{After: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			if seen[item.ID] || item.ID == "work-new" {
				t.Fatal("pagination duplicated or inserted ahead of its anchor", item)
			}
			seen[item.ID] = true
		}
		cursor = page.Next
	}
	if len(seen) != 310 {
		t.Fatal("history was silently truncated", len(seen))
	}
	if _, err := s.PageConversations(ConversationQuery{Text: "different", After: first.Next}); err == nil {
		t.Fatal("cursor from another query accepted")
	}
}

func TestConversationOrganizationIsVersionedReversibleAndKeepsExecutionState(t *testing.T) {
	s, id, dir := requestTestStore(t)
	initial, err := s.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := s.OrganizeConversation(ConversationChange{ID: id, Version: ConversationDisplayVersion(initial), Action: "rename", Title: "New chat"})
	if err != nil || !renamed.TitleEdited {
		t.Fatal(renamed, err)
	}
	if _, err := s.SaveRequests(id, RequestJournal{Version: 1, Requests: []Request{retainedRequest(t, "Do not replace my chosen title")}}); err != nil {
		t.Fatal(err)
	}
	renamed, err = s.Load(id)
	if err != nil || renamed.Title != "New chat" {
		t.Fatal("explicit title was overwritten", renamed, err)
	}
	pinned, err := s.OrganizeConversation(ConversationChange{ID: id, Version: ConversationDisplayVersion(renamed), Action: "pin"})
	if err != nil || !pinned.Pinned {
		t.Fatal(pinned, err)
	}
	if _, err := s.OrganizeConversation(ConversationChange{ID: id, Version: ConversationDisplayVersion(renamed), Action: "archive"}); !errors.Is(err, ErrConversationChanged) {
		t.Fatal("stale organization accepted", err)
	}
	archived, err := s.OrganizeConversation(ConversationChange{ID: id, Version: ConversationDisplayVersion(pinned), Action: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	active, err := s.PageConversations(ConversationQuery{})
	if err != nil || len(active.Items) != 0 {
		t.Fatal(active, err)
	}
	hidden, err := s.PageConversations(ConversationQuery{Scope: "archived"})
	if err != nil || len(hidden.Items) != 1 || !hidden.Items[0].Pinned {
		t.Fatal(hidden, err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Load(id)
	if err != nil || !loaded.Archived || loaded.HeadRevision != initial.HeadRevision || loaded.SnapshotID != initial.SnapshotID {
		t.Fatal("organization changed execution state", loaded, err)
	}
	journal, err := reopened.LoadRequests(id)
	if err != nil || len(journal.Requests) != 1 || journal.Requests[0].Text != "Do not replace my chosen title" {
		t.Fatal("archive discarded admitted work", journal, err)
	}
	if _, err := s.OrganizeConversation(ConversationChange{ID: id, Version: ConversationDisplayVersion(archived), Action: "restore"}); err != nil {
		t.Fatal(err)
	}
	// Older releases still see only the canonical schema. Organization is an
	// optional sidecar, included with the chat in profile backups.
	var canonical map[string]any
	data, err := os.ReadFile(filepath.Join(s.root, id, "conversation.json"))
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(data, &canonical) != nil {
		t.Fatal("canonical JSON invalid")
	}
	for _, key := range []string{"pinned", "archived", "title_edited", "metadata_revision"} {
		if _, ok := canonical[key]; ok {
			t.Fatal("organization broke old conversation readers", key)
		}
	}
}

func TestConversationOrganizationConcurrentChangesHaveOneWinner(t *testing.T) {
	s, id, _ := requestTestStore(t)
	c, err := s.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	expected := ConversationDisplayVersion(c)
	var wg sync.WaitGroup
	results := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.OrganizeConversation(ConversationChange{ID: id, Version: expected, Action: "rename", Title: fmt.Sprintf("Title %d", i)})
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrConversationChanged) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal("concurrent edits overwrote one another", success)
	}
	for _, id := range []string{"..", "../outside", "/outside", "."} {
		if _, err := s.OrganizeConversation(ConversationChange{ID: id, Version: expected, Action: "pin"}); err == nil {
			t.Fatal("unsafe metadata identity accepted", id)
		}
	}
}

func TestConversationCatalogSearchesActualChatTextWithoutReplayingPrivateState(t *testing.T) {
	s, id, _ := requestTestStore(t)
	c, err := s.AddRevision(id, Revision{ID: "rev-search", SnapshotID: "snap-search", Objective: "Find a decision", BundlePath: "/retained/bundle", Status: "completed", FinalText: "Decision: choose the jade heron plan. 保留这个决定。", Replay: &ReplayState{Version: Version, Provider: "private-provider", Configuration: json.RawMessage(`{"system":"private-system-marker"}`)}, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	byText, err := s.PageConversations(ConversationQuery{Text: "JADE HERON", Content: true})
	if err != nil || len(byText.Items) != 1 || byText.Items[0].Match != "messages" || !strings.Contains(byText.Items[0].Snippet, "jade heron") {
		t.Fatal(byText, err)
	}
	for _, query := range []string{"private-system-marker", "private-provider"} {
		page, err := s.PageConversations(ConversationQuery{Text: query, Content: true})
		if err != nil || len(page.Items) != 0 {
			t.Fatal("search disclosed private replay", page, err)
		}
	}
	unicode, err := s.PageConversations(ConversationQuery{Text: "保留这个决定", Content: true})
	if err != nil || len(unicode.Items) != 1 {
		t.Fatal(unicode, err)
	}
	titles, err := s.PageConversations(ConversationQuery{Text: "jade heron"})
	if err != nil || len(titles.Items) != 0 {
		t.Fatal("title-only search inspected messages", titles, err)
	}
	if err := os.WriteFile(filepath.Join(s.revisionsPath(c.ID), c.HeadRevision+".json"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	failed, err := s.PageConversations(ConversationQuery{Text: "missing", Content: true})
	if err != nil || !failed.Incomplete {
		t.Fatal("incomplete search claimed no matches", failed, err)
	}
}
