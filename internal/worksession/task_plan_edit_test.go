package worksession

import (
	"bytes"
	"encoding/json"
	"path/filepath"
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

func TestInitialPlanCorrectionRetiresUnfinishedProposalWithoutChangingAdmission(t *testing.T) {
	s, id, initial := planFixture(t)
	initial = retainedPlan(t, s, id, initial.ID)
	turn := editMessage(t, s, id, initial.ID, "correct-initial-plan-001")
	_, pending, claimed, err := s.BeginTaskPlanEdit(id, initial.ID, 0, "", id, turn.ID, turn.Text)
	if err != nil || !claimed || pending.Plan.Content != nil || pending.Plan.Phase != "revising" || pending.Plan.Revision != 1 || pending.Plan.Edit.BaseRevision != 0 {
		t.Fatalf("initial correction: %+v %v", pending.Plan, err)
	}
	if !bytes.Equal(pending.Options, initial.Options) || pending.ID != initial.ID || pending.Text != initial.Text || pending.Plan.Binding != initial.Plan.Binding || pending.Plan.Budget != initial.Plan.Budget {
		t.Fatal("correction replaced frozen admission")
	}
	if _, err := s.ReviseTaskPlan(id, initial.ID, 0, "", proposedPlan()); err == nil {
		t.Fatal("obsolete first proposal replaced the correction")
	}
	if _, _, err := s.ApproveTaskPlan(id, initial.ID, pending.Plan.Revision, pending.Plan.Digest); err == nil {
		t.Fatal("unfinished correction approved")
	}
	if _, _, duplicate, err := s.BeginTaskPlanEdit(id, initial.ID, 0, "", id, turn.ID, turn.Text); err != nil || duplicate {
		t.Fatal("transport repeat started another correction", err)
	}
	content := proposedPlan()
	content.Outcome = "Corrected initial plan"
	if _, err := s.FinishTaskPlanEdit(id, pending.ID, pending.Plan.Revision, pending.Plan.Digest, turn.ID, &content, ""); err != nil {
		t.Fatal(err)
	}
	retained := retainedPlan(t, s, id, initial.ID)
	if retained.Plan.Revision != 2 || retained.Plan.Content.Outcome != content.Outcome || retained.Plan.Approval != nil || retained.State != "queued" {
		t.Fatal("correction did not require its own review")
	}
	if _, err := s.ReviseTaskPlan(id, initial.ID, 0, "", proposedPlan()); err == nil {
		t.Fatal("late first proposal revived after correction")
	}
	result, err := s.PublishPlanEditResult(id, initial.ID)
	if err != nil || result.State != "guided" || result.ID != turn.ID {
		t.Fatal("correction lost actual requesting turn", err)
	}
	if _, _, err := s.ApproveTaskPlan(id, retained.ID, retained.Plan.Revision, retained.Plan.Digest); err != nil {
		t.Fatal("fresh exact correction could not be reviewed", err)
	}
}

func TestInitialPlanCorrectionFailureCancellationAndExplicitRetrySurviveReopen(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure", true: "cancel"}[cancel], func(t *testing.T) {
			s, id, initial := planFixture(t)
			initial = retainedPlan(t, s, id, initial.ID)
			turn := editMessage(t, s, id, initial.ID, "initial-correction-failure")
			_, pending, _, err := s.BeginTaskPlanEdit(id, initial.ID, 0, "", id, turn.ID, turn.Text)
			if err != nil {
				t.Fatal(err)
			}
			if cancel {
				_, err = s.CancelTaskPlan(id, initial.ID, pending.Plan.Revision, pending.Plan.Digest)
			} else {
				_, err = s.FinishTaskPlanEdit(id, initial.ID, pending.Plan.Revision, pending.Plan.Digest, turn.ID, nil, "Provider disconnected")
			}
			if err != nil {
				t.Fatal(err)
			}
			s, err = Open(filepath.Dir(filepath.Dir(s.root)))
			if err != nil {
				t.Fatal(err)
			}
			retained := retainedPlan(t, s, id, initial.ID)
			if retained.Plan.Content != nil || retained.Plan.Phase != "interrupted" || retained.Plan.Approval != nil || !bytes.Equal(retained.Options, initial.Options) {
				t.Fatal("failed initial correction lost task or acquired authority")
			}
			result, err := s.PublishPlanEditResult(id, initial.ID)
			if err != nil || result.State != map[bool]string{false: "failed", true: "interrupted"}[cancel] || strings.Contains(result.Issue, "previous plan is retained") {
				t.Fatal("failure invented a previous proposal", err, result)
			}
			content := proposedPlan()
			if _, err := s.FinishTaskPlanEdit(id, initial.ID, pending.Plan.Revision, pending.Plan.Digest, turn.ID, &content, ""); err == nil {
				t.Fatal("old correction completion survived failure or cancellation")
			}
			if !cancel {
				retry := editMessage(t, s, id, initial.ID, "initial-correction-explicit-retry")
				_, next, claimed, err := s.BeginTaskPlanEdit(id, initial.ID, retained.Plan.Revision, retained.Plan.Digest, id, retry.ID, retry.Text)
				if err != nil || !claimed || next.ID != initial.ID || next.State != "queued" {
					t.Fatal("explicit retry replaced original task", err)
				}
				if _, err := s.FinishTaskPlanEdit(id, next.ID, next.Plan.Revision, next.Plan.Digest, retry.ID, &content, ""); err != nil {
					t.Fatal(err)
				}
			}
		})
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

