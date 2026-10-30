package goal

import (
	"testing"
	"time"
)

func TestRecurringBlocksKeepWallTimeAndSkipMissingDSTTime(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 3, 7, 2, 30, 0, 0, loc)
	step := Step{Status: StepPending, StartsAt: start, EndsAt: start.Add(time.Hour), Recurrence: &Recurrence{Frequency: "daily", Interval: 1, Timezone: loc.String()}}
	items, err := ExpandBlock(step, start, start.AddDate(0, 0, 3))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Date != "2026-03-07" || items[1].Date != "2026-03-09" || items[1].Step.StartsAt.In(loc).Hour() != 2 {
		t.Fatalf("DST occurrences: %#v", items)
	}
}

func TestRecurringOccurrenceUpdatesDoNotCompleteSeries(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := s.Add("Study", "", "")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	plan, err := s.ApplyPlanChange(record.ID, PlanChange{Kind: "add_step", Title: "Study", StartsAt: start, EndsAt: start.Add(time.Hour), Recurrence: &Recurrence{Frequency: "weekly", Interval: 1, Timezone: "UTC"}})
	if err != nil {
		t.Fatal(err)
	}
	id := plan.Steps[0].ID
	plan, err = s.ApplyPlanChange(record.ID, PlanChange{Kind: "update_occurrence", ExpectedRevision: plan.Revision, StepID: id, Occurrence: &OccurrenceOverride{Date: "2026-11-02", Status: "done"}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Steps[0].Status != StepPending {
		t.Fatal("occurrence completion ended series")
	}
	moved := start.AddDate(0, 0, 10)
	plan, err = s.ApplyPlanChange(record.ID, PlanChange{Kind: "update_occurrence", ExpectedRevision: plan.Revision, StepID: id, Occurrence: &OccurrenceOverride{Date: "2026-11-09", Status: "pending", StartsAt: moved, EndsAt: moved.Add(time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	agenda, err := s.Agenda(record.ID, start, start.AddDate(0, 0, 15))
	if err != nil {
		t.Fatal(err)
	}
	if len(agenda.Items) != 2 || agenda.Items[0].OccurrenceDate != "2026-11-09" || !agenda.Items[0].Step.StartsAt.Equal(moved) {
		t.Fatalf("occurrence agenda: %#v", agenda)
	}
	if _, err = s.ApplyPlanChange(record.ID, PlanChange{Kind: "update_occurrence", ExpectedRevision: plan.Revision, StepID: id, Occurrence: &OccurrenceOverride{Date: "2026-11-10", Status: "skipped"}}); err == nil {
		t.Fatal("non-series date accepted")
	}
	if _, err = s.ApplyPlanChange(record.ID, PlanChange{Kind: "update_step", ExpectedRevision: plan.Revision, StepID: id, ClearRecurrence: true}); err == nil {
		t.Fatal("silently discarded occurrence history")
	}
}
