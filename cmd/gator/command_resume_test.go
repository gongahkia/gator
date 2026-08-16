package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/journal"
)

func TestResolveResumeThreadUsesLatestProjectThread(t *testing.T) {
	stateDirectory := t.TempDir()
	repository := "/workspace/current-project"
	older := saveResumeThread(t, stateDirectory, repository, "thread-older", time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC))
	newer := saveResumeThread(t, stateDirectory, repository, "thread-newer", time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC))
	_ = saveResumeThread(t, stateDirectory, "/workspace/other-project", "thread-other", time.Date(2026, 8, 16, 11, 0, 0, 0, time.UTC))

	selected, err := resolveResumeThread(stateDirectory, repository, "", true, false)
	if err != nil {
		t.Fatalf("resolve latest thread: %v", err)
	}
	if selected.ID != newer.ID || selected.HeadStatePath != newer.HeadStatePath {
		t.Fatalf("selected = %#v, want %#v", selected, newer)
	}
	if selected.ID == older.ID {
		t.Fatal("selected an older project thread")
	}
}

func TestResolveResumeThreadSupportsUniquePrefixAcrossProjects(t *testing.T) {
	stateDirectory := t.TempDir()
	_ = saveResumeThread(t, stateDirectory, "/workspace/current-project", "thread-current", time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC))
	want := saveResumeThread(t, stateDirectory, "/workspace/other-project", "thread-other-unique", time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC))

	selected, err := resolveResumeThread(stateDirectory, "/workspace/current-project", "thread-other-u", false, true)
	if err != nil {
		t.Fatalf("resolve cross-project prefix: %v", err)
	}
	if selected.ID != want.ID || selected.Repository != want.Repository {
		t.Fatalf("selected = %#v, want %#v", selected, want)
	}
}

func TestResolveResumeThreadRejectsUnavailableWorktree(t *testing.T) {
	stateDirectory := t.TempDir()
	repository := "/workspace/current-project"
	if err := journal.SaveThread(stateDirectory, journal.Thread{
		Version:       1,
		ID:            "thread-missing",
		Repository:    repository,
		WorktreePath:  filepath.Join(t.TempDir(), "missing"),
		Provider:      "openai",
		Task:          "Missing worktree",
		HeadStatePath: "/state/missing",
		TurnCount:     1,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}); err != nil {
		t.Fatalf("save missing thread: %v", err)
	}
	if _, err := resolveResumeThread(stateDirectory, repository, "", true, false); err == nil {
		t.Fatal("latest unavailable thread resumed")
	}
}

func TestLooksLikeRunRecordPathRecognizesExplicitPaths(t *testing.T) {
	directory := t.TempDir()
	if !looksLikeRunRecordPath(directory) || !looksLikeRunRecordPath("state/run-001") {
		t.Fatal("run record paths were not recognized")
	}
	if looksLikeRunRecordPath("thread-001") {
		t.Fatal("thread ID was treated as a path")
	}
}

func saveResumeThread(t *testing.T, stateDirectory, repository, id string, updatedAt time.Time) journal.RecentThread {
	t.Helper()
	worktreePath := filepath.Join(t.TempDir(), id)
	if err := os.Mkdir(worktreePath, 0o700); err != nil {
		t.Fatal(err)
	}
	thread := journal.Thread{
		Version:       1,
		ID:            id,
		Repository:    repository,
		WorktreePath:  worktreePath,
		Provider:      "openai",
		Task:          "Resume " + id,
		HeadStatePath: filepath.Join(stateDirectory, id),
		TurnCount:     1,
		CreatedAt:     updatedAt.Add(-time.Minute),
		UpdatedAt:     updatedAt,
	}
	if err := journal.SaveThread(stateDirectory, thread); err != nil {
		t.Fatalf("save thread: %v", err)
	}
	return journal.RecentThread{
		ID:            thread.ID,
		Repository:    thread.Repository,
		HeadStatePath: thread.HeadStatePath,
	}
}
