package snapshot

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateFreezesAndDeduplicatesSource(t *testing.T) {
	source := t.TempDir()
	state := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "report.txt"), []byte("first"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".env"), []byte("SECRET=yes"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	first, err := Create(source, state, Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if first.Files != 1 || first.Bytes != 5 || len(first.Exclusions) != 1 || first.SHA256 == "" {
		t.Fatalf("unexpected manifest: %#v", first)
	}
	if err := os.WriteFile(filepath.Join(source, "report.txt"), []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(first.Materialized, "report.txt"))
	if err != nil || string(got) != "first" {
		t.Fatalf("frozen bytes = %q, %v", got, err)
	}
	opened, err := Open(state, first.ID)
	if err != nil || opened.SHA256 != first.SHA256 {
		t.Fatalf("Open = %#v, %v", opened, err)
	}
}

func TestCreateRejectsLimitsAndSymlinks(t *testing.T) {
	source := t.TempDir()
	state := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "large"), []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("large", filepath.Join(source, "link")); err != nil {
		t.Fatal(err)
	}
	_, err := Create(source, state, Options{Limits: Limits{MaxFiles: 2, MaxTotal: 4, MaxFileBytes: 4}})
	var limit *LimitError
	if !errors.As(err, &limit) {
		t.Fatalf("error = %v, want LimitError", err)
	}
}
