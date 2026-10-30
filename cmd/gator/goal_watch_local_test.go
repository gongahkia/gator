package main

import (
	"context"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/workrun"
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

func TestGoalWatchLocalDigestTracksEvidenceNotWording(t *testing.T) {
	now := time.Now()
	plan := goal.Plan{GoalID: "goal-a", Steps: []goal.Step{{ID: "step-a", Status: goal.StepPending, DueAt: now}}}
	decision := jobs.WatchDecision{Status: "attention", Summary: "Deadline passed", Evidence: []string{"goal plan step step-a"}}
	first := localWatchDecisionDigest(decision, plan, now, nil)
	decision.Summary = "Please check this overdue task"
	if first == "" || localWatchDecisionDigest(decision, plan, now, nil) != first {
		t.Fatal("rewording changed concern identity")
	}
	decision.Urgent = true
	if localWatchDecisionDigest(decision, plan, now, nil) == first {
		t.Fatal("urgency escalation suppressed")
	}
	decision.Urgent = false
	plan.Steps[0].DueAt = now.Add(time.Hour)
	if localWatchDecisionDigest(decision, plan, now, nil) == first {
		t.Fatal("rescheduling suppressed")
	}
	decision.Evidence = []string{"source/notes.md:1"}
	if localWatchDecisionDigest(decision, plan, now, nil) != "" {
		t.Fatal("unfingerprinted evidence collapsed")
	}
}

func TestGoalWatchDoesNotPresentConcurrentEditsAsCurrent(t *testing.T) {
	for _, pause := range []bool{false, true} {
		state := t.TempDir()
		goals, err := goal.Open(state)
		if err != nil {
			t.Fatal(err)
		}
		record, err := goals.Add("Workshop", "", "")
		if err != nil {
			t.Fatal(err)
		}
		definition := jobs.Definition{ID: "job-watch", GoalWatch: true, GoalID: record.ID}
		result, err := runJobProcessWith(context.Background(), definition, state, func(context.Context, string, string, string, workrun.Request) (workrun.Outcome, error) {
			if pause {
				_, err = goals.SetStatus(record.ID, goal.Paused)
			} else {
				_, err = goals.ApplyPlanChange(record.ID, goal.PlanChange{Kind: "set_outcome", Outcome: "New owner instructions"})
			}
			if err != nil {
				t.Fatal(err)
			}
			return workrun.Outcome{Manifest: artifact.Manifest{Status: artifact.Completed}, Result: agent.Result{FinalText: `{"attention":false,"summary":"Nothing changed."}`}}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.WatchDecision == nil {
			t.Fatal("missing decision")
		}
		if pause && (result.WatchAttention || result.Urgent) {
			t.Fatalf("paused goal alerted: %#v", result)
		}
		if !pause && (result.WatchDecision.Status != "review" || !result.WatchAttention) {
			t.Fatalf("stale conclusion accepted: %#v", result)
		}
	}
}

func TestGoalWatchFeedbackIsScopedAndCorrectionsAreFresh(t *testing.T) {
	state := t.TempDir()
	mailbox, err := inbox.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	goalID := "goal-aaaaaaaaaaaaaaaaaaaaaaaa"
	for _, id := range []string{"job-watch", "job-other"} {
		_, err := mailbox.Add(inbox.Entry{ID: id, Kind: "job", Title: "Update", Summary: id + " concern", JobID: id, GoalID: goalID, Status: "completed"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := mailbox.Rate(id, "noise", time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	definition := jobs.Definition{ID: "job-watch", GoalID: goalID}
	text, err := goalWatchContinuity(state, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "job-watch concern") || strings.Contains(text, "job-other concern") {
		t.Fatalf("incorrect feedback scope: %s", text)
	}
	if _, err := mailbox.Rate("job-watch", "clear", time.Now()); err != nil {
		t.Fatal(err)
	}
	text, err = goalWatchContinuity(state, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "job-watch concern") {
		t.Fatalf("withdrawn feedback retained: %s", text)
	}
}
