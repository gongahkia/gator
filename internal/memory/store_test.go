package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMemoryPersistsScopedCorrectionsAndExpiry(t *testing.T) {
	state := t.TempDir()
	now := time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)
	store, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return now }
	goalID := "goal-0123456789abcdef01234567"
	global, err := store.Add("preference", "Prefer concise daily briefs", "", "user", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	local, err := store.Add("decision", "Send the launch deck on Friday", goalID, "meeting notes", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.Add("fact", "The venue is Harbour Hall", "goal-aaaaaaaaaaaaaaaaaaaaaaaa", "email", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	reopened.now = func() time.Time { return now }
	results, err := reopened.Search("launch deck", goalID, 8)
	if err != nil || len(results) != 1 || results[0].ID != local.ID {
		t.Fatalf("search = %#v, %v", results, err)
	}
	results, err = reopened.Search("venue", goalID, 8)
	if err != nil || len(results) != 0 {
		t.Fatalf("cross-goal leak = %#v, %v", results, err)
	}
	results, err = reopened.Search("briefs", goalID, 8)
	if err != nil || len(results) != 1 || results[0].ID != global.ID {
		t.Fatalf("global memory = %#v, %v", results, err)
	}
	if _, err := reopened.Correct(local.ID, "Send the launch deck on Monday", "meeting notes"); err != nil {
		t.Fatal(err)
	}
	results, err = reopened.Search("Friday", goalID, 8)
	if err != nil || len(results) != 0 {
		t.Fatalf("stale correction = %#v, %v", results, err)
	}
	results, err = reopened.Search("Monday", goalID, 8)
	if err != nil || len(results) != 1 {
		t.Fatalf("corrected memory = %#v, %v", results, err)
	}
	reopened.now = func() time.Time { return now.Add(2 * time.Hour) }
	results, err = reopened.Search("Monday", goalID, 8)
	if err != nil || len(results) != 0 {
		t.Fatalf("expired memory = %#v, %v", results, err)
	}
	if err := reopened.Forget(other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Load(other.ID); !os.IsNotExist(err) {
		t.Fatalf("forgotten memory load = %v", err)
	}
}

func TestMemoryRejectsInvalidOnDiskRecord(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Add("note", "A trusted note", "", "user", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.root, record.ID+".json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "outside"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Search("trusted", "", 8); err == nil || !strings.Contains(err.Error(), "bounded regular") {
		t.Fatalf("symlink accepted: %v", err)
	}
}
