package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/jobs"
)

func TestJobCommandCreatesDurableInspectSchedule(t *testing.T) {
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	var output bytes.Buffer
	if err := jobCommand([]string{"add", "daily", "--schedule", "0 9 * * *", "--timezone", "Asia/Singapore", "--mode", "inspect", "--", "Summarize the folder"}, &output); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := jobCommand([]string{"list"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "daily") || !strings.Contains(output.String(), "Asia/Singapore") {
		t.Fatalf("list = %q", output.String())
	}
}

func TestJobCommandRejectsUnattendedApproval(t *testing.T) {
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	err := jobCommand([]string{"add", "unsafe", "--schedule", "* * * * *", "--actions", "approve", "--", "Send this"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("scheduled approval was accepted")
	}
}

func TestJobEditRecapturesExplicitFrozenSource(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	store, err := jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	definition, err := store.Save(jobs.Definition{Name: "frozen", Enabled: true, Schedule: "* * * * *", SourcePath: source, Objective: "Inspect", Mode: action.Inspect, Contract: artifact.InspectionContract()})
	if err != nil {
		t.Fatal(err)
	}
	original := definition.SnapshotID
	if err := os.WriteFile(filepath.Join(source, "new.txt"), []byte("changed source"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := jobCommand([]string{"edit", definition.ID, "--name", "renamed"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	unchanged, err := store.Load(definition.ID)
	if err != nil || unchanged.SnapshotID != original {
		t.Fatalf("unrelated edit changed capture: %+v, %v", unchanged, err)
	}
	for _, selected := range []string{source, t.TempDir()} {
		if err := jobCommand([]string{"edit", definition.ID, "--source", selected}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		updated, err := store.Load(definition.ID)
		if err != nil || updated.SnapshotID == original || updated.SourcePath != selected || updated.Project == nil || updated.Project.Origin != selected {
			t.Fatalf("explicit source edit did not recapture source/configuration: %+v, %v", updated, err)
		}
		original = updated.SnapshotID
	}
}
