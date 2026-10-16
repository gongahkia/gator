package goal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGoalSurvivesRestartAndKeepsOneConversation(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Add("Plan the launch", "Watch the partner deadline", source)
	if err != nil {
		t.Fatal(err)
	}
	store, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(record.ID)
	if err != nil || loaded.Title != record.Title || loaded.Context != record.Context || loaded.Status != Active {
		t.Fatalf("reloaded goal = %#v, %v", loaded, err)
	}
	attached, err := store.AttachConversation(record.ID, "work-conversation-one")
	if err != nil || attached.ConversationID != "work-conversation-one" {
		t.Fatalf("attached goal = %#v, %v", attached, err)
	}
	if _, err := store.AttachConversation(record.ID, "work-conversation-two"); err == nil {
		t.Fatal("goal accepted a second conversation")
	}
	if _, err := store.SetStatus(record.ID, Completed); err != nil {
		t.Fatal(err)
	}
	list, err := store.List()
	if err != nil || len(list) != 1 || list[0].Status != Completed {
		t.Fatalf("goal list = %#v, %v", list, err)
	}
}

func TestGoalRejectsTraversalAndSymlinkRecord(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("../escape"); err == nil {
		t.Fatal("accepted traversal ID")
	}
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	id := "goal-aaaaaaaaaaaaaaaaaaaaaaaa"
	if err := os.Symlink(outside, store.path(id)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(id); err == nil {
		t.Fatal("accepted symlink goal")
	}
}

func TestGoalWithoutSourceGetsPrivateWorkspace(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Add("Track a trip", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(record.SourcePath); err != nil || !info.IsDir() {
		t.Fatalf("private goal workspace = %q, %v", record.SourcePath, err)
	}
}
