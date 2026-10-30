package goal

import (
	"strings"
	"testing"
	"time"
)

func TestCrossProjectDependenciesRejectCyclesAndReferencedRemoval(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Add("Launch", "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Add("Legal", "", "")
	if err != nil {
		t.Fatal(err)
	}
	pa, err := s.InitializePlan(a.ID, "Launch", []string{"Publish"})
	if err != nil {
		t.Fatal(err)
	}
	pb, err := s.InitializePlan(b.ID, "Approval", []string{"Approve"})
	if err != nil {
		t.Fatal(err)
	}
	ref := b.ID + "/" + pb.Steps[0].ID
	pa, err = s.ApplyPlanChange(a.ID, PlanChange{Kind: "update_step", ExpectedRevision: pa.Revision, StepID: pa.Steps[0].ID, DependsOn: []string{ref}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyPlanChange(b.ID, PlanChange{Kind: "update_step", ExpectedRevision: pb.Revision, StepID: pb.Steps[0].ID, DependsOn: []string{a.ID + "/" + pa.Steps[0].ID}}); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cross-goal cycle accepted: %v", err)
	}
	if _, err = s.ApplyPlanChange(b.ID, PlanChange{Kind: "remove_step", ExpectedRevision: pb.Revision, StepID: pb.Steps[0].ID}); err == nil {
		t.Fatal("removed referenced step")
	}
	now := time.Now()
	agenda, err := s.Agenda(a.ID, now, now.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(agenda.Items) != 1 || len(agenda.Items[0].BlockedBy) != 1 {
		t.Fatalf("missing blocker: %#v", agenda)
	}
	if _, err = s.ApplyPlanChange(b.ID, PlanChange{Kind: "update_step", ExpectedRevision: pb.Revision, StepID: pb.Steps[0].ID, Status: StepDone}); err != nil {
		t.Fatal(err)
	}
	states, err := s.DependencyStates(pa)
	if err != nil || states[ref] != StepDone {
		t.Fatalf("dependency state: %#v %v", states, err)
	}
	agenda, err = s.Agenda(a.ID, now, now.Add(24*time.Hour))
	if err != nil || len(agenda.Items[0].BlockedBy) != 0 {
		t.Fatalf("unresolved completed dependency: %#v %v", agenda, err)
	}
}
