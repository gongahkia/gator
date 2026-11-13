package worksession

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// PlanEditReference ties a tool-free correction to its immutable conversation
// turn. It joins the content digest; it never grants implementation authority.
type PlanEditReference struct {
	ParentID     string           `json:"parent_id"`
	TurnID       string           `json:"turn_id"`
	Instruction  string           `json:"instruction"`
	BaseRevision uint64           `json:"base_revision"`
	BaseDigest   string           `json:"base_digest"`
	Prior        []PlanCorrection `json:"prior_corrections,omitempty"`
}

// PlanCorrection retains an earlier user instruction, without claiming that
// a provider successfully applied it or granting execution authority.
type PlanCorrection struct {
	ParentID    string `json:"parent_id"`
	TurnID      string `json:"turn_id"`
	Instruction string `json:"instruction"`
}

func validCorrectionHistory(prior []PlanCorrection, current PlanCorrection) bool {
	if len(prior) > 32 {
		return false
	}
	seen := map[string]bool{current.ParentID + ":" + current.TurnID: true}
	bytes := len(current.Instruction)
	for _, item := range prior {
		key := item.ParentID + ":" + item.TurnID
		bytes += len(item.Instruction)
		if validID(item.ParentID) != nil || !planDigest(item.TurnID) || !boundedPlanText(item.Instruction, 64*1024) || seen[key] || bytes > 64*1024 {
			return false
		}
		seen[key] = true
	}
	return true
}

type PlanEdit struct {
	PlanEditReference
	State          string `json:"state"`
	ResultRevision uint64 `json:"result_revision,omitempty"`
	ResultDigest   string `json:"result_digest,omitempty"`
	Issue          string `json:"issue,omitempty"`
}

func (e *PlanEdit) clone() *PlanEdit {
	if e == nil {
		return nil
	}
	copy := *e
	copy.Prior = append([]PlanCorrection(nil), e.Prior...)
	return &copy
}
func planDigest(value string) bool {
	return len(value) == 64 && strings.Trim(value, "0123456789abcdef") == ""
}

func validatePlanEdit(r Request) error {
	p := r.Plan
	if p.Edit == nil {
		if p.Phase == "revising" {
			return errors.New("plan revision reference missing")
		}
		return nil
	}
	e := p.Edit
	if !validCorrectionHistory(e.Prior, PlanCorrection{e.ParentID, e.TurnID, e.Instruction}) {
		return errors.New("invalid retained correction history")
	}
	validBase := e.BaseRevision == 0 && e.BaseDigest == "" || e.BaseRevision > 0 && planDigest(e.BaseDigest)
	if validID(e.ParentID) != nil || !planDigest(e.TurnID) || !boundedPlanText(e.Instruction, 64*1024) || !validBase || e.BaseRevision >= p.Revision || len(e.Issue) > 4096 {
		return errors.New("invalid plan revision reference")
	}
	switch e.State {
	case "preparing":
		if p.Phase != "revising" || r.State != "queued" || p.Approval != nil || p.Revision != e.BaseRevision+1 || e.ResultRevision != 0 || e.ResultDigest != "" || e.Issue != "" {
			return errors.New("invalid pending plan revision")
		}
	case "applied":
		if p.Phase == "revising" || e.ResultRevision <= e.BaseRevision+1 || e.ResultRevision > p.Revision || !planDigest(e.ResultDigest) || p.Revision == e.ResultRevision && p.Digest != e.ResultDigest || e.Issue != "" {
			return errors.New("invalid retained plan revision result")
		}
	case "failed", "cancelled":
		if p.Phase == "revising" || !boundedPlanText(e.Issue, 4096) || e.ResultRevision != 0 || e.ResultDigest != "" {
			return errors.New("invalid plan revision failure")
		}
	default:
		return errors.New("invalid plan revision state")
	}
	return nil
}

