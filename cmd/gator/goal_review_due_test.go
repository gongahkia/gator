package main

import (
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/inbox"
)

func TestDueGoalReviewCreatesOneReminderUntilMarkedReviewed(t *testing.T) {
	state := t.TempDir()
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Plan the offsite", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := goals.ApplyPlanChange(record.ID, goal.PlanChange{Kind: "set_outcome", ExpectedRevision: 0, Outcome: "Choose a venue"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err = goals.ApplyPlanChange(record.ID, goal.PlanChange{Kind: "add_step", ExpectedRevision: plan.Revision, Title: "Confirm capacity"})
	if err != nil {
		t.Fatal(err)
	}
	due := time.Now().UTC().Add(-time.Hour)
	plan, err = goals.ApplyPlanChange(record.ID, goal.PlanChange{Kind: "set_next_review", ExpectedRevision: plan.Revision, NextReviewAt: due})
	if err != nil {
		t.Fatal(err)
	}
	runDueGoalReviews(state, false, time.Now().UTC())
	runDueGoalReviews(state, false, time.Now().UTC())
	mailbox, err := inbox.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := mailbox.List(false, 10)
	if err != nil || len(entries) != 1 || entries[0].Kind != "goal_review" || entries[0].GoalID != record.ID {
		t.Fatalf("review reminders = %#v, %v", entries, err)
	}
	if _, err := goals.ApplyPlanChange(record.ID, goal.PlanChange{Kind: "mark_reviewed", ExpectedRevision: plan.Revision}); err != nil {
		t.Fatal(err)
	}
	updated, err := goals.LoadPlan(record.ID)
	if err != nil || !updated.NextReviewAt.IsZero() {
		t.Fatalf("review did not clear due time: %#v, %v", updated, err)
	}
	runDueGoalReviews(state, false, time.Now().UTC())
	entries, err = mailbox.List(false, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("reviewed goal reminded again: %#v, %v", entries, err)
	}
}
