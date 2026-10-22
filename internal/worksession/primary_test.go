package worksession

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrimaryKeepsIndependentTopicsAndSurvivesReopen(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	older, _ := store.Create("Private topic", "/private", "snap-private", time.Now())
	if id, err := store.Primary(); err != nil || id != "" {
		t.Fatalf("adopted existing topic: %q %v", id, err)
	}
	main, _ := store.Create("Main", "/main", "snap-main", time.Now())
	if err := store.BindPrimary(main.ID); err != nil {
		t.Fatal(err)
	}
	reopened, _ := Open(directory)
	if id, err := reopened.Primary(); err != nil || id != main.ID {
		t.Fatalf("lost primary: %q %v", id, err)
	}
	if err := reopened.BindPrimary(older.ID); err == nil {
		t.Fatal("replaced another window's primary")
	}
	if err := reopened.BindPrimary(main.ID); err != nil {
		t.Fatal(err)
	}
	if record, err := reopened.Load(older.ID); err != nil || record.SourcePath != "/private" {
		t.Fatalf("changed independent topic: %+v %v", record, err)
	}
	if err := os.RemoveAll(filepath.Join(store.root, main.ID)); err != nil {
		t.Fatal(err)
	}
	if id, err := reopened.Primary(); err != nil || id != "" {
		t.Fatalf("retained deleted primary: %q %v", id, err)
	}
	if err := reopened.BindPrimary(older.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPrimaryRejectsCorruption(t *testing.T) {
	store, _ := Open(t.TempDir())
	if err := os.WriteFile(filepath.Join(store.root, "primary.json"), []byte(`{"version":1,"conversation_id":"../outside"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Primary(); err == nil {
		t.Fatal("accepted invalid identity")
	}
}
