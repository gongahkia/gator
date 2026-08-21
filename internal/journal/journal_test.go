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
	if err := journal.Append(agent.Event{
		Kind: agent.EventCommandApprovalRequested,
		At:   now,
		Step: 3,
		Argv: []string{"echo", "secret-argv"},
		Text: "echo secret-argv",
	}); err != nil {
		t.Fatalf("append approval event: %v", err)
	}
	if err := journal.Finish("completed", "Feature ready.", now.Add(time.Minute)); err != nil {
		t.Fatalf("finish journal: %v", err)
	}

	events, err := os.ReadFile(filepath.Join(record.StatePath, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(events), "secret source text") || strings.Contains(string(events), "secret-argv") || !strings.Contains(string(events), "apply_patch") {
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

func TestJournalRoundTripsAllowedCommands(t *testing.T) {
	journal, record, err := Open("/workspace/project", "run-allowed", "/runs/run-allowed", t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	session := Session{
		Version:         2,
		Repository:      "/workspace/project",
		WorktreePath:    "/runs/run-allowed",
		Provider:        "openai",
		Task:            "Explore then verify",
		Messages:        []agent.Message{{Role: agent.RoleUser, Content: "Explore then verify"}},
		AllowedCommands: [][]string{{"/bin/bash", "-lc", "go test ./internal/foo"}},
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
	if len(loaded.AllowedCommands) != 1 || strings.Join(loaded.AllowedCommands[0], " ") != "/bin/bash -lc go test ./internal/foo" {
		t.Fatalf("loaded allowed commands = %#v", loaded.AllowedCommands)
	}
}

func TestSessionOmitsRawAttachmentBytesAndKeepsManifest(t *testing.T) {
	journal, record, err := Open("/workspace/project", "run-attachments", "/runs/run-attachments", t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	const contents = "attachment bytes must not be persisted"
	session := Session{
		Version:      2,
		Repository:   "/workspace/project",
		WorktreePath: "/runs/run-attachments",
		Provider:     "openai",
		Task:         "Review the attachment",
		Messages: []agent.Message{{
			Role:        agent.RoleUser,
			Content:     "Review the attachment",
			Attachments: []agent.Attachment{{Name: "report.pdf", MediaType: "application/pdf", Data: []byte(contents)}},
			Images:      []agent.Image{{Name: "screen.png", MediaType: "image/png", Data: []byte(contents)}},
		}},
	}
	if err := journal.SaveSession(session); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if err := journal.Close(); err != nil {
		t.Fatalf("close journal: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(record.StatePath, "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), contents) {
		t.Fatalf("session persisted raw attachment bytes: %s", raw)
	}
	loaded, err := LoadSession(record.StatePath)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if len(loaded.Messages) != 1 || len(loaded.Messages[0].Images) != 0 || len(loaded.Messages[0].Attachments) != 0 || len(loaded.AttachmentManifest) != 2 {
		t.Fatalf("loaded sanitized session = %#v", loaded)
	}
	for _, attachment := range loaded.AttachmentManifest {
		if attachment.SHA256 == "" || attachment.Bytes != len(contents) {
			t.Fatalf("attachment manifest = %#v", loaded.AttachmentManifest)
		}
	}
}

func TestLoadSessionScrubsLegacyRawAttachmentBytes(t *testing.T) {
	journal, record, err := Open("/workspace/project", "run-legacy-attachments", "/runs/run-legacy-attachments", t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	if err := journal.Close(); err != nil {
		t.Fatalf("close journal: %v", err)
	}
	const contents = "legacy raw attachment bytes"
	legacy := Session{
		Version:      2,
		Repository:   "/workspace/project",
		WorktreePath: "/runs/run-legacy-attachments",
		Provider:     "openai",
		Task:         "Review the attachment",
		Messages: []agent.Message{{
			Role:        agent.RoleUser,
			Content:     "Review the attachment",
			Attachments: []agent.Attachment{{Name: "report.pdf", MediaType: "application/pdf", Data: []byte(contents)}},
		}},
	}
	if err := writeJSON(filepath.Join(record.StatePath, "session.json"), legacy); err != nil {
		t.Fatalf("write legacy session: %v", err)
	}
	loaded, err := LoadSession(record.StatePath)
	if err != nil {
		t.Fatalf("load legacy session: %v", err)
	}
	if len(loaded.Messages) != 1 || len(loaded.Messages[0].Attachments) != 0 || len(loaded.AttachmentManifest) != 1 {
		t.Fatalf("loaded legacy session = %#v", loaded)
	}
	raw, err := os.ReadFile(filepath.Join(record.StatePath, "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), contents) {
		t.Fatalf("legacy raw attachment bytes remained on disk: %s", raw)
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

func TestListAllRecentThreadsIncludesProjectsAndPreservesRepositoryMetadata(t *testing.T) {
	stateDirectory := t.TempDir()
	firstWorktree := filepath.Join(t.TempDir(), "first")
	secondWorktree := filepath.Join(t.TempDir(), "second")
	for _, worktree := range []string{firstWorktree, secondWorktree} {
		if err := os.Mkdir(worktree, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	firstRepository := "/workspace/first-project"
	secondRepository := "/workspace/second-project"
	first := Thread{
		Version:       threadVersion,
		ID:            "thread-first",
		Repository:    firstRepository,
		WorktreePath:  firstWorktree,
		Provider:      "openai",
		Task:          "First project task",
		HeadStatePath: "/state/first",
		TurnCount:     1,
		CreatedAt:     time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC),
	}
	second := Thread{
		Version:       threadVersion,
		ID:            "thread-second",
		Repository:    secondRepository,
		WorktreePath:  secondWorktree,
		Provider:      "anthropic",
		Task:          "Second project task",
		HeadStatePath: "/state/second",
		TurnCount:     2,
		CreatedAt:     time.Date(2026, 8, 16, 11, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 8, 16, 11, 0, 0, 0, time.UTC),
	}
	for _, thread := range []Thread{first, second} {
		if err := SaveThread(stateDirectory, thread); err != nil {
			t.Fatalf("save thread %s: %v", thread.ID, err)
		}
	}

	recent, err := ListAllRecentThreads(stateDirectory, 10)
	if err != nil {
		t.Fatalf("list all recent threads: %v", err)
	}
	if len(recent) != 2 || recent[0].ID != second.ID || recent[1].ID != first.ID {
		t.Fatalf("recent threads = %#v", recent)
	}
	if recent[0].Repository != secondRepository || recent[1].Repository != firstRepository || !recent[0].Available || !recent[1].Available {
		t.Fatalf("recent thread metadata = %#v", recent)
	}
}

func TestListThreadForksGroupsIndependentBranchesBySourceTurn(t *testing.T) {
	stateDirectory := t.TempDir()
	repository := "/workspace/project"
	worktree := filepath.Join(t.TempDir(), "retained")
	if err := os.Mkdir(worktree, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, thread := range []Thread{
		{Version: threadVersion, ID: "source-thread", Repository: repository, WorktreePath: worktree, Provider: "openai", Task: "Source", HeadStatePath: "/state/source", TurnCount: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{Version: threadVersion, ID: "fork-one", Repository: repository, WorktreePath: worktree, Provider: "openai", Task: "Fork one", HeadStatePath: "/state/fork-one", ForkedFrom: "/state/source", TurnCount: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{Version: threadVersion, ID: "fork-two", Repository: repository, WorktreePath: worktree, Provider: "openai", Task: "Fork two", HeadStatePath: "/state/fork-two", ForkedFrom: "/state/source", TurnCount: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	} {
		if err := SaveThread(stateDirectory, thread); err != nil {
			t.Fatalf("save %s: %v", thread.ID, err)
		}
	}
	forks, err := ListThreadForks(stateDirectory, repository)
	if err != nil {
		t.Fatalf("list forks: %v", err)
	}
	if len(forks["/state/source"]) != 2 {
		t.Fatalf("forks = %#v", forks)
	}
}

func TestLoadThreadLineageReturnsChronologicalPrivateTurnSummaries(t *testing.T) {
	stateDirectory := t.TempDir()
	repository := "/workspace/project"
	worktreePath := filepath.Join(t.TempDir(), "retained")
	if err := os.Mkdir(worktreePath, 0o700); err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	create := func(runID, parent, instruction, status, finalText string, at time.Time) Record {
		t.Helper()
		entry, record, err := Open(repository, runID, worktreePath, stateDirectory, at)
		if err != nil {
			t.Fatalf("open %s: %v", runID, err)
		}
		messages := []agent.Message{{Role: agent.RoleUser, Content: "Implement the retained feature"}}
		if instruction != "" {
			messages = append(messages, agent.Message{Role: agent.RoleUser, Content: "Continue the original task with this developer instruction:\n" + instruction})
		}
		if err := entry.SaveSession(Session{
			Version:         2,
			Repository:      repository,
			WorktreePath:    worktreePath,
			Provider:        "openai",
			Model:           "gpt-5.6",
			Task:            "Implement the retained feature",
			ThreadID:        "thread-001",
			Mode:            "execute",
			Messages:        messages,
			ParentStatePath: parent,
		}); err != nil {
			t.Fatalf("save %s: %v", runID, err)
		}
		if err := entry.Finish(status, finalText, at.Add(time.Minute)); err != nil {
			t.Fatalf("finish %s: %v", runID, err)
		}
		return record
	}

	first := create("run-thread-001", "", "", "completed", "First turn complete.", started)
	second := create("run-thread-002", first.StatePath, "Address the verifier failure", "failed", "Verifier still fails.", started.Add(2*time.Minute))
	turns, err := LoadThreadLineage(second.StatePath)
	if err != nil {
		t.Fatalf("load lineage: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("turn count = %d, want 2", len(turns))
	}
	if turns[0].StatePath != first.StatePath || turns[0].Task != "Implement the retained feature" || turns[0].Status != "completed" || turns[0].FinalText != "First turn complete." {
		t.Fatalf("first turn = %#v", turns[0])
	}
	if turns[1].StatePath != second.StatePath || turns[1].Task != "Address the verifier failure" || turns[1].Status != "failed" || turns[1].FinishedAt.IsZero() {
		t.Fatalf("second turn = %#v", turns[1])
	}
}

func TestLoadThreadLineageRejectsMismatchedThreadIdentity(t *testing.T) {
	stateDirectory := t.TempDir()
	repository := "/workspace/project"
	worktreePath := filepath.Join(t.TempDir(), "retained")
	if err := os.Mkdir(worktreePath, 0o700); err != nil {
		t.Fatal(err)
	}
	create := func(runID, threadID, parent string) Record {
		t.Helper()
		entry, record, err := Open(repository, runID, worktreePath, stateDirectory, time.Now())
		if err != nil {
			t.Fatalf("open %s: %v", runID, err)
		}
		if err := entry.SaveSession(Session{Version: 2, Repository: repository, WorktreePath: worktreePath, Provider: "openai", Task: "Retained task", ThreadID: threadID, ParentStatePath: parent}); err != nil {
			t.Fatalf("save %s: %v", runID, err)
		}
		if err := entry.Close(); err != nil {
			t.Fatalf("close %s: %v", runID, err)
		}
		return record
	}
	first := create("run-thread-root", "thread-root", "")
	second := create("run-thread-head", "thread-other", first.StatePath)
	if _, err := LoadThreadLineage(second.StatePath); err == nil || !strings.Contains(err.Error(), "crosses thread identities") {
		t.Fatalf("lineage error = %v", err)
	}
}