// BeginTaskPlanEdit revokes the old review before calling a model. Sources,
// budgets, original task text/options, and the execution owner remain frozen.
// Transport repeats return the original attempt and never start a second call.
func (s Store) BeginTaskPlanEdit(id, request string, revision uint64, digest, parent, turnID, instruction string) (RequestJournal, Request, bool, error) {
	j, err := s.LoadRequests(id)
	if err != nil {
		return j, Request{}, false, err
	}
	for _, r := range j.Requests {
		if r.ID == request && r.Plan != nil && r.Plan.Edit != nil {
			e := r.Plan.Edit
			if e.ParentID == parent && e.TurnID == turnID && e.Instruction == instruction && e.BaseRevision == revision && e.BaseDigest == digest {
				return j, r, false, nil
			}
		}
	}
	j, index, err := s.taskPlanRequest(id, request, revision, digest)
	if err != nil {
		return j, Request{}, false, err
	}
	r := &j.Requests[index]
	if err := s.CheckPlanBinding(id, *r); err != nil {
		return j, Request{}, false, err
	}
	initial := r.State == "queued" && r.Plan.Phase == "preparing" && r.Plan.Content == nil
	retryInitial := r.State == "interrupted" && r.Plan.Phase == "interrupted" && r.Plan.Content == nil && r.Plan.Approval == nil
	review := r.State == "queued" && r.Plan.Content != nil && (r.Plan.Phase == "review" || r.Plan.Phase == "approved")
	revising := r.State == "queued" && r.Plan.Phase == "revising" && r.Plan.Edit != nil && r.Plan.Edit.State == "preparing" && r.Plan.Approval == nil
	if (!initial && !retryInitial && !review && !revising) || revision >= math.MaxUint64-1 || !boundedPlanText(instruction, 64*1024) {
		return j, Request{}, false, ErrPlanChanged
	}
	if r.Plan.Edit != nil && r.Plan.Edit.State != "preparing" {
		// Retain the previous correction's actual receipt before its reference
		// can be replaced by another explicit correction or retry.
		if _, err := s.PublishPlanEditResult(id, request); err != nil {
			return j, Request{}, false, err
		}
	}
	ref := PlanEditReference{ParentID: parent, TurnID: turnID, Instruction: instruction, BaseRevision: revision, BaseDigest: digest}
	if previous := r.Plan.Edit; previous != nil {
		ref.Prior = append([]PlanCorrection(nil), previous.Prior...)
		ref.Prior = append(ref.Prior, PlanCorrection{previous.ParentID, previous.TurnID, previous.Instruction})
	}
	if !validCorrectionHistory(ref.Prior, PlanCorrection{parent, turnID, instruction}) {
		return j, Request{}, false, errors.New("correction history is full; inspect the retained instructions before continuing")
	}
	if turn, err := s.planEditMessage(id, *r, ref); err != nil || turn.State != "editing_plan" {
		if err == nil {
			err = ErrPlanChanged
		}
		return j, Request{}, false, err
	}
	p := r.Plan.Clone()
	p.Revision++
	p.Phase, p.Approval = "revising", nil
	p.Edit = &PlanEdit{PlanEditReference: ref, State: "preparing"}
	p.Digest = p.contentDigest()
	r.Plan, r.UpdatedAt, r.Issue, r.State = p, time.Now().UTC(), "", "queued"
	saved, err := s.SaveRequests(id, j)
	return saved, *r, err == nil, err
}

func (s Store) planEditMessage(id string, r Request, ref PlanEditReference) (DialogueTurn, error) {
	turn, err := s.ReadDialogue(ref.ParentID, ref.TurnID)
	if err != nil {
		return turn, err
	}
	var decision struct{ Kind, Target, Instruction string }
	if json.Unmarshal(turn.Decision, &decision) != nil || decision.Kind != "revise_plan" || decision.Instruction != ref.Instruction {
		return turn, ErrPlanChanged
	}
	if ref.ParentID == id {
		if decision.Target != r.ID {
			return turn, ErrPlanChanged
		}
	} else {
		origin, err := s.DialogueTask(id)
		if err != nil || origin == nil || origin.ParentConversationID != ref.ParentID || origin.TurnID != r.ClientID || decision.Target != id {
			return turn, ErrPlanChanged
		}
	}
	return turn, nil
}

