package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/knowledge"
)

func TestLocalJobInputHashTracksFilesPlanAndNotes(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "agenda.md"), []byte("Venue A"), 0600); err != nil {
		t.Fatal(err)
	}
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Plan offsite", "", source)
	if err != nil {
		t.Fatal(err)
	}
	definition := jobs.Definition{ChangeOnly: true, RefreshSnapshot: true, GoalID: record.ID, SourcePath: source, Objective: "Check plan"}
	initial, err := localJobInputHash(definition, state)
	if err != nil {
		t.Fatal(err)
	}
	again, err := localJobInputHash(definition, state)
	if err != nil || initial != again {
		t.Fatalf("stable hash changed: %s %s %v", initial, again, err)
	}
	if err := os.WriteFile(filepath.Join(source, "agenda.md"), []byte("Venue B"), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := localJobInputHash(definition, state)
	if err != nil || changed == initial {
		t.Fatalf("source edit did not change hash: %s %s %v", initial, changed, err)
	}
	if _, err := goals.ApplyPlanChange(record.ID, goal.PlanChange{Kind: "add_step", ExpectedRevision: 0, Title: "Confirm capacity"}); err != nil {
		t.Fatal(err)
	}
	withPlan, err := localJobInputHash(definition, state)
	if err != nil || withPlan == changed {
		t.Fatalf("plan edit did not change hash: %s %s %v", changed, withPlan, err)
	}
	notes, err := knowledge.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := notes.AddNote("Venue", "Room holds 90", record.ID); err != nil {
		t.Fatal(err)
	}
	withNote, err := localJobInputHash(definition, state)
	if err != nil || withNote == withPlan {
		t.Fatalf("note did not change hash: %s %s %v", withPlan, withNote, err)
	}
}

func TestChangeOnlySkipsOnlyAfterMatchingSuccessfulAttempt(t *testing.T) {
	current := jobs.Attempt{ID: "current", InputSHA256: "same"}
	completed := jobs.Attempt{ID: "old", Status: "completed", InputSHA256: "same", NotificationPolicySHA256: "policy"}
	if !unchangedJobInput([]jobs.Attempt{current, completed}, current, "policy") {
		t.Fatal("matching successful input was not skipped")
	}
	if unchangedJobInput([]jobs.Attempt{{Status: "failed"}, completed}, current, "policy") || unchangedJobInput([]jobs.Attempt{completed}, current, "changed-policy") {
		t.Fatal("failure or changed policy was suppressed")
	}
	d := jobs.Definition{Version: jobs.Version, ID: "job-000000000000000000000000", Name: "daily", Enabled: true, Schedule: "0 9 * * *", Timezone: "UTC", Missed: "skip", SourcePath: t.TempDir(), Objective: "Check", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 1, ChangeOnly: true, RefreshSnapshot: true, ConnectorIDs: []string{"google"}}
	if err := d.Validate(); err == nil {
		t.Fatal("connected job accepted change-only gate")
	}
}