func TestPlanCorrectionHistorySurvivesFailureRetryAndRestart(t *testing.T) {
	s, id, initial := planFixture(t)
	if _, err := s.ReviseTaskPlan(id, initial.ID, 0, "", proposedPlan()); err != nil {
		t.Fatal(err)
	}
	original := retainedPlan(t, s, id, initial.ID)
	first := editMessage(t, s, id, original.ID, "first-history-correction")
	_, pending, _, err := s.BeginTaskPlanEdit(id, original.ID, original.Plan.Revision, original.Plan.Digest, id, first.ID, first.Text)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishTaskPlanEdit(id, original.ID, pending.Plan.Revision, pending.Plan.Digest, first.ID, nil, "Provider disconnected"); err != nil {
		t.Fatal(err)
	}
	failed := retainedPlan(t, s, id, original.ID)
	second := editMessage(t, s, id, original.ID, "second-history-correction")
	_, next, _, err := s.BeginTaskPlanEdit(id, original.ID, failed.Plan.Revision, failed.Plan.Digest, id, second.ID, second.Text)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Plan.Edit.Prior) != 1 || next.Plan.Edit.Prior[0].TurnID != first.ID || next.Plan.Edit.Prior[0].Instruction != first.Text {
		t.Fatal("failed correction was discarded")
	}
	prior, err := s.ReadDialogue(id, first.ID)
	if err != nil || prior.State != "failed" {
		t.Fatal("earlier actual outcome was overwritten", err)
	}
	reloaded := retainedPlan(t, s, id, original.ID)
	if reloaded.Plan.Digest != next.Plan.Digest || len(reloaded.Plan.Edit.Prior) != 1 {
		t.Fatal("persisted history lost")
	}
	clone := reloaded.Plan.Clone()
	clone.Edit.Prior[0].Instruction = "mutated"
	if reloaded.Plan.Edit.Prior[0].Instruction != first.Text {
		t.Fatal("clone aliases immutable history")
	}
	reloaded.Plan.Edit.Prior[0].Instruction = "tampered"
	if err := validateTaskPlan(reloaded); err == nil {
		t.Fatal("history changed without invalidating review digest")
	}
	if _, _, err := s.ApproveTaskPlan(id, original.ID, failed.Plan.Revision, failed.Plan.Digest); err == nil {
		t.Fatal("prior review approved replacement")
	}
	if _, err := s.FinishTaskPlanEdit(id, original.ID, next.Plan.Revision, next.Plan.Digest, second.ID, ptrPlanContent(proposedPlan()), ""); err != nil {
		t.Fatal(err)
	}
	applied := retainedPlan(t, s, id, original.ID)
	if applied.Plan.Phase != "review" || applied.Plan.Approval != nil || len(applied.Plan.Edit.Prior) != 1 {
		t.Fatal("history granted approval or vanished")
	}
}

func ptrPlanContent(value PlanContent) *PlanContent { return &value }

func TestCorrectionHistoryRejectsDuplicatesAndUnboundedContext(t *testing.T) {
	current := PlanCorrection{ParentID: "chat", TurnID: strings.Repeat("a", 64), Instruction: "Latest instruction"}
	earlier := PlanCorrection{ParentID: "chat", TurnID: strings.Repeat("b", 64), Instruction: "Earlier instruction"}
	if !validCorrectionHistory([]PlanCorrection{earlier}, current) {
		t.Fatal("bounded history rejected")
	}
	for _, prior := range [][]PlanCorrection{{current}, {earlier, earlier}, {{ParentID: "chat", TurnID: "invalid", Instruction: "Earlier"}}, {{ParentID: "chat", TurnID: earlier.TurnID, Instruction: strings.Repeat("x", 64*1024)}}} {
		if validCorrectionHistory(prior, current) {
			t.Fatal("unsafe correction history accepted")
		}
	}
}
