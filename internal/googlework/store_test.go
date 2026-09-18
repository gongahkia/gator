package googlework

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestStoreMirrorsLiveGoogleResponsesAndSearchesPrivately(t *testing.T) {
	store, err := Open(t.TempDir(), "google-work")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mirror permissions = %o", info.Mode().Perm())
	}
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	if err := store.Mirror(context.Background(), "drive_search", json.RawMessage(`{"files":[{"id":"file-1","name":"Q4 Roadmap","modifiedTime":"2026-09-18T09:00:00Z","mimeType":"application/vnd.google-apps.document"}]}`), now); err != nil {
		t.Fatal(err)
	}
	records, err := store.Search(context.Background(), "roadmap", []string{"drive_file"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != "file-1" || records[0].Name != "Q4 Roadmap" || !json.Valid(records[0].Raw) {
		t.Fatalf("records = %#v", records)
	}
	if err := store.Mirror(context.Background(), "tasks_list", json.RawMessage(`{"items":[{"id":"task-1","title":"Prepare roadmap","updated":"2026-09-18T09:30:00Z"}]}`), now); err != nil {
		t.Fatal(err)
	}
	records, err = store.Search(context.Background(), "prepare", []string{"task"}, 10)
	if err != nil || len(records) != 1 || records[0].ID != "task-1" {
		t.Fatalf("task records = %#v, %v", records, err)
	}
}

func TestStoreRejectsUnsafeConnectorIDAndInvalidSearch(t *testing.T) {
	if _, err := Open(t.TempDir(), "../other"); err == nil {
		t.Fatal("unsafe connector ID was accepted")
	}
	store, err := Open(t.TempDir(), "google-work")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.Search(context.Background(), "", nil, 10); err == nil {
		t.Fatal("empty search was accepted")
	}
}
