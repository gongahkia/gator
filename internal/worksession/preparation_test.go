package worksession

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
)

func preparationFixture(t *testing.T, planned bool) (Store, string, Request, PlanBinding) {
	t.Helper()
	s, id, _ := requestTestStore(t)
	r := retainedRequest(t, "Read the original source")
	if planned {
		var err error
		_, r, err = s.AdmitPlanRequest(id, r.Text, json.RawMessage(`{"PlanFirst":true,"Mode":"inspect","MaxSteps":24,"Code":{"MaxSteps":16},"ConnectorIDs":["docs"]}`), "original-client", []string{"docs"}, PlanBudget{MaxSteps: 24, CodeMaxSteps: 16}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.ReviseTaskPlan(id, r.ID, 0, "", proposedPlan()); err != nil {
			t.Fatal(err)
		}
		r = retainedPlan(t, s, id, r.ID)
		if _, _, err := s.ApproveTaskPlan(id, r.ID, r.Plan.Revision, r.Plan.Digest); err != nil {
			t.Fatal(err)
		}
		r = retainedPlan(t, s, id, r.ID)
		r.Plan.Phase = "dispatching"
	} else {
		b, err := s.ExecutionBoundary(id, r)
		if err != nil {
			t.Fatal(err)
		}
		r.ExecutionBoundary = &b
	}
	r.State = "running"
	j, err := s.LoadRequests(id)
	if err != nil {
		t.Fatal(err)
	}
	j.Requests = []Request{r}
	if _, err := s.SaveRequests(id, j); err != nil {
		t.Fatal(err)
	}
	c, _ := s.Load(id)
	b, err := s.PreparationBoundary(id, r.ID, r.Text, c.SourcePath, r.Options)
	if err != nil {
		t.Fatal(err)
	}
	return s, id, r, b
}

func TestPreparationBoundaryRequiresTheOriginalRunningOwnerAndInputs(t *testing.T) {
	for _, planned := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "planned"}[planned], func(t *testing.T) {
			s, id, r, b := preparationFixture(t, planned)
			c, _ := s.Load(id)
			if b.RequestID != r.ID || b.ConversationID != id || b.OptionsSHA256 != optionsHash(r.Options) {
				t.Fatal("wrong preparation identity")
			}
			for _, bad := range []struct {
				owner, request, text, source string
				options                      json.RawMessage
			}{
				{id, "other", r.Text, c.SourcePath, r.Options}, {id, r.ID, "Changed task", c.SourcePath, r.Options},
				{id, r.ID, r.Text, t.TempDir(), r.Options}, {id, r.ID, r.Text, c.SourcePath, json.RawMessage(`{"MaxSteps":99}`)},
			} {
				if _, err := s.PreparationBoundary(bad.owner, bad.request, bad.text, bad.source, bad.options); err == nil {
					t.Fatal("mismatched owner/input attested", bad)
				}
			}
			j, _ := s.LoadRequests(id)
			j.Requests[0].State = "stopped"
			if _, err := s.SaveRequests(id, j); err != nil {
				t.Fatal(err)
			}
			if _, err := s.PreparationBoundary(id, r.ID, r.Text, c.SourcePath, r.Options); err == nil {
				t.Fatal("stopped task attested")
			}
		})
	}
}

