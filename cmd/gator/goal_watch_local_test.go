package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/suggestion"
)

func TestGoalWatchLocalTracksBoundariesAndDependencies(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	plan := goal.Plan{Steps: []goal.Step{
		{ID: "approval", Status: goal.StepPending},
		{ID: "draft", Status: goal.StepPending, DueAt: now, DependsOn: []string{"approval"}},
	}}
	before := goalPlanWatchItems(plan, now.Add(-time.Second))
	after := goalPlanWatchItems(plan, now)
	history := []jobs.Attempt{{Status: "completed", NotificationPolicySHA256: "policy", WatchItems: before}}
	changes := diffGoalWatchItems(history, after, "policy")
	if len(changes) != 1 || changes[0].ID != "draft" || changes[0].Kind != "updated" {
		t.Fatalf("deadline transition: %#v", changes)
	}
	history[0].WatchItems = after
	if got := diffGoalWatchItems(history, goalPlanWatchItems(plan, now.Add(time.Hour)), "policy"); len(got) != 0 {
		t.Fatalf("unchanged overdue repeated: %#v", got)
	}
	plan.Steps[0].Status = goal.StepDone
	changes = diffGoalWatchItems(history, goalPlanWatchItems(plan, now), "policy")
	if len(changes) != 2 {
		t.Fatalf("dependency resolution: %#v", changes)
	}
	// An empty latest snapshot must not resurrect items from an older check.
	empty := jobs.Attempt{Status: "completed", NotificationPolicySHA256: "policy"}
	if got := diffGoalWatchItems(append([]jobs.Attempt{empty}, history...), nil, "policy"); len(got) != 0 {
		t.Fatalf("resurrected removed items: %#v", got)
	}
}

func TestGoalWatchContinuityScopesPendingProposals(t *testing.T) {
	state := t.TempDir()
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Workshop", "", "")
	if err != nil {
		t.Fatal(err)
	}
	proposals, err := suggestion.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{record.ID, "goal-aaaaaaaaaaaaaaaaaaaaaaaa"} {
		_, err = proposals.Create(suggestion.Record{Kind: "plan", GoalID: id, WorkID: "work-test", PlanChange: goal.PlanChange{Kind: "set_outcome", Outcome: id}})
		if err != nil {
			t.Fatal(err)
		}
	}
	definition := jobs.Definition{ID: "job-watch", GoalID: record.ID}
	history := []jobs.Attempt{{Status: "completed", NotificationPolicySHA256: jobNotificationPolicyHash(definition), WatchDecision: &jobs.WatchDecision{Status: "quiet", Summary: "Previously quiet"}}}
	context, err := goalWatchContinuity(state, definition, history)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(context, "Previously quiet") || !strings.Contains(context, "set_outcome") {
		t.Fatalf("missing continuity: %s", context)
	}
	if strings.Contains(context, "aaaaaaaaaaaaaaaaaaaaaaaa") {
		t.Fatalf("cross-goal proposal: %s", context)
	}
}
