package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateWorkUsesOrdinaryDirectoryAndPrivateOutput(t *testing.T) {
	t.Parallel()

	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "notes.txt"), []byte("source material\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := t.TempDir()
	sourceRoot, err := Open(source)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 8, 1, 2, 3, 0, time.UTC)
	work, err := CreateWork(source, state, "work-001", now)
	if err != nil {
		t.Fatal(err)
	}
	if work.Source.Path() != sourceRoot.Path() || work.CreatedAt != now || work.Output.Path() == sourceRoot.Path() || work.Scratch.Path() == work.Output.Path() {
		t.Fatalf("work = %#v", work)
	}
	if _, err := os.Stat(filepath.Join(source, ".git")); !os.IsNotExist(err) {
		t.Fatalf("ordinary source unexpectedly became a Git repository: %v", err)
	}
	for _, path := range []string{work.Path, work.Output.Path(), work.Scratch.Path()} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("permissions for %s = %o", path, info.Mode().Perm())
		}
	}
	metadata, err := os.Stat(filepath.Join(work.Path, "workspace.json"))
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Mode().Perm() != 0o600 {
		t.Fatalf("metadata permissions = %o", metadata.Mode().Perm())
	}

	reopened, err := OpenWork(work.Path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.ID != work.ID || reopened.Source.Path() != work.Source.Path() || reopened.Output.Path() != work.Output.Path() {
		t.Fatalf("reopened work = %#v; want %#v", reopened, work)
	}
}

func TestCreateWorkRejectsCollisionsAndOverlap(t *testing.T) {
	t.Parallel()

	source := t.TempDir()
	state := t.TempDir()
	now := time.Now()
	if _, err := CreateWork(source, state, "work-duplicate", now); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateWork(source, state, "work-duplicate", now); err == nil {
		t.Fatal("duplicate work id was accepted")
	}

	overlappingState := filepath.Join(source, "private-state")
	if _, err := CreateWork(source, overlappingState, "work-overlap", now); err == nil {
		t.Fatal("output workspace nested inside source was accepted")
	}
	if _, err := os.Stat(overlappingState); !os.IsNotExist(err) {
		t.Fatalf("rejected overlapping state path was created: %v", err)
	}
}

func TestOpenWorkRejectsTamperedMetadata(t *testing.T) {
	t.Parallel()

	work, err := CreateWork(t.TempDir(), t.TempDir(), "work-tamper", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(work.Path, "workspace.json")
	if err := os.WriteFile(metadataPath, []byte(`{"version":1,"id":"other","source":"/tmp","created_at":"2026-09-08T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWork(work.Path); err == nil {
		t.Fatal("tampered work metadata was accepted")
	}
}
