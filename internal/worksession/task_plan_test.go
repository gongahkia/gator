package worksession

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func planFixture(t *testing.T) (Store, string, Request) {
	t.Helper()
	s, id, _ := requestTestStore(t)
	_, r, err := s.AdmitPlanRequest(id, "Implement the parser", json.RawMessage(`{"PlanFirst":true,"MaxSteps":24,"Code":{"MaxSteps":16},"ConnectorIDs":["docs"]}`), "plan-client-1", []string{"docs"}, PlanBudget{MaxSteps: 24, CodeMaxSteps: 16}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return s, id, r
}
func proposedPlan() PlanContent {
	return PlanContent{Outcome: "A parser with checked tests", Steps: []PlanStep{{ID: "read", Text: "Inspect the existing parser"}, {ID: "implement", Text: "Stage a reviewed patch"}, {ID: "check", Text: "Run the required verification"}}, Sources: []string{"docs"}, Effects: []string{"read", "code", "checks"}}
}
func retainedPlan(t *testing.T, s Store, id, request string) Request {
	t.Helper()
	j, err := s.LoadRequests(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range j.Requests {
		if r.ID == request {
			return r
		}
	}
	t.Fatal("missing plan")
	return Request{}
}
func TestTaskPlanReviewRevisionApprovalAndDispatchAreSeparate(t *testing.T) {
	s, id, r := planFixture(t)
	if err := s.CheckPlanDispatch(id, r); !errors.Is(err, ErrPlanChanged) {
		t.Fatal("preparation authorized execution", err)
	}
	content := proposedPlan()
	if _, err := s.ReviseTaskPlan(id, r.ID, 0, "", content); err != nil {
		t.Fatal(err)
	}
	content.Steps[0].Text = "changed caller buffer"
	r = retainedPlan(t, s, id, r.ID)
	if r.Plan.Revision != 1 || r.Plan.Content.Steps[0].Text == content.Steps[0].Text {
		t.Fatal("revision not retained independently")
	}
	if err := s.CheckPlanDispatch(id, r); err == nil {
		t.Fatal("unapproved review authorized work")
	}
	if _, _, err := s.ApproveTaskPlan("other", r.ID, 1, r.Plan.Digest); err == nil {
		t.Fatal("wrong conversation approved plan")
	}
	for _, bad := range []struct {
		revision uint64
		digest   string
	}{{0, r.Plan.Digest}, {1, strings.Repeat("0", 64)}} {
		if _, _, err := s.ApproveTaskPlan(id, r.ID, bad.revision, bad.digest); err == nil {
			t.Fatal("stale review approved")
		}
	}
	if _, changed, err := s.ApproveTaskPlan(id, r.ID, 1, r.Plan.Digest); err != nil || !changed {
		t.Fatal("valid review not admitted", err)
	}
	r = retainedPlan(t, s, id, r.ID)
	if err := s.CheckPlanDispatch(id, r); err != nil {
		t.Fatal("reviewed plan rejected", err)
	}
	if _, changed, err := s.ApproveTaskPlan(id, r.ID, 1, r.Plan.Digest); err != nil || changed {
		t.Fatal("duplicate review admitted again", err)
	}
	old := r.Plan.Digest
	next := proposedPlan()
	next.Steps[1].Text = "Implement one bounded improvement"
	if _, err := s.ReviseTaskPlan(id, r.ID, 1, old, next); err != nil {
		t.Fatal(err)
	}
	r = retainedPlan(t, s, id, r.ID)
	if r.Plan.Revision != 2 || r.Plan.Approval != nil || r.Plan.Digest == old || r.Plan.Phase != "review" {
		t.Fatal("edit preserved approval")
	}
	if _, _, err := s.ApproveTaskPlan(id, r.ID, 1, old); err == nil {
		t.Fatal("old approval accepted after edit")
	}
	if err := s.CheckPlanDispatch(id, r); err == nil {
		t.Fatal("edited plan dispatched")
	}
}
func TestTaskPlanBindingsRestartCancellationAndNoImplicitRetry(t *testing.T) {
	s, id, r := planFixture(t)
	_, duplicate, err := s.AdmitPlanRequest(id, r.Text, r.Options, "plan-client-1", []string{"docs"}, r.Plan.Budget, time.Now())
	if err != nil || duplicate.ID != r.ID {
		t.Fatal("duplicate task changed", err)
	}
	if _, _, err := s.AdmitPlanRequest(id, "different scope", r.Options, "plan-client-1", []string{"docs"}, r.Plan.Budget, time.Now()); err == nil {
		t.Fatal("conflicting client identity reused")
	}
	if _, err := s.ReviseTaskPlan(id, r.ID, 0, "", proposedPlan()); err != nil {
		t.Fatal(err)
	}
	r = retainedPlan(t, s, id, r.ID)
	if _, _, err := s.ApproveTaskPlan(id, r.ID, r.Plan.Revision, r.Plan.Digest); err != nil {
		t.Fatal(err)
	}
	reopened := Store{root: s.root}
	r = retainedPlan(t, reopened, id, r.ID)
	if err := reopened.CheckPlanDispatch(id, r); err != nil {
		t.Fatal("durable review lost", err)
	}
	if _, err := s.CancelTaskPlan(id, r.ID, r.Plan.Revision, r.Plan.Digest); err != nil {
		t.Fatal(err)
	}
	r = retainedPlan(t, s, id, r.ID)
	if r.State != "discarded" || r.Plan.Phase != "interrupted" || s.CheckPlanDispatch(id, r) == nil {
		t.Fatal("cancelled plan remained authorized")
	}
	if _, _, err := s.ApproveTaskPlan(id, r.ID, r.Plan.Revision, r.Plan.Digest); err == nil {
		t.Fatal("cancelled plan reactivated")
	}
}
func TestTaskPlanScopeAndPayloadFailClosed(t *testing.T) {
	s, id, r := planFixture(t)
	for _, edit := range []func(*PlanContent){func(p *PlanContent) { p.Sources = []string{"unselected"} }, func(p *PlanContent) { p.Effects = []string{"arbitrary_command"} }, func(p *PlanContent) { p.Steps[1].ID = p.Steps[0].ID }, func(p *PlanContent) { p.Outcome = strings.Repeat("x", 4097) }, func(p *PlanContent) { p.Steps = nil }} {
		p := proposedPlan()
		edit(&p)
		if _, err := s.ReviseTaskPlan(id, r.ID, 0, "", p); err == nil {
			t.Fatal("invalid plan accepted")
		}
	}
	if _, err := s.ReviseTaskPlan(id, r.ID, 0, "", proposedPlan()); err != nil {
		t.Fatal(err)
	}
	r = retainedPlan(t, s, id, r.ID)
	if _, err := s.AddRevision(id, Revision{ID: "new-head", SnapshotID: "snap-new", Objective: "Intervening work", FinalText: "Done", BundlePath: "/bundle/new", Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ApproveTaskPlan(id, r.ID, r.Plan.Revision, r.Plan.Digest); err == nil {
		t.Fatal("approval ignored new conversation context")
	}
	j, _ := s.LoadRequests(id)
	j.Requests[0].Options = json.RawMessage(`{"PlanFirst":false}`)
	if _, err := s.SaveRequests(id, j); err == nil {
		t.Fatal("frozen options could be altered without rebinding")
	}
	j, _ = s.LoadRequests(id)
	j.Requests[0].Plan.Content.Outcome = "tampered"
	if _, err := s.SaveRequests(id, j); err == nil {
		t.Fatal("tampered reviewed content retained")
	}
}

func TestTaskPlanConcurrentApprovalAdmitsOnceAndRetainsReceiptAfterCompletion(t *testing.T) {
	s, id, r := planFixture(t)
	j, err := s.ReviseTaskPlan(id, r.ID, 0, "", proposedPlan())
	if err != nil {
		t.Fatal(err)
	}
	r = j.Requests[0]
	var wg sync.WaitGroup
	changed := make(chan bool, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, fresh, err := s.ApproveTaskPlan(id, r.ID, r.Plan.Revision, r.Plan.Digest)
			if err != nil && !errors.Is(err, ErrRequestsChanged) {
				failures <- err
			}
			changed <- fresh
		}()
	}
	wg.Wait()
	close(changed)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	count := 0
	for fresh := range changed {
		if fresh {
			count++
		}
	}
	if count != 1 {
		t.Fatal("approval was admitted multiple times", count)
	}
	j, _ = s.LoadRequests(id)
	j.Requests[0].State = "completed"
	j.Requests[0].Plan.Phase = "finished"
	if _, err = s.SaveRequests(id, j); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddRevision(id, Revision{ID: "completed-new-head", SnapshotID: "snap-new", Objective: "Completed reviewed work", FinalText: "Done", BundlePath: "/bundle/completed", Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	if _, fresh, err := s.ApproveTaskPlan(id, r.ID, r.Plan.Revision, r.Plan.Digest); err != nil || fresh {
		t.Fatal("reconciliation reauthorized completed work", err)
	}
}

func TestTaskPlanHeadReturningToOldRevisionDoesNotRestoreApprovalAuthority(t *testing.T) {
	s, id, _ := requestTestStore(t)
	for _, name := range []string{"one", "two"} {
		parent := ""
		if name == "two" {
			parent = "rev-one"
		}
		if _, err := s.AddRevision(id, Revision{ID: "rev-" + name, ParentRevisionID: parent, SnapshotID: "snap-" + name, Objective: "Task " + name, FinalText: "Done", BundlePath: "/bundle/" + name, Status: "completed"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.MoveHead(id, "rev-one"); err != nil {
		t.Fatal(err)
	}
	_, r, err := s.AdmitPlanRequest(id, "Plan against revision one", json.RawMessage(`{"PlanFirst":true,"ConnectorIDs":["docs"]}`), "plan-head-generation", []string{"docs"}, PlanBudget{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.ReviseTaskPlan(id, r.ID, 0, "", proposedPlan())
	if err != nil {
		t.Fatal(err)
	}
	r = j.Requests[0]
	if _, err := s.MoveHead(id, "rev-two"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveHead(id, "rev-one"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ApproveTaskPlan(id, r.ID, r.Plan.Revision, r.Plan.Digest); err == nil {
		t.Fatal("returning to an old head revived stale authority")
	}
}
