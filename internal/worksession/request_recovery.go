package worksession

import (
	"encoding/json"
	"errors"
	"time"
)

// RequestRecovery describes an admitted task's actual failure outcome, never
// a model claim. A safe retry still requires explicit review and original scope.
type RequestRecovery struct {
	Needs            []TaskAccessNeed `json:"needs,omitempty"`
	ProcessingDigest string           `json:"processing_digest,omitempty"`
	ScopeDigest      string           `json:"scope_digest,omitempty"`
	Kind             string           `json:"kind"`
	Status           int              `json:"status,omitempty"`
	Summary          string           `json:"summary"`
	Repair           string           `json:"repair"`
	RetrySafe        bool             `json:"retry_safe"`
}

func validateRequestRecovery(value *RequestRecovery) error {
	if value == nil {
		return nil
	}
	switch value.Kind {
	case "source_access", "credentials", "model_access", "rate_limit", "provider_budget", "work_budget":
	default:
		return errors.New("invalid request recovery kind")
	}
	if err := ValidateTaskAccessNeeds(value.Needs); err != nil {
		return err
	}
	if value.ProcessingDigest != "" && !planDigest(value.ProcessingDigest) {
		return errors.New("invalid original processing scope")
	}
	if value.ScopeDigest != "" && !planDigest(value.ScopeDigest) {
		return errors.New("invalid request recovery scope")
	}
	if value.Status < 0 || value.Status > 599 || !boundedPlanText(value.Summary, 1024) || !boundedPlanText(value.Repair, 1024) {
		return errors.New("invalid request recovery metadata")
	}
	return nil
}

// PrepareFailedRequestReview keeps the logical request while accounting its
// failed read-only attempt. It cannot replay work; only tool-free planning follows.
func (s Store) PrepareFailedRequestReview(id, request, parent, turnID, instruction string) (RequestJournal, Request, error) {
	return s.PrepareFailedRequestAccessReview(id, request, parent, turnID, instruction, nil)
}
func (s Store) PrepareFailedRequestAccessReview(id, request, parent, turnID, instruction string, access *PlanAccessChange) (RequestJournal, Request, error) {
	j, err := s.LoadRequests(id)
	if err != nil {
		return j, Request{}, err
	}
	index := -1
	for i, r := range j.Requests {
		if r.ID == request {
			index = i
			break
		}
	}
	if index < 0 {
		return j, Request{}, ErrPlanChanged
	}
	r := &j.Requests[index]
	if r.State != "failed" || r.Recovery == nil || !r.Recovery.RetrySafe || r.Plan == nil && r.ExecutionBoundary == nil || validID(r.WorkID) != nil || validID(r.RevisionID) != nil {
		return j, Request{}, errors.New("inspect the original outcome before preparing a retry")
	}
	if access != nil {
		if err := access.validate(); err != nil {
			return j, Request{}, err
		}
		if len(r.Recovery.Needs) == 0 || access.BeforeScope != r.Recovery.ScopeDigest || access.ProcessingDigest != r.Recovery.ProcessingDigest {
			return j, Request{}, errors.New("task access review differs from the original recovery scope")
		}
	}
	c, err := s.Load(id)
	if err != nil {
		return j, Request{}, err
	}
	ref := PlanEditReference{ParentID: parent, TurnID: turnID, Instruction: instruction}
	turn, err := s.planEditMessage(id, *r, ref)
	if err != nil || turn.State != "editing_plan" {
		return j, Request{}, ErrPlanChanged
	}
	var frozen struct {
		ConnectorIDs []string
		MaxSteps     int
		Code         struct{ MaxSteps int }
	}
	if json.Unmarshal(r.Options, &frozen) != nil {
		return j, Request{}, ErrPlanChanged
	}
	p := r.Plan.Clone()
	if p == nil {
		original := append(json.RawMessage(nil), r.Options...)
		gated, err := promotePlanOptions(original)
		if err != nil {
			return j, Request{}, err
		}
		r.Options = gated
		p = newTaskPlan(c, *r, frozen.ConnectorIDs, PlanBudget{MaxSteps: frozen.MaxSteps, CodeMaxSteps: frozen.Code.MaxSteps})
		p.Binding.HeadRevision, p.Binding.SnapshotID, p.Binding.UpdatedAt = r.ExecutionBoundary.HeadRevision, r.ExecutionBoundary.SnapshotID, r.ExecutionBoundary.UpdatedAt
		p.Binding.SourceSHA256 = r.ExecutionBoundary.SourceSHA256
		p.InputChanges = []PlanInputChange{{ParentID: parent, TurnID: turnID, BeforeOptions: original, AfterSHA256: optionsHash(gated), Promoted: true}}
	} else {
		if p.Phase != "finished" || p.Approval == nil || p.Revision >= ^uint64(0)-1 {
			return j, Request{}, ErrPlanChanged
		}
		ref.BaseRevision, ref.BaseDigest = p.Revision, p.Digest
		if previous := p.Edit; previous != nil {
			if _, err := s.PublishPlanEditResult(id, request); err != nil {
				return j, Request{}, err
			}
			ref.Prior = append(ref.Prior, previous.Prior...)
			ref.Prior = append(ref.Prior, PlanCorrection{previous.ParentID, previous.TurnID, previous.Instruction})
		}
	}
	if access != nil {
		before := append(json.RawMessage(nil), r.Options...)
		after, err := updatePlanConnections(before, access.ConnectorIDs)
		if err != nil {
			return j, Request{}, err
		}
		if r.Plan == nil {
			change := &p.InputChanges[0]
			change.Access = access
			change.AfterSHA256 = optionsHash(after)
		} else {
			if len(p.InputChanges) >= 32 {
				return j, Request{}, errors.New("reviewed task scope history is full")
			}
			p.InputChanges = append(p.InputChanges, PlanInputChange{ParentID: parent, TurnID: turnID, BeforeOptions: before, AfterSHA256: optionsHash(after), Access: access})
		}
		r.Options = after
		p.Sources = append([]string{}, access.ConnectorIDs...)
		p.Binding.OptionsSHA256 = optionsHash(after)
		recovery := *r.Recovery
		recovery.ScopeDigest = access.AfterScope
		r.Recovery = &recovery
	}
	if len(p.Attempts) >= 32 || !validCorrectionHistory(ref.Prior, PlanCorrection{parent, turnID, instruction}) {
		return j, Request{}, errors.New("retained attempt or correction history is full")
	}
	p.Revision++
	p.Phase, p.Approval = "revising", nil
	p.DrainingRun = r.WorkID
	p.Edit = &PlanEdit{PlanEditReference: ref, State: "preparing"}
	p, err = s.accountDrainedPlanRun(id, *r, p, r.WorkID, r.RevisionID)
	if err != nil {
		return j, Request{}, err
	}
	if err := s.checkPlanAttemptEffects(p); err != nil {
		return j, Request{}, err
	}
	if _, _, err := p.RemainingExecutionBudget(); err != nil {
		return j, Request{}, err
	}
	r.Plan = p
	r.State = "queued"
	r.UpdatedAt = time.Now().UTC()
	r.Issue = ""
	saved, err := s.SaveRequests(id, j)
	return saved, *r, err
}
