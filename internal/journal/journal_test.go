package journal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
)

func TestJournalRecordsBoundedEventMetadata(t *testing.T) {
	stateDirectory := t.TempDir()
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	journal, record, err := Open("/workspace/project", "run-001", "/runs/run-001", stateDirectory, now)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	event := agent.Event{
		Kind: agent.EventToolCalled,
		At:   now,
		Step: 2,
		ToolCall: &agent.ToolCall{
			ID:        "call-1",
			Name:      "apply_patch",
			Arguments: json.RawMessage(`{"patch":"secret source text"}`),
		},
	}
	if err := journal.Append(event); err != nil {
		t.Fatalf("append event: %v", err)
	}
	if err := journal.Finish("completed", "Feature ready.", now.Add(time.Minute)); err != nil {
		t.Fatalf("finish journal: %v", err)
	}

	events, err := os.ReadFile(filepath.Join(record.StatePath, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(events), "secret source text") || !strings.Contains(string(events), "apply_patch") {
		t.Fatalf("event journal = %s", events)
	}
	result, err := os.ReadFile(filepath.Join(record.StatePath, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), "Feature ready.") || !strings.Contains(string(result), "completed") {
		t.Fatalf("result journal = %s", result)
	}
}

func TestJournalRejectsUnsafeRunID(t *testing.T) {
	_, _, err := Open("/workspace/project", "../escape", "/runs/escape", t.TempDir(), time.Now())
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("unsafe run id error = %v", err)
	}
}

