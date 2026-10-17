package workrun

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
)

func TestReadInteractionRecordsValidatesAndScopesRetainedDecisions(t *testing.T) {
	state := t.TempDir()
	base := filepath.Join(state, "gator", "interactions")
	for _, run := range []string{"run-one", "run-two"} {
		if err := os.MkdirAll(filepath.Join(base, run), 0700); err != nil {
			t.Fatal(err)
		}
	}
	approved := false
	for _, item := range []struct {
		run    string
		record InteractionRecord
	}{
		{"run-one", InteractionRecord{Version: 1, Status: "pending", Interaction: Interaction{ID: 1, Kind: "action", Preview: "send"}}},
		{"run-two", InteractionRecord{Version: 1, Status: "resolved", Interaction: Interaction{ID: 1, Kind: "browser", Preview: "open"}, Approved: &approved}},
	} {
		data, err := json.Marshal(item.record)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(base, item.run, "1.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	all, err := ReadInteractionRecords(state, "")
	if err != nil || len(all) != 2 || all[0].RunID != "run-two" || all[1].RunID != "run-one" {
		t.Fatalf("all records = %#v, %v", all, err)
	}
	one, err := ReadInteractionRecords(state, "run-one")
	if err != nil || len(one) != 1 || one[0].Status != "pending" || one[0].LiveStatus != "unknown" {
		t.Fatalf("scoped records = %#v, %v", one, err)
	}
	if _, err := ReadInteractionRecords(state, "../outside"); err == nil {
		t.Fatal("accepted escaping run ID")
	}
	if err := os.WriteFile(filepath.Join(base, "run-one", "2.json"), []byte(`{"version":1,"status":"pending","interaction":{"id":1,"kind":"action"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadInteractionRecords(state, "run-one"); err == nil {
		t.Fatal("accepted interaction ID mismatch")
	}
}

func TestWorkRejectsEscapingRunIDBeforeWritingState(t *testing.T) {
	state := t.TempDir()
	_, err := (Executor{Model: &scriptedModel{turns: []agent.Turn{{Text: "done"}}}, StateDir: state}).Execute(context.Background(), Request{
		RunID: "../escape", SourcePath: t.TempDir(), Objective: "inspect", Mode: action.Inspect, Contract: artifact.InspectionContract(),
	})
	if err == nil {
		t.Fatal("accepted escaping Work run ID")
	}
	if _, err := os.Stat(filepath.Join(state, "escape")); !os.IsNotExist(err) {
		t.Fatalf("unexpected state outside Work directory: %v", err)
	}
}

func TestInteractionRunLockReportsActiveThenStopped(t *testing.T) {
	state := t.TempDir()
	lock, err := acquireInteractionRunLock(state, "work-live")
	if err != nil {
		t.Fatal(err)
	}
	if second, err := acquireInteractionRunLock(state, "work-live"); err == nil {
		second.Close()
		t.Fatal("concurrent Work run acquired the same lock")
	}
	root, err := os.OpenRoot(filepath.Join(state, "gator", "interactions"))
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if status, err := interactionRunLiveness(root, "work-live"); err != nil || status != "active" {
		t.Fatalf("live lock = %q, %v", status, err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if status, err := interactionRunLiveness(root, "work-live"); err != nil || status != "stopped" {
		t.Fatalf("released lock = %q, %v", status, err)
	}
}