// FinishTaskPlanEdit applies only to the exact undispatched edit epoch. Failure
// keeps the old content for fresh human review, with its old approval revoked.
func (s Store) FinishTaskPlanEdit(id, request string, revision uint64, digest, turnID string, content *PlanContent, issue string) (RequestJournal, error) {
	j, index, err := s.taskPlanRequest(id, request, revision, digest)
	if err != nil {
		return j, err
	}
	r := &j.Requests[index]
	p := r.Plan.Clone()
	if r.State != "queued" || p.Phase != "revising" || p.Edit == nil || p.Edit.TurnID != turnID || p.Edit.State != "preparing" {
		return j, ErrPlanChanged
	}
	if content != nil {
		if issue != "" || !validPlanContent(*content, p.Sources) || p.Revision == math.MaxUint64 {
			return j, ErrPlanChanged
		}
		if err := s.CheckPlanBinding(id, *r); err != nil {
			return j, err
		}
		raw, _ := json.Marshal(content)
		var retained PlanContent
		_ = json.Unmarshal(raw, &retained)
		p.Content, p.Edit.State = &retained, "applied"
		p.Revision++
		p.Edit.ResultRevision = p.Revision
	} else {
		if !boundedPlanText(issue, 4096) {
			return j, errors.New("plan revision failure reason missing")
		}
		p.Edit.State, p.Edit.Issue = "failed", issue
	}
	p.Phase, p.Approval = "review", nil
	if p.Content == nil {
		p.Phase, r.State = "interrupted", "interrupted"
	}
	p.Digest = p.contentDigest()
	if content != nil {
		p.Edit.ResultDigest = p.Digest
	}
	r.Plan, r.Issue, r.UpdatedAt = p, issue, time.Now().UTC()
	return s.SaveRequests(id, j)
}

// PublishPlanEditResult retains the actual outcome in the requesting dialogue.
// The plan owner may finish after its parent view closes; neither journal needs
// to hold the other's execution lease. Final turns cannot be rewritten/replayed.
func (s Store) PublishPlanEditResult(id, request string) (DialogueTurn, error) {
	if err := s.PublishPriorPlanEdits(id, request); err != nil {
		return DialogueTurn{}, err
	}
	j, err := s.LoadRequests(id)
	if err != nil {
		return DialogueTurn{}, err
	}
	for _, r := range j.Requests {
		if r.ID != request || r.Plan == nil || r.Plan.Edit == nil {
			continue
		}
		e := r.Plan.Edit
		if e.State == "preparing" {
			return DialogueTurn{}, errors.New("plan revision is still preparing")
		}
		turn, err := s.planEditMessage(id, r, e.PlanEditReference)
		if err != nil {
			return turn, err
		}
		if turn.State == "guided" || turn.State == "failed" || turn.State == "interrupted" {
			return turn, nil
		}
		if turn.State != "editing_plan" {
			return turn, ErrPlanChanged
		}
		turn.UpdatedAt = time.Now().UTC()
		if turn.UpdatedAt.Before(turn.CreatedAt) {
			turn.UpdatedAt = turn.CreatedAt
		}
		switch e.State {
		case "applied":
			turn.State, turn.Response = "guided", "I saved the revised plan in the original task. Review the new plan before starting; no implementation was started."
		case "cancelled":
			turn.State, turn.Issue = "interrupted", e.Issue
			turn.Response = ""
		default:
			suffix := " The previous plan is retained for fresh review; no implementation was started."
			if r.Plan.Content == nil {
				suffix = " No proposal was saved. The original task and inputs are retained; ask to retry its plan. No implementation was started."
			}
			issue := e.Issue
			for len(issue) > 4096-len(suffix) {
				_, size := utf8.DecodeLastRuneInString(issue)
				issue = issue[:len(issue)-size]
			}
			turn.State, turn.Issue = "failed", issue+suffix
			turn.Response = ""
		}
		return s.UpdateDialogue(turn, turn.Revision)
	}
	return DialogueTurn{}, ErrPlanChanged
}

// PublishPriorPlanEdits reconciles displaced, still-pending revision receipts
// from the durable history. Completed or failed receipts are never rewritten.
func (s Store) PublishPriorPlanEdits(id, request string) error {
	j, err := s.LoadRequests(id)
	if err != nil {
		return err
	}
	for _, r := range j.Requests {
		if r.ID != request || r.Plan == nil || r.Plan.Edit == nil {
			continue
		}
		for _, prior := range r.Plan.Edit.Prior {
			turn, err := s.planEditMessage(id, r, PlanEditReference{ParentID: prior.ParentID, TurnID: prior.TurnID, Instruction: prior.Instruction})
			if err != nil {
				return err
			}
			if turn.State != "editing_plan" {
				continue
			}
			turn.State, turn.Response = "interrupted", ""
			turn.Issue = "A later correction superseded this proposal attempt. Your instruction is retained for the replacement; no implementation was started."
			turn.UpdatedAt = time.Now().UTC()
			if _, err := s.UpdateDialogue(turn, turn.Revision); err != nil {
				return err
			}
		}
		return nil
	}
	return ErrPlanChanged
}
