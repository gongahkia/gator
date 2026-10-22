package workhome

import (
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
)

func TestCheckHealthSeparatesFailureOfflineAndLastSuccessfulPolicy(t *testing.T) {
	state := t.TempDir()
	goals, _ := goal.Open(state)
	record, err := goals.Add("Project", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, _ := jobs.Open(state)
	definition, err := store.Save(jobs.Definition{Name: "Project", GoalID: record.ID, SourcePath: record.SourcePath, Enabled: true, RefreshSnapshot: true, Schedule: "*/15 * * * *", Objective: "Check changes", Mode: action.Inspect, Contract: artifact.InspectionContract()})
	if err != nil {
		t.Fatal(err)
	}
	policy, _ := definition.PolicyDigest()
	now := time.Now().UTC()
	for i, status := range []string{"completed", "failed"} {
		attempt, err := store.Begin(definition, now.Add(time.Duration(i-2)*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		attempt.Status, attempt.NotificationPolicySHA256, attempt.FinishedAt = status, policy, time.Now().UTC()
		if status == "failed" {
			attempt.Error = "selected source access expired"
		}
		if err := store.Record(attempt); err != nil {
			t.Fatal(err)
		}
	}
	checks, err := ReadChecks(state, true, now)
	if err != nil {
		t.Fatal(err)
	}
	if checks[0].State != "couldn't check" || checks[0].LastSuccess.IsZero() || checks[0].NextDue.IsZero() {
		t.Fatalf("failed read looked quiet: %#v", checks[0])
	}
	checks, err = ReadChecks(state, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if checks[0].State != "offline" || checks[0].LastSuccess.IsZero() || !checks[0].NextDue.IsZero() {
		t.Fatalf("offline check claimed a next execution: %#v", checks[0])
	}
	definition.Objective = "Check a different outcome"
	if _, err := store.Save(definition); err != nil {
		t.Fatal(err)
	}
	checks, err = ReadChecks(state, true, now)
	if err != nil {
		t.Fatal(err)
	}
	if !checks[0].LastSuccess.IsZero() {
		t.Fatal("old policy's success was applied to new settings")
	}
	definition.EndsAt = now
	if _, err := store.Save(definition); err != nil {
		t.Fatal(err)
	}
	for _, online := range []bool{true, false} {
		checks, err = ReadChecks(state, online, now)
		if err != nil || checks[0].State != "ended" || !checks[0].NextDue.IsZero() {
			t.Fatalf("expiry health: %+v %v", checks, err)
		}
	}
}
