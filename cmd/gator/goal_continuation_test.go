package main

import (
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
)

func TestGoalContinuationWakeAndStall(t *testing.T) {
	now := time.Now().UTC()
	plan := goal.Plan{Steps: []goal.Step{{ID: "step-a", Status: goal.StepPending}}}
	proposed := goalWatchDecision{Continuation: "ready", NextStepID: "step-a", NextStep: "Draft the outline"}
	d := checkedGoalContinuation(jobs.WatchDecision{Status: "quiet"}, proposed, plan, jobs.Definition{PollMinutes: 15}, now)
	if d.Continuation != "ready" || !d.NextCheckAt.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("continuation: %#v", d)
	}
	prior := jobs.Attempt{ID: "prior", Status: "completed", InputSHA256: "input", NotificationPolicySHA256: "policy", WatchDecision: &d}
	quiet := jobs.Attempt{ID: "quiet", Status: "unchanged"}
	if goalContinuationDue([]jobs.Attempt{quiet, prior}, jobs.Attempt{}, "policy", now) {
		t.Fatal("woke early")
	}
	if !goalContinuationDue([]jobs.Attempt{quiet, prior}, jobs.Attempt{}, "policy", d.NextCheckAt) {
		t.Fatal("lost wake across unchanged attempts")
	}
	if goalContinuationDue([]jobs.Attempt{prior}, jobs.Attempt{}, "changed-policy", d.NextCheckAt) {
		t.Fatal("obsolete policy resumed")
	}
	current := prior
	current.ID = "current"
	older := prior
	older.ID = "older"
	held := guardStalledContinuation([]jobs.Attempt{prior, quiet, older}, current)
	if !held.WatchAttention || held.WatchDecision.Continuation != "blocked" || !held.WatchDecision.NextCheckAt.IsZero() {
		t.Fatalf("stalled loop: %#v", held)
	}
	current.InputSHA256 = "new-input"
	if guardStalledContinuation([]jobs.Attempt{prior, older}, current).WatchDecision.Continuation != "ready" {
		t.Fatal("new evidence did not reset bound")
	}
}

func TestGoalContinuationRejectsInvalidAuthorityAndTiming(t *testing.T) {
	now := time.Now()
	plan := goal.Plan{Steps: []goal.Step{{ID: "a", Status: goal.StepPending, DependsOn: []string{"b"}}, {ID: "b", Status: goal.StepPending}}}
	for _, proposed := range []goalWatchDecision{
		{Continuation: "ready", NextStepID: "a", NextStep: "Start blocked work"},
		{Continuation: "ready", NextStepID: "missing", NextStep: "Invent work"},
		{Continuation: "waiting"},
		{Continuation: "waiting", NextCheckAt: now.Add(time.Minute)},
		{Continuation: "waiting", NextCheckAt: now.Add(8 * 24 * time.Hour)},
		{Continuation: "idle", NextCheckAt: now.Add(time.Hour)},
	} {
		d := checkedGoalContinuation(jobs.WatchDecision{Status: "quiet"}, proposed, plan, jobs.Definition{}, now)
		if d.Status != "review" || !d.NextCheckAt.IsZero() {
			t.Fatalf("invalid continuation accepted: %#v", d)
		}
	}
}
