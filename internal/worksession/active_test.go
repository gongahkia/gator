package worksession

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestActiveChatsKeepBlankIdentitiesAndMigratePrimary(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(root)
	old, _ := s.Create("Older chat", t.TempDir(), "snap-old", time.Now())
	if err := s.BindPrimary(old.ID); err != nil {
		t.Fatal(err)
	}
	if id, err := s.Active(); err != nil || id != old.ID {
		t.Fatal(id, err)
	}
	first, err := s.CreateChat(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateChat(first.SourcePath, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || first.SnapshotID != "" || first.HeadRevision != "" {
		t.Fatal("new chats must have separate identities without fabricated evidence")
	}
	if err = s.SetActive(first.ID); err != nil {
		t.Fatal(err)
	}
	reopened, _ := Open(root)
	if id, err := reopened.Active(); err != nil || id != first.ID {
		t.Fatal(id, err)
	}
	if err = reopened.SetActive(second.ID); err != nil {
		t.Fatal(err)
	}
	if id, err := reopened.Active(); err != nil || id != second.ID {
		t.Fatal(id, err)
	}
	if id, _ := reopened.Primary(); id != old.ID {
		t.Fatal("rewrote legacy conversation designation")
	}
	if err = os.WriteFile(filepath.Join(s.root, "active.json"), []byte(`{"version":1,"conversation_id":"../escape"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Active(); err == nil {
		t.Fatal("corrupt last chat silently selected another conversation")
	}
}