func savePreparationFailure(t *testing.T, s Store, id string, r Request, b PlanBinding) Request {
	t.Helper()
	r.State = "failed"
	if r.Plan != nil {
		r.Plan.Phase = "finished"
	}
	r.Recovery = &RequestRecovery{Preparation: &b, Kind: "source_access", RetrySafe: true, Needs: []TaskAccessNeed{{ConnectorID: "docs", Kind: "unconfigured", Operation: "configure"}}, ScopeDigest: strings.Repeat("a", 64), ProcessingDigest: strings.Repeat("b", 64), Summary: "Source missing before execution", Repair: "Review repaired access"}
	j, _ := s.LoadRequests(id)
	j.Requests[0] = r
	if _, err := s.SaveRequests(id, j); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestUnstartedPreparationRecoveryKeepsPriorBudgetAndRequiresNewReview(t *testing.T) {
	for _, planned := range []bool{false, true} {
		s, id, r, b := preparationFixture(t, planned)
		if planned {
			a := budgetAttempt()
			a.Execution.MaxSteps, a.Execution.CodeMaxSteps = r.Plan.Budget.MaxSteps, r.Plan.Budget.CodeMaxSteps
			r.Plan.Attempts = []PlanAttempt{a}
			r.Plan.Digest = r.Plan.contentDigest()
			r.Plan.Approval.Digest = r.Plan.Digest
		}
		r = savePreparationFailure(t, s, id, r, b)
		var before []PlanAttempt
		var beforeBudget PlanBudget
		var beforeLimits agent.Limits
		if r.Plan != nil {
			before = append(before, r.Plan.Attempts...)
			var err error
			beforeBudget, beforeLimits, err = r.Plan.RemainingExecutionBudget()
			if err != nil {
				t.Fatal(err)
			}
		}
		turn := editMessage(t, s, id, r.ID, "preparation-review-0001")
		access := &PlanAccessChange{ConnectorIDs: []string{"docs"}, BeforeScope: r.Recovery.ScopeDigest, AfterScope: strings.Repeat("c", 64), ProcessingDigest: r.Recovery.ProcessingDigest, ReviewID: strings.Repeat("d", 64)}
		_, next, err := s.PrepareFailedRequestAccessReview(id, r.ID, id, turn.ID, turn.Text, access)
		if err != nil {
			t.Fatal(err)
		}
		if next.ID != r.ID || next.Text != r.Text || next.Plan.Approval != nil || next.Plan.DrainingRun != "" || next.Plan.Phase != "revising" || !reflect.DeepEqual(before, next.Plan.Attempts) {
			t.Fatal("lost identity, budget or review boundary", next.Plan)
		}
		if s.CheckPlanDispatch(id, next) == nil {
			t.Fatal("setup review authorized execution")
		}
		if planned {
			afterBudget, afterLimits, err := next.Plan.RemainingExecutionBudget()
			if err != nil || afterBudget != beforeBudget || afterLimits != beforeLimits {
				t.Fatal("preparation failure reset spent budget", err)
			}
		}
		content := proposedPlan()
		if _, err := s.FinishTaskPlanEdit(id, r.ID, next.Plan.Revision, next.Plan.Digest, turn.ID, &content, ""); err != nil {
			t.Fatal(err)
		}
		retained := retainedPlan(t, s, id, r.ID)
		if !retained.CanReviewPendingAccess() {
			t.Fatal("unstarted recovery lost its pending setup controls")
		}
	}
}

func TestPreparationFailureValidationAndStaleContextNeverGrantRetry(t *testing.T) {
	for _, change := range []string{"no-attestation", "need-without-attestation", "request", "options", "work", "revision", "context", "stopped"} {
		t.Run(change, func(t *testing.T) {
			s, id, r, b := preparationFixture(t, false)
			r = savePreparationFailure(t, s, id, r, b)
			tampered := r
			recovery := *r.Recovery
			boundary := *recovery.Preparation
			recovery.Preparation = &boundary
			tampered.Recovery = &recovery
			switch change {
			case "request":
				boundary.RequestID = "another-request"
			case "options":
				boundary.OptionsSHA256 = strings.Repeat("9", 64)
			case "work":
				tampered.WorkID = "an-actual-run"
			case "revision":
				tampered.RevisionID = "an-actual-revision"
			case "no-attestation":
				recovery.Preparation = nil
				recovery.Needs = nil
			case "need-without-attestation":
				recovery.Preparation = nil
			case "stopped":
				tampered.State = "stopped"
			case "context":
				c, _ := s.Load(id)
				c.UpdatedAt = c.UpdatedAt.Add(time.Second)
				if err := writeJSON(filepath.Join(s.root, id, "conversation.json"), c); err != nil {
					t.Fatal(err)
				}
			}
			if change == "request" || change == "options" || change == "work" || change == "revision" || change == "need-without-attestation" {
				if validateRequest(tampered) == nil {
					t.Fatal("invalid native attestation accepted")
				}
				return
			}
			j, _ := s.LoadRequests(id)
			j.Requests[0] = tampered
			if _, err := s.SaveRequests(id, j); err != nil {
				t.Fatal(err)
			}
			turn := editMessage(t, s, id, r.ID, "preparation-stale-0001")
			path := filepath.Join(s.root, id, "requests.json")
			before, _ := os.ReadFile(path)
			if _, _, err := s.PrepareFailedRequestReview(id, r.ID, id, turn.ID, turn.Text); err == nil {
				t.Fatal("uncertain/stale/stopped preparation retried")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("rejected retry changed original request")
			}
		})
	}
}
