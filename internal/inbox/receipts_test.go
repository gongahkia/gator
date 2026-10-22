package inbox

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestUpdateReceiptsAreIndependentOwnedAndRetained(t *testing.T) {
	state := t.TempDir()
	s, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	entry := Entry{ID: JobUpdateID("job-one", "attempt-one"), Kind: "job", Title: "Venue changed", Summary: "Review the new room", Status: "completed", JobID: "job-one", AttemptID: "attempt-one", OriginConversationID: "chat-origin", ConversationID: "chat-execution"}
	entry, err = s.Add(entry)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, action := range []string{"delivered", "read", "resolve", "reopen"} {
		if _, err := s.Receipt("chat-execution", entry.ID, action, 0, now); err == nil {
			t.Fatalf("execution chat changed %s receipt", action)
		}
	}
	shown, err := s.Receipt("chat-origin", entry.ID, "delivered", 0, now)
	if err != nil || shown.DeliveredAt.IsZero() || !shown.ReadAt.IsZero() || !shown.ResolvedAt.IsZero() {
		t.Fatalf("shown = %+v, %v", shown, err)
	}
	read, err := s.Receipt("chat-origin", entry.ID, "read", 0, now)
	if err != nil || read.ReadAt.IsZero() || !read.ResolvedAt.IsZero() {
		t.Fatalf("read = %+v, %v", read, err)
	}
	done, err := s.Receipt("chat-origin", entry.ID, "resolve", 0, now)
	if err != nil || done.ResolvedAt.IsZero() || done.ResolutionVersion != 1 {
		t.Fatalf("done = %+v, %v", done, err)
	}
	if _, err := s.Receipt("chat-origin", entry.ID, "reopen", 0, now); err == nil {
		t.Fatal("stale reopen accepted")
	}
	again, err := s.Add(entry)
	if err != nil || again.ReadAt.IsZero() || again.ResolvedAt.IsZero() || again.DeliveredAt.IsZero() || again.ResolutionVersion != 1 {
		t.Fatalf("redelivery lost receipts = %+v, %v", again, err)
	}
	if _, err := s.Receipt("chat-origin", entry.ID, "resolve", 0, now); err != nil {
		t.Fatalf("retry was not idempotent: %v", err)
	}
	reopened, err := s.Receipt("chat-origin", entry.ID, "reopen", 1, now)
	if err != nil || !reopened.ResolvedAt.IsZero() || reopened.ReadAt.IsZero() || reopened.ResolutionVersion != 2 {
		t.Fatalf("reopened = %+v, %v", reopened, err)
	}
	if _, err := s.Receipt("chat-origin", entry.ID, "resolve", 0, now); err == nil {
		t.Fatal("old resolve undid reopen")
	}
	entry.Summary = "Substituted result"
	if _, err := s.Add(entry); err == nil {
		t.Fatal("same identity replaced result")
	}
	if JobUpdateID("job-one", "attempt-one") == JobUpdateID("job-one", "attempt-two") {
		t.Fatal("different attempts collided")
	}
	s, err = Open(state)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := s.Load(entry.ID)
	if err != nil || retained.Summary != "Review the new room" || retained.ResolutionVersion != 2 {
		t.Fatalf("restart lost update: %+v %v", retained, err)
	}
}

func TestConversationUpdatesDoNotGuessLegacyOrigins(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"", "chat-one", "chat-two"} {
		if _, err := s.Add(Entry{Title: "Update", Status: "completed", ConversationID: "chat-one", OriginConversationID: origin}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := s.ForConversation("chat-one", 10)
	if err != nil || len(entries) != 1 || entries[0].OriginConversationID != "chat-one" {
		t.Fatalf("updates = %+v, %v", entries, err)
	}
	if _, err := s.ForConversation("", 10); err == nil {
		t.Fatal("empty origin selected legacy entries")
	}
}

func TestConcurrentRedeliveryAndReceiptsDoNotLoseUpdates(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	entry, err := s.Add(Entry{ID: JobUpdateID("job", "attempt"), Kind: "job", Title: "Update", Status: "completed", OriginConversationID: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for range 12 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if _, err := s.Add(entry); err != nil {
				t.Error(err)
			}
			if err := s.MarkRead(entry.ID, time.Now()); err != nil {
				t.Error(err)
			}
			if _, err := s.Rate(entry.ID, "useful", time.Now()); err != nil {
				t.Error(err)
			}
			if _, err := s.Receipt("chat", entry.ID, "delivered", 0, time.Now()); err != nil {
				t.Error(err)
			}
			if _, err := s.Receipt("chat", entry.ID, "resolve", 0, time.Now()); err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
	got, err := s.Load(entry.ID)
	if err != nil || got.ReadAt.IsZero() || got.DeliveredAt.IsZero() || got.ResolvedAt.IsZero() || got.Rating != "useful" || got.ResolutionVersion != 1 {
		t.Fatalf("lost receipt: %+v, %v", got, err)
	}
	all, err := s.List(false, 0)
	if err != nil || len(all) != 1 {
		t.Fatalf("duplicate delivery: %+v, %v", all, err)
	}
}

func TestRedeliveryDoesNotReplaceCorruptEntry(t *testing.T) {
	state := t.TempDir()
	s, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(state, "gator", "inbox", "broken.json")
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(Entry{ID: "broken", Title: "Update", Status: "completed"}); err == nil {
		t.Fatal("corruption overwritten")
	}
}
