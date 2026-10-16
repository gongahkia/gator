package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
)

// runDueGoalReviews sends one local reminder for each user-selected plan
// review time. It never calls a model or changes the plan on its own.
func runDueGoalReviews(stateDir string, notify bool, now time.Time) {
	goals, err := goal.Open(stateDir)
	if err != nil {
		return
	}
	mailbox, err := inbox.Open(stateDir)
	if err != nil {
		return
	}
	records, err := goals.List()
	if err != nil {
		return
	}
	ownedByWatch := map[string]bool{}
	if jobStore, err := jobs.Open(stateDir); err == nil {
		if definitions, err := jobStore.List(); err == nil {
			for _, definition := range definitions {
				if definition.GoalWatch && definition.Enabled {
					ownedByWatch[definition.GoalID] = true
				}
			}
		}
	}
	for _, record := range records {
		if record.Status != goal.Active || ownedByWatch[record.ID] {
			continue
		}
		plan, err := goals.LoadPlan(record.ID)
		if err != nil || plan.NextReviewAt.IsZero() || plan.NextReviewAt.After(now) || !plan.LastReviewAt.Before(plan.NextReviewAt) {
			continue
		}
		id := fmt.Sprintf("goal-review-%s-%d", record.ID, plan.NextReviewAt.UTC().UnixNano())
		exists, err := mailbox.Has(id)
		if err != nil || exists {
			continue
		}
		summary := "Review this goal and choose its next step."
		if strings.TrimSpace(plan.Outcome) != "" {
			summary = "Review outcome: " + plan.Outcome
		}
		for _, step := range plan.Steps {
			if step.Status != goal.StepDone {
				summary += " Next open step: " + step.Title
				break
			}
		}
		if len(summary) > 1000 {
			summary = string([]rune(summary)[:min(len([]rune(summary)), 500)])
		}
		entry, err := mailbox.Add(inbox.Entry{ID: id, Kind: "goal_review", Title: record.Title, Summary: summary, GoalID: record.ID, ConversationID: record.ConversationID, Status: "review_due"})
		if err == nil && notify {
			desktopNotification(entry.Title, entry.Summary)
		}
	}
}
