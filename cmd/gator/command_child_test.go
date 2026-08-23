package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/journal"
)

func TestChildCommandListsAndShowsDurableWriterManifest(t *testing.T) {
	now := time.Date(2026, 8, 23, 13, 0, 0, 0, time.UTC)
	entry, record, err := journal.Open("/workspace/project", "run-parent-cli", "/runs/run-parent-cli", t.TempDir(), now)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	t.Cleanup(func() { _ = entry.Close() })
	finished := now.Add(time.Minute)
	manifest := journal.ChildManifest{
		Version:          1,
		ID:               "run-child-cli",
		ParentRunID:      "run-parent-cli",
		Kind:             "writer",
		Status:           journal.ChildCompleted,
		Repository:       "/workspace/project",
		WorktreePath:     "/runs/run-child-cli",
		BaseCommit:       "abcdef0",
		Role:             "test-fixer",
		TaskSHA256:       strings.Repeat("a", 64),
		StartedAt:        now,
		UpdatedAt:        finished,
		FinishedAt:       &finished,
		StatePath:        "/state/run-child-cli",
		PatchSHA256:      strings.Repeat("b", 64),
		PatchBytes:       42,
		PatchAvailable:   true,
		ReviewRequired:   true,
		WorktreeRetained: true,
	}
	if err := entry.SaveChildManifest(manifest); err != nil {
		t.Fatalf("save child manifest: %v", err)
	}
	var output bytes.Buffer
	if err := childCommand([]string{"list", record.StatePath}, &output); err != nil {
		t.Fatalf("list child manifests: %v", err)
	}
	if got := output.String(); !strings.Contains(got, "run-child-cli") || !strings.Contains(got, "completed") || !strings.Contains(got, "role=test-fixer") || !strings.Contains(got, "patch=42 bytes") {
		t.Fatalf("child list = %q", got)
	}
	output.Reset()
	if err := childCommand([]string{"show", record.StatePath, "run-child-cli"}, &output); err != nil {
		t.Fatalf("show child manifest: %v", err)
	}
	if got := output.String(); !strings.Contains(got, "task sha256: "+manifest.TaskSHA256) || !strings.Contains(got, "child record: /state/run-child-cli") || !strings.Contains(got, "42 bytes (available to parent)") {
		t.Fatalf("child show = %q", got)
	}
}
