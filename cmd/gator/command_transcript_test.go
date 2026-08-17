package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/journal"
)

func TestExportTranscriptWritesEscapedLocalHTML(t *testing.T) {
	directory := t.TempDir()
	entry, record, err := journal.Open(directory, "run-transcript", filepath.Join(directory, "worktree"), t.TempDir(), time.Now())
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	if err := entry.SaveSession(journal.Session{
		Version: 2, Repository: directory, WorktreePath: filepath.Join(directory, "worktree"), Provider: "openai", Model: "test", Task: "Review <script>", ThreadID: "thread-1", Mode: "execute",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "Inspect <unsafe>"}, {Role: agent.RoleAgent, Content: "Done", ToolCalls: []agent.ToolCall{{Name: "git_diff"}}}},
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if err := entry.Close(); err != nil {
		t.Fatalf("close journal: %v", err)
	}
	var output bytes.Buffer
	if err := exportTranscript([]string{record.StatePath}, &output); err != nil {
		t.Fatalf("export transcript: %v", err)
	}
	if !strings.Contains(output.String(), "<!doctype html>") || !strings.Contains(output.String(), "&lt;unsafe&gt;") || strings.Contains(output.String(), "<unsafe>") || !strings.Contains(output.String(), "Tool calls: git_diff") {
		t.Fatalf("transcript = %q", output.String())
	}
}
