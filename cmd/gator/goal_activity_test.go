package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/suggestion"
)

func TestGoalActivityShowsDurableChecksAndPendingDecisions(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	goals, err := goal.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Plan the offsite", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := goals.InitializePlan(record.ID, "Choose a venue", []string{"Confirm availability"}); err != nil {
		t.Fatal(err)
	}
	jobStore, err := jobs.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := jobStore.Save(jobs.Definition{Name: "Offsite watch", GoalID: record.ID, Enabled: true, Schedule: "0 9 * * *", Timezone: "UTC", Missed: "run_once", SourcePath: record.SourcePath, RefreshSnapshot: true, Objective: "Check venue changes", Mode: action.Inspect, Contract: artifact.InspectionContract()})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := jobStore.Begin(definition, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	attempt.Status, attempt.Summary, attempt.FinishedAt = "completed", "Venue A cancelled", time.Now().UTC()
	if err := jobStore.Record(attempt); err != nil {
		t.Fatal(err)
	}
	inboxStore, err := inbox.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inboxStore.Add(inbox.Entry{GoalID: record.ID, Title: "Offsite", Status: "completed", Summary: "Review venue cancellation"}); err != nil {
		t.Fatal(err)
	}
	if _, err := inboxStore.Add(inbox.Entry{Title: "Other", Status: "completed", Summary: "Unrelated private alert"}); err != nil {
		t.Fatal(err)
	}
	suggestions, err := suggestion.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := suggestions.Create(suggestion.Record{Kind: "note", WorkID: "work-test", GoalID: record.ID, Title: "Venue fallback", Body: "Venue B is available"}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := goalCommand([]string{"activity", record.ID}, &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Outcome: Choose a venue", "Step:", "next:", "last:", "Venue A cancelled", "Review venue cancellation", "Venue fallback", "gator goal pause " + record.ID} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("activity missing %q:\n%s", expected, output.String())
		}
	}
	if strings.Contains(output.String(), "Unrelated private alert") {
		t.Fatalf("activity leaked another goal's inbox item:\n%s", output.String())
	}
	if err := goalCommand([]string{"pause", record.ID}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := goalCommand([]string{"activity", record.ID}, &output); err != nil || !strings.Contains(output.String(), "goal paused") || strings.Contains(output.String(), "next:") {
		t.Fatalf("paused activity = %q, %v", output.String(), err)
	}
}
