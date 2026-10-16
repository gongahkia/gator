package main

import (
	"bytes"
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
