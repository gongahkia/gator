package main

import (
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/goal"
)

func TestTUIPlanUpdatesCurrentGoal(t *testing.T) {
	store, err := goal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Add("Plan offsite", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		"outcome Confirm the venue",
		"add Check capacity | 2026-10-20T09:00:00+08:00 | venue.md:12",
		"review-at 2026-10-21T09:00:00+08:00",
	} {
		if _, err := workTUIPlanAction(store, record.ID, command); err != nil {
			t.Fatalf("%q: %v", command, err)
		}
	}
	plan, err := store.LoadPlan(record.ID)
	if err != nil || len(plan.Steps) != 1 || plan.Outcome != "Confirm the venue" {
		t.Fatalf("plan = %#v, %v", plan, err)
	}
	if _, err := workTUIPlanAction(store, record.ID, "step "+plan.Steps[0].ID+" done | user-confirmed: room holds 90"); err != nil {
		t.Fatal(err)
	}
	shown, err := workTUIPlanAction(store, record.ID, "")
	if err != nil || !strings.Contains(shown, "Confirm the venue") || !strings.Contains(shown, "user-confirmed: room holds 90") {
		t.Fatalf("shown = %q, %v", shown, err)
	}
	if _, err := workTUIPlanAction(store, record.ID, "remove "+plan.Steps[0].ID); err == nil {
		t.Fatal("step removal did not require --yes")
	}
}