func TestJournalSavesAndLoadsPrivateSession(t *testing.T) {
	journal, record, err := Open("/workspace/project", "run-002", "/runs/run-002", t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	session := Session{
		Version:      2,
		Repository:   "/workspace/project",
		WorktreePath: "/runs/run-002",
		Provider:     "openai",
		Model:        "test-model",
		Task:         "Add a feature",
		Messages:     []agent.Message{{Role: agent.RoleUser, Content: "Add a feature"}},
	}
	if err := journal.SaveSession(session); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if err := journal.Close(); err != nil {
		t.Fatalf("close journal: %v", err)
	}
	loaded, err := LoadSession(record.StatePath)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if loaded.Model != "test-model" || loaded.Messages[0].Content != "Add a feature" {
		t.Fatalf("loaded session = %#v", loaded)
	}
	info, err := os.Stat(filepath.Join(record.StatePath, "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("session permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestDraftRoundTripUsesPrivateStateAndDeletes(t *testing.T) {
	stateDirectory := t.TempDir()
	draft := Draft{
		Repository:   "/workspace/project",
		Task:         "Add a focused feature",
		Verification: "go test ./...",
		Provider:     "anthropic",
		Model:        "claude-sonnet-5",
		UpdatedAt:    time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC),
	}
	if err := SaveDraft(stateDirectory, draft); err != nil {
		t.Fatalf("save draft: %v", err)
	}
	loaded, found, err := LoadDraft(stateDirectory, draft.Repository)
	if err != nil {
		t.Fatalf("load draft: %v", err)
	}
	if !found || loaded.Task != draft.Task || loaded.Provider != draft.Provider || loaded.Verification != "go test ./..." {
		t.Fatalf("loaded draft = %#v, found = %t", loaded, found)
	}
	path := filepath.Join(stateDirectory, "gator", "drafts", repositoryFingerprint(draft.Repository)+".json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("draft permissions = %o, want 600", info.Mode().Perm())
	}
	if err := DeleteDraft(stateDirectory, draft.Repository); err != nil {
		t.Fatalf("delete draft: %v", err)
	}
	if _, found, err := LoadDraft(stateDirectory, draft.Repository); err != nil || found {
		t.Fatalf("draft after delete = found %t, error %v", found, err)
	}
}

func TestListRecentRunsSortsSessionsAndMarksMissingWorktrees(t *testing.T) {
	stateDirectory := t.TempDir()
	repository := "/workspace/project"
	create := func(runID, worktree string) Record {
		t.Helper()
		journal, record, err := Open(repository, runID, worktree, stateDirectory, time.Now())
		if err != nil {
			t.Fatalf("open %s: %v", runID, err)
		}
		if err := journal.SaveSession(Session{Version: 2, Repository: repository, WorktreePath: worktree, Provider: "openai", Task: "Task " + runID}); err != nil {
			t.Fatalf("save %s: %v", runID, err)
		}
		if err := journal.Close(); err != nil {
			t.Fatalf("close %s: %v", runID, err)
		}
		return record
	}
	availableWorktree := filepath.Join(t.TempDir(), "retained")
	if err := os.Mkdir(availableWorktree, 0o700); err != nil {
		t.Fatal(err)
	}
	first := create("run-001", availableWorktree)
	second := create("run-002", filepath.Join(t.TempDir(), "missing"))
	if err := os.Chtimes(filepath.Join(first.StatePath, "session.json"), time.Now().Add(-time.Minute), time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	runs, err := ListRecentRuns(stateDirectory, repository, 10)
	if err != nil {
		t.Fatalf("list recent runs: %v", err)
	}
	if len(runs) != 2 || runs[0].StatePath != second.StatePath || runs[1].StatePath != first.StatePath {
		t.Fatalf("recent runs = %#v", runs)
	}
	if !runs[1].Available || runs[0].Available {
		t.Fatalf("availability = %#v", runs)
	}
}

func TestThreadRoundTripAndRecentList(t *testing.T) {
	stateDirectory := t.TempDir()
	repository := "/workspace/project"
	worktreePath := filepath.Join(t.TempDir(), "retained")
	if err := os.Mkdir(worktreePath, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	thread := Thread{
		Version:       1,
		ID:            "thread-001",
		Repository:    repository,
		WorktreePath:  worktreePath,
		Provider:      "openai",
		Model:         "test-model",
		Task:          "Plan a focused change",
		HeadStatePath: "/state/thread-001",
		TurnCount:     2,
		CreatedAt:     now.Add(-time.Minute),
		UpdatedAt:     now,
	}
	if err := SaveThread(stateDirectory, thread); err != nil {
		t.Fatalf("save thread: %v", err)
	}
	loaded, err := LoadThread(stateDirectory, repository, thread.ID)
	if err != nil {
		t.Fatalf("load thread: %v", err)
	}
	if loaded.ID != thread.ID || loaded.TurnCount != 2 || loaded.WorktreePath != worktreePath {
		t.Fatalf("loaded thread = %#v", loaded)
	}
	path := filepath.Join(stateDirectory, "gator", "threads", repositoryFingerprint(repository), thread.ID+".json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("thread permissions = %o, want 600", info.Mode().Perm())
	}
	recent, err := ListRecentThreads(stateDirectory, repository, 10)
	if err != nil {
		t.Fatalf("list recent threads: %v", err)
	}
	if len(recent) != 1 || recent[0].ID != thread.ID || !recent[0].Available || recent[0].TurnCount != 2 {
		t.Fatalf("recent threads = %#v", recent)
	}
}

func TestListRecentThreadsFallsBackToLegacyRuns(t *testing.T) {
	stateDirectory := t.TempDir()
	repository := "/workspace/project"
	worktreePath := filepath.Join(t.TempDir(), "retained")
	if err := os.Mkdir(worktreePath, 0o700); err != nil {
		t.Fatal(err)
	}
	entry, record, err := Open(repository, "run-legacy-001", worktreePath, stateDirectory, time.Now())
	if err != nil {
		t.Fatalf("open legacy run: %v", err)
	}
	if err := entry.SaveSession(Session{Version: 2, Repository: repository, WorktreePath: worktreePath, Provider: "openai", Task: "Legacy task"}); err != nil {
		t.Fatalf("save legacy session: %v", err)
	}
	if err := entry.Close(); err != nil {
		t.Fatalf("close legacy run: %v", err)
	}
	recent, err := ListRecentThreads(stateDirectory, repository, 10)
	if err != nil {
		t.Fatalf("list recent threads: %v", err)
	}
	if len(recent) != 1 || recent[0].ID != "run-legacy-001" || recent[0].HeadStatePath != record.StatePath || !recent[0].Available {
		t.Fatalf("legacy recent threads = %#v", recent)
	}
}
