package workhome

import (
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/jobs"
)

func TestPauseResumeRetainsSuccessfulCheckUntilSettingsChange(t *testing.T) {
	state := t.TempDir()
	store, err := jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	d, err := store.Save(jobs.Definition{Name: "Project", Enabled: true, Schedule: "* * * * *", SourcePath: t.TempDir(), RefreshSnapshot: true, Objective: "Check changes", Mode: action.Inspect, Contract: artifact.InspectionContract()})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.Begin(d, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// Use the old enabled-definition digest to verify historical compatibility.
	attempt.NotificationPolicySHA256, _ = d.PolicyDigest()
	attempt.Status, attempt.FinishedAt = "completed", time.Now().UTC()
	if err := store.Record(attempt); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true, false} {
		d, err = store.SetEnabled(d.ID, enabled)
		if err != nil {
			t.Fatal(err)
		}
		checks, err := ReadChecks(state, true, time.Now())
		if err != nil || len(checks) != 1 {
			t.Fatalf("read check: %+v %v", checks, err)
		}
		c := checks[0]
		if !c.LastSuccess.Equal(attempt.FinishedAt) || !enabled && (c.State != "paused" || c.DueNow || !c.NextDue.IsZero()) {
			t.Fatalf("pause lost evidence or advertised work: %+v", c)
		}
	}
	d.SourcePath = t.TempDir()
	if _, err := store.Save(d); err != nil {
		t.Fatal(err)
	}
	checks, err := ReadChecks(state, true, time.Now())
	if err != nil || len(checks) != 1 || !checks[0].LastSuccess.IsZero() {
		t.Fatalf("changed source reused success: %+v %v", checks, err)
	}
}
