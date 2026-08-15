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
