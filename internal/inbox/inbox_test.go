package inbox

import (
	"testing"
	"time"
)

func TestAddListAndMarkRead(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	entry, err := store.Add(Entry{Kind: "job", Title: "Weekly brief", Summary: "Completed", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := store.List(true, 10)
	if err != nil || len(entries) != 1 || entries[0].ID != entry.ID {
		t.Fatalf("List = %#v, %v", entries, err)
	}
	if err := store.MarkRead(entry.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	entries, err = store.List(true, 10)
	if err != nil || len(entries) != 0 {
		t.Fatalf("unread List = %#v, %v", entries, err)
	}
}

func TestMarkReadRejectsTraversal(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRead("../outside", time.Now()); err == nil {
		t.Fatal("inbox traversal ID was accepted")
	}
}
