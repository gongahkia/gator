package worksession

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func editMessage(t *testing.T, s Store, parent, target, client string) DialogueTurn {
	t.Helper()
	turn, err := s.AdmitDialogue(parent, client, "Keep the parser change small", json.RawMessage(`{"captured":"unchanged"}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	turn.State = "routing"
	turn, err = s.UpdateDialogue(turn, turn.Revision)
	if err != nil {
		t.Fatal(err)
	}
	turn.Decision, _ = json.Marshal(map[string]string{"kind": "revise_plan", "target": target, "instruction": turn.Text})
	turn.State = "ready"
	turn, err = s.UpdateDialogue(turn, turn.Revision)
	if err != nil {
		t.Fatal(err)
	}
	turn.State, turn.Response = "editing_plan", "Requesting a revised plan for review…"
	turn, err = s.UpdateDialogue(turn, turn.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return turn
}

func TestPlanEditsRevokeOldReviewsRetainScopeAndPublishActualResult(t *testing.T) {
	s, id, initial := planFixture(t)
	if _, err := s.ReviseTaskPlan(id, initial.ID, 0, "", proposedPlan()); err != nil {
		t.Fatal(err)
	}
	original := retainedPlan(t, s, id, initial.ID)
	if _, _, err := s.ApproveTaskPlan(id, original.ID, original.Plan.Revision, original.Plan.Digest); err != nil {
		t.Fatal(err)
	}
	turn := editMessage(t, s, id, original.ID, "edit-existing-plan-001")
	_, pending, claimed, err := s.BeginTaskPlanEdit(id, original.ID, original.Plan.Revision, original.Plan.Digest, id, turn.ID, turn.Text)
	if err != nil || !claimed || pending.Plan.Phase != "revising" || pending.Plan.Approval != nil {
		t.Fatal("revision was not admitted safely", err)
	}
	if pending.Plan.Revision <= original.Plan.Revision || pending.Plan.Digest == original.Plan.Digest || pending.Plan.Budget != original.Plan.Budget || !bytes.Equal(pending.Options, original.Options) || pending.Text != original.Text {
		t.Fatal("old review or frozen scope lost")
	}
	if _, _, err := s.ApproveTaskPlan(id, original.ID, original.Plan.Revision, original.Plan.Digest); err == nil {
		t.Fatal("old review approved during revision")
	}
	if _, _, err := s.ApproveTaskPlan(id, pending.ID, pending.Plan.Revision, pending.Plan.Digest); err == nil {
		t.Fatal("incomplete proposal approved")
	}
	if _, _, duplicate, err := s.BeginTaskPlanEdit(id, original.ID, original.Plan.Revision, original.Plan.Digest, id, turn.ID, turn.Text); err != nil || duplicate {
		t.Fatal("retry started a second proposal", err)
	}
	changed := proposedPlan()
	changed.Outcome = "A smaller reviewed parser"
	if _, err := s.FinishTaskPlanEdit(id, pending.ID, pending.Plan.Revision, pending.Plan.Digest, "different-turn", &changed, ""); err == nil {
		t.Fatal("mismatched completion replaced plan")
	}
	if _, err := s.FinishTaskPlanEdit(id, pending.ID, pending.Plan.Revision, pending.Plan.Digest, turn.ID, &changed, ""); err != nil {
		t.Fatal(err)
	}
	revised := retainedPlan(t, s, id, pending.ID)
	if revised.ID != original.ID || revised.Plan.Content.Outcome != changed.Outcome || revised.Plan.Phase != "review" || revised.Plan.Approval != nil {
		t.Fatal("revision executed or changed task identity")
	}
	result, err := s.PublishPlanEditResult(id, revised.ID)
	if err != nil || result.State != "guided" || !strings.Contains(result.Response, "no implementation") || result.ID != turn.ID || !bytes.Equal(result.Options, turn.Options) {
		t.Fatal("actual result was not retained in the requesting conversation", err)
	}
	if _, err := s.FinishTaskPlanEdit(id, pending.ID, pending.Plan.Revision, pending.Plan.Digest, turn.ID, &changed, ""); err == nil {
		t.Fatal("old completion replayed")
	}
	if _, _, err := s.ApproveTaskPlan(id, original.ID, original.Plan.Revision, original.Plan.Digest); err == nil {
		t.Fatal("old approval survived content change")
	}
}

func TestPlanEditFailureCancellationAndStaleProposalsPreserveReviewBoundaries(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		s, id, initial := planFixture(t)
		if _, err := s.ReviseTaskPlan(id, initial.ID, 0, "", proposedPlan()); err != nil {
			t.Fatal(err)
		}
		original := retainedPlan(t, s, id, initial.ID)
		turn := editMessage(t, s, id, original.ID, "failed-edit-plan-001")
		_, pending, _, err := s.BeginTaskPlanEdit(id, original.ID, original.Plan.Revision, original.Plan.Digest, id, turn.ID, turn.Text)
		if err != nil {
			t.Fatal(err)
		}
		if cancel {
			_, err = s.CancelTaskPlan(id, pending.ID, pending.Plan.Revision, pending.Plan.Digest)
			if err != nil {
				t.Fatal(err)
			}
			content := proposedPlan()
			if _, err := s.FinishTaskPlanEdit(id, pending.ID, pending.Plan.Revision, pending.Plan.Digest, turn.ID, &content, ""); err == nil {
				t.Fatal("cancelled proposal revived a plan")
			}
		} else {
			_, err = s.FinishTaskPlanEdit(id, pending.ID, pending.Plan.Revision, pending.Plan.Digest, turn.ID, nil, "Provider disconnected.")
			if err != nil {
				t.Fatal(err)
			}
		}
		retained := retainedPlan(t, s, id, pending.ID)
		if retained.Plan.Content.Outcome != original.Plan.Content.Outcome || retained.Plan.Approval != nil {
			t.Fatal("failure lost old draft or granted approval")
		}
		result, err := s.PublishPlanEditResult(id, pending.ID)
		if err != nil || result.Issue == "" || result.State != map[bool]string{false: "failed", true: "interrupted"}[cancel] {
			t.Fatal("revision failure was not retained", err)
		}
		if _, _, err := s.ApproveTaskPlan(id, original.ID, original.Plan.Revision, original.Plan.Digest); err == nil {
			t.Fatal("old review became usable after failure")
		}
	}
}
