package worksession

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func accessRereviewFixture(t *testing.T) (Store, string, Request) {
	t.Helper()
	s, id, r := planFixture(t)
	if _, err := s.ReviseTaskPlan(id, r.ID, 0, "", proposedPlan()); err != nil {
		t.Fatal(err)
	}
	j, err := s.LoadRequests(id)
	if err != nil {
		t.Fatal(err)
	}
	r = j.Requests[0]
	need := TaskAccessNeed{ConnectorID: "docs", Kind: "http_json", Operation: "fetch"}
	access := &PlanAccessChange{ConnectorIDs: []string{"docs"}, BeforeScope: strings.Repeat("f", 64), AfterScope: strings.Repeat("a", 64), ProcessingDigest: strings.Repeat("c", 64), ReviewID: strings.Repeat("d", 64), Requirements: []TaskAccessSelection{{Need: need, ConnectorID: "docs"}}}
	r.Recovery = &RequestRecovery{Kind: "source_access", Needs: []TaskAccessNeed{need}, ScopeDigest: access.AfterScope, ProcessingDigest: access.ProcessingDigest, RetrySafe: true, Summary: "Repair requested source access", Repair: "Review the selected account"}
	r.Plan = r.Plan.Clone()
	attempt := budgetAttempt()
	attempt.Execution.MaxSteps, attempt.Execution.CodeMaxSteps = r.Plan.Budget.MaxSteps, r.Plan.Budget.CodeMaxSteps
	r.Plan.Attempts = []PlanAttempt{attempt}
	r.WorkID, r.RevisionID = attempt.RunID, attempt.RevisionID
	r.Plan.InputChanges = []PlanInputChange{{ParentID: id, TurnID: strings.Repeat("e", 64), BeforeOptions: append(json.RawMessage(nil), r.Options...), AfterSHA256: optionsHash(r.Options), Access: access}}
	r.Plan.Digest = r.Plan.contentDigest()
	j.Requests[0] = r
	if _, err := s.SaveRequests(id, j); err != nil {
		t.Fatal(err)
	}
	return s, id, r
}
func TestPendingAccessRereviewRevokesApprovalRetainsProposalAndDoesNotReaccountAttempt(t *testing.T) {
	for _, approved := range []bool{false, true} {
		t.Run(map[bool]string{false: "review", true: "approved"}[approved], func(t *testing.T) {
			s, id, r := accessRereviewFixture(t)
			if approved {
				if _, _, err := s.ApproveTaskPlan(id, r.ID, r.Plan.Revision, r.Plan.Digest); err != nil {
					t.Fatal(err)
				}
				r = retainedPlan(t, s, id, r.ID)
			}
			before := r.Plan.Clone()
			budget, limits, err := before.RemainingExecutionBudget()
			if err != nil {
				t.Fatal(err)
			}
			turn := editMessage(t, s, id, r.ID, "access-rereview-0001")
			nextAccess := &PlanAccessChange{ConnectorIDs: []string{"reports"}, BeforeScope: r.Recovery.ScopeDigest, AfterScope: strings.Repeat("b", 64), ProcessingDigest: r.Recovery.ProcessingDigest, ReviewID: strings.Repeat("9", 64), Requirements: []TaskAccessSelection{{Need: r.Recovery.Needs[0], ConnectorID: "reports"}}}
			_, pending, err := s.PrepareFailedRequestAccessReview(id, r.ID, id, turn.ID, turn.Text, nextAccess)
			if err != nil {
				t.Fatal(err)
			}
			if pending.ID != r.ID || pending.Plan.Phase != "revising" || pending.Plan.Approval != nil || pending.Plan.Content != nil || pending.Plan.DrainingRun != "" {
				t.Fatal("replacement retained executable authority", pending.Plan)
			}
			if !reflect.DeepEqual(pending.Plan.Attempts, before.Attempts) {
				t.Fatal("an accounted attempt was charged again")
			}
			afterBudget, afterLimits, err := pending.Plan.RemainingExecutionBudget()
			if err != nil || afterBudget != budget || afterLimits != limits {
				t.Fatal("remaining budget changed without execution", err)
			}
			prior := pending.Plan.LatestAccessChange().PriorPlan
			if prior == nil || prior.Revision != before.Revision || prior.Digest != before.Digest || !reflect.DeepEqual(prior.Content, before.Content) || !reflect.DeepEqual(prior.Approval, before.Approval) {
				t.Fatal("original proposal/review was lost", prior)
			}
			content := proposedPlan()
			content.Sources = []string{"reports"}
			if _, err := s.FinishTaskPlanEdit(id, r.ID, pending.Plan.Revision, pending.Plan.Digest, turn.ID, &content, ""); err != nil {
				t.Fatal(err)
			}
			if _, _, err := s.ApproveTaskPlan(id, r.ID, before.Revision, before.Digest); err == nil {
				t.Fatal("old review approved the replacement account scope")
			}
		})
	}
}
func TestPendingAccessEligibilityRejectsCancelledLiveAndUnaccountedWork(t *testing.T) {
	_, _, r := accessRereviewFixture(t)
	if !r.CanReviewPendingAccess() {
		t.Fatal("fixture has no pending recovery")
	}
	for _, state := range []string{"running", "stopped", "discarded", "completed", "failed"} {
		copy := r
		copy.State = state
		if copy.CanReviewPendingAccess() {
			t.Fatal("unexpected scope rereview for state", state)
		}
	}
	for _, phase := range []string{"preparing", "revising", "dispatching", "finished"} {
		copy := r
		copy.Plan = r.Plan.Clone()
		copy.Plan.Phase = phase
		if copy.CanReviewPendingAccess() {
			t.Fatal("unexpected scope rereview for phase", phase)
		}
	}
	copy := r
	copy.Plan = r.Plan.Clone()
	copy.Plan.Attempts = nil
	if copy.CanReviewPendingAccess() {
		t.Fatal("missing accounting permitted scope retry")
	}
	copy = r
	copy.Plan = r.Plan.Clone()
	copy.Plan.DrainingRun = "live-original"
	if copy.CanReviewPendingAccess() {
		t.Fatal("undrained owner permitted scope retry")
	}
}
func TestPendingAccessHistoryRejectsUnselectedOrDuplicateRequirementMappings(t *testing.T) {
	_, _, r := accessRereviewFixture(t)
	a := r.Plan.LatestAccessChange()
	if id, ok := a.SelectionFor(r.Recovery.Needs[0]); !ok || id != "docs" {
		t.Fatal("lost requirement identity")
	}
	a.Requirements = append(a.Requirements, a.Requirements[0])
	if a.validate() == nil {
		t.Fatal("duplicate requirement mapping accepted")
	}
	a.Requirements = a.Requirements[:1]
	a.Requirements[0].ConnectorID = "unselected"
	if a.validate() == nil {
		t.Fatal("mapping acquired an unselected account")
	}
	a.Requirements = nil
	a.PriorPlan = &PlanReviewSnapshot{Revision: 1, Digest: strings.Repeat("a", 64), Content: &PlanContent{Outcome: "old", Steps: []PlanStep{{ID: "read", Text: "Read"}}, Sources: []string{"unselected"}, Effects: []string{"read"}}, Approval: &PlanApproval{Revision: 1, Digest: strings.Repeat("a", 64), At: time.Now()}}
	if a.validatePriorPlan(r.Options) == nil {
		t.Fatal("historical proposal did not bind original sources")
	}
}
