package goal

import (
	"testing"
	"time"
)

func TestAgendaTracksDatesDependenciesConflictsAndReportedCompletion(t *testing.T) {
	s, _ := Open(t.TempDir())
	r, _ := s.Add("Workshop", "", "")
	p, _ := s.InitializePlan(r.ID, "Deliver workshop", []string{"Get approval", "Draft slides"})
	now := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	var err error
	p, err = s.ApplyPlanChange(r.ID, PlanChange{Kind: "update_step", ExpectedRevision: p.Revision, StepID: p.Steps[0].ID, DueAt: now.Add(-time.Hour), StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.ApplyPlanChange(r.ID, PlanChange{Kind: "update_step", ExpectedRevision: p.Revision, StepID: p.Steps[1].ID, StartsAt: now.Add(90 * time.Minute), EndsAt: now.Add(3 * time.Hour), DependsOn: []string{p.Steps[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	agenda, err := s.Agenda("", now, now.Add(24*time.Hour))
	if err != nil || len(agenda.Items) != 2 || !agenda.Items[0].Overdue || len(agenda.Items[1].BlockedBy) != 1 || len(agenda.Items[0].Conflicts) != 1 {
		t.Fatalf("agenda: %#v %v", agenda, err)
	}
	p, err = s.ApplyPlanChange(r.ID, PlanChange{Kind: "update_step", ExpectedRevision: p.Revision, StepID: p.Steps[0].ID, Status: StepDone})
	if err != nil {
		t.Fatal(err)
	}
	agenda, err = s.Agenda(r.ID, now.Add(4*time.Hour), now.Add(24*time.Hour))
	if err != nil || len(agenda.Items) != 1 || len(agenda.Items[0].BlockedBy) != 0 || agenda.Items[0].Step.Status != StepPending || len(agenda.Items[0].Conflicts) != 0 {
		t.Fatalf("completion/time inference: %#v %v", agenda, err)
	}
	// Stale corrections, cycles and removal of referenced steps must not mutate state.
	for _, change := range []PlanChange{
		{Kind: "update_step", ExpectedRevision: p.Revision - 1, StepID: p.Steps[1].ID, Status: StepDone},
		{Kind: "update_step", ExpectedRevision: p.Revision, StepID: p.Steps[0].ID, DependsOn: []string{p.Steps[1].ID}},
		{Kind: "remove_step", ExpectedRevision: p.Revision, StepID: p.Steps[0].ID},
		{Kind: "update_step", ExpectedRevision: p.Revision, StepID: p.Steps[1].ID, StartsAt: now, EndsAt: now.Add(-time.Hour)},
	} {
		if _, err := s.ApplyPlanChange(r.ID, change); err == nil {
			t.Fatalf("invalid change accepted: %#v", change)
		}
	}
	current, _ := s.LoadPlan(r.ID)
	if current.Revision != p.Revision {
		t.Fatal("failed edit changed revision")
	}
}

func TestReviewedPlanRetryDoesNotRepeatMutationOrLoseNewerEdit(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(root)
	r, _ := s.Add("Plan", "", "")
	change := PlanChange{Kind: "add_step", Title: "Draft", ExpectedRevision: 0}
	id := "suggestion-0123456789abcdef01234567"
	p, err := s.ApplyReviewedPlanChange(r.ID, change, id)
	if err != nil {
		t.Fatal(err)
	}
	s, _ = Open(root)
	again, err := s.ApplyReviewedPlanChange(r.ID, change, id)
	if err != nil || again.Revision != p.Revision || len(again.Steps) != 1 {
		t.Fatalf("retry: %#v %v", again, err)
	}
	if _, err := s.ApplyPlanChange(r.ID, PlanChange{Kind: "update_step", ExpectedRevision: p.Revision, StepID: p.Steps[0].ID, Status: StepDoing}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyReviewedPlanChange(r.ID, change, id); err == nil {
		t.Fatal("retry ignored newer owner edit")
	}
}
