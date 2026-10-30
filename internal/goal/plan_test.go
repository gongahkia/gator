package goal

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/privatefs"
)

func TestGoalPlanPersistsEvidenceAndRejectsStaleChanges(t *testing.T) {
	state := t.TempDir()
	store, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	goalRecord, err := store.Add("Plan the offsite", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.LoadPlan(goalRecord.ID)
	if err != nil || plan.Revision != 0 || len(plan.Steps) != 0 {
		t.Fatalf("initial plan = %#v, %v", plan, err)
	}
	plan, err = store.ApplyPlanChange(goalRecord.ID, PlanChange{Kind: "set_outcome", ExpectedRevision: 0, Outcome: "Choose a venue and confirm the budget"})
	if err != nil || plan.Revision != 1 {
		t.Fatalf("outcome change = %#v, %v", plan, err)
	}
	due := time.Date(2026, 10, 20, 9, 0, 0, 0, time.FixedZone("SGT", 8*3600))
	plan, err = store.ApplyPlanChange(goalRecord.ID, PlanChange{Kind: "add_step", ExpectedRevision: 1, Title: "Confirm room capacity", DueAt: due, Evidence: []string{"source/venue-email.md:12"}})
	if err != nil || len(plan.Steps) != 1 || plan.Steps[0].ID == "" || !plan.Steps[0].DueAt.Equal(due) {
		t.Fatalf("step change = %#v, %v", plan, err)
	}
	if _, err := store.ApplyPlanChange(goalRecord.ID, PlanChange{Kind: "set_outcome", ExpectedRevision: 1, Outcome: "stale"}); err == nil || !strings.Contains(err.Error(), "expected revision") {
		t.Fatalf("stale write accepted: %v", err)
	}
	plan, err = store.ApplyPlanChange(goalRecord.ID, PlanChange{Kind: "update_step", ExpectedRevision: 2, StepID: plan.Steps[0].ID, Status: StepDone, Evidence: []string{"user-confirmed: room holds 90"}})
	if err != nil || plan.Steps[0].Status != StepDone || plan.Steps[0].Evidence[0] != "user-confirmed: room holds 90" {
		t.Fatalf("completed step = %#v, %v", plan, err)
	}
	plan, err = store.ApplyPlanChange(goalRecord.ID, PlanChange{Kind: "update_step", ExpectedRevision: 3, StepID: plan.Steps[0].ID, ClearDue: true, ClearEvidence: true})
	if err != nil || !plan.Steps[0].DueAt.IsZero() || len(plan.Steps[0].Evidence) != 0 {
		t.Fatalf("corrected step = %#v, %v", plan, err)
	}
	plan, err = store.ApplyPlanChange(goalRecord.ID, PlanChange{Kind: "set_next_review", ExpectedRevision: 4, NextReviewAt: due})
	if err != nil || !plan.NextReviewAt.Equal(due) {
		t.Fatalf("review time = %#v, %v", plan, err)
	}
	plan, err = store.ApplyPlanChange(goalRecord.ID, PlanChange{Kind: "clear_next_review", ExpectedRevision: 5})
	if err != nil || !plan.NextReviewAt.IsZero() {
		t.Fatalf("cleared review time = %#v, %v", plan, err)
	}
	reopened, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.LoadPlan(goalRecord.ID)
	if err != nil || loaded.Revision != 6 || loaded.Outcome != "Choose a venue and confirm the budget" || loaded.Steps[0].Status != StepDone {
		t.Fatalf("reopened plan = %#v, %v", loaded, err)
	}
	for _, path := range []string{reopened.root, filepath.Join(reopened.root, "plans"), reopened.path(goalRecord.ID), reopened.planPath(goalRecord.ID)} {
		if err := privatefs.Check(path); err != nil {
			t.Fatalf("private goal path %s: %v", path, err)
		}
	}
}

func TestGoalPlanSerializesConcurrentChangesAndRejectsSymlinks(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Add("Track launch", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, title := range []string{"Check date", "Check budget"} {
		wg.Add(1)
		go func(title string) {
			defer wg.Done()
			_, err := store.ApplyPlanChange(record.ID, PlanChange{Kind: "add_step", ExpectedRevision: 0, Title: title})
			results <- err
		}(title)
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent successes = %d, want one", successes)
	}
	plan, err := store.LoadPlan(record.ID)
	if err != nil || plan.Revision != 1 || len(plan.Steps) != 1 {
		t.Fatalf("concurrent plan = %#v, %v", plan, err)
	}
	if err := os.Remove(store.planPath(record.ID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "outside.json"), store.planPath(record.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadPlan(record.ID); err == nil {
		t.Fatal("symlinked plan accepted")
	}
}
