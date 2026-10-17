package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/workrun"
)

func TestMemoryCLIAndPromptProjection(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	var output bytes.Buffer
	if err := run([]string{"goal", "add", "Plan launch"}, &output); err != nil {
		t.Fatal(err)
	}
	goalID := strings.Fields(output.String())[0]
	output.Reset()
	if err := run([]string{"memory", "add", "--goal", goalID, "--kind", "decision", "--source", "planning call", "Present the launch deck on Monday"}, &output); err != nil {
		t.Fatal(err)
	}
	memoryID := strings.Fields(output.String())[0]
	request := workrun.Request{GoalID: goalID, Objective: "What day should the launch deck be presented?"}
	if err := applyMemoryContext(state, &request); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(request.MemoryContext, memoryID) || !strings.Contains(request.MemoryContext, "Monday") {
		t.Fatalf("projection = %q", request.MemoryContext)
	}
	output.Reset()
	if err := run([]string{"memory", "correct", memoryID, "Present the launch deck on Tuesday"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := applyMemoryContext(state, &request); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(request.MemoryContext, "Monday") || !strings.Contains(request.MemoryContext, "Tuesday") {
		t.Fatalf("corrected projection = %q", request.MemoryContext)
	}
	output.Reset()
	if err := run([]string{"memory", "forget", memoryID}, &output); err != nil {
		t.Fatal(err)
	}
	if err := applyMemoryContext(state, &request); err != nil || request.MemoryContext != "" {
		t.Fatalf("forgotten projection = %q, %v", request.MemoryContext, err)
	}
}

func TestMemoryCLIFileSourceRequiresReviewAfterChange(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	path := filepath.Join(t.TempDir(), "brief.txt")
	if err := os.WriteFile(path, []byte("Daily brief at 9am"), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run([]string{"memory", "add", "--kind", "preference", "--source-file", path, "Daily brief at 9am"}, &output); err != nil {
		t.Fatal(err)
	}
	id := strings.Fields(output.String())[0]
	request := workrun.Request{Objective: "When is the daily brief?"}
	if err := applyMemoryContext(state, &request); err != nil || !strings.Contains(request.MemoryContext, id) {
		t.Fatalf("initial memory = %q, %v", request.MemoryContext, err)
	}
	if err := os.WriteFile(path, []byte("Daily brief at 8am"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := applyMemoryContext(state, &request); err != nil || request.MemoryContext != "" {
		t.Fatalf("changed source remained in prompt: %q, %v", request.MemoryContext, err)
	}
	output.Reset()
	if err := run([]string{"memory", "show", id}, &output); err != nil || !strings.Contains(output.String(), "Source status: changed") {
		t.Fatalf("memory status = %q, %v", output.String(), err)
	}
	if err := run([]string{"memory", "correct", id, "--source-file", path, "Daily brief at 8am"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := applyMemoryContext(state, &request); err != nil || !strings.Contains(request.MemoryContext, "8am") {
		t.Fatalf("reviewed memory = %q, %v", request.MemoryContext, err)
	}
}
