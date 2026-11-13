package worksession

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

func (s Store) ExecutionBoundary(id string, r Request) (PlanBinding, error) {
	c, err := s.Load(id)
	if err != nil {
		return PlanBinding{}, err
	}
	return newTaskPlan(c, r, nil, PlanBudget{}).Binding, nil
}
func promotePlanOptions(before json.RawMessage) (json.RawMessage, error) {
	validated, _, _, err := mergePlanInputs(before, nil, nil)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(validated, &fields)
	key := "PlanFirst"
	for candidate := range fields {
		if strings.EqualFold(candidate, key) {
			key = candidate
			break
		}
	}
	var opted bool
	if raw := fields[key]; len(raw) > 0 && json.Unmarshal(raw, &opted) != nil {
		return nil, errors.New("invalid original review intent")
	}
	if opted {
		return nil, errors.New("task already requires plan review")
	}
	fields[key] = json.RawMessage("true")
	return json.Marshal(fields)
}

// PromoteRunningTaskInputs changes only review intent and selected files. The
// caller must fence the actual runner first, then cancel it after persistence.
func (s Store) PromoteRunningTaskInputs(id, request, parent, turnID, instruction, runID string) (RequestJournal, Request, error) {
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
	if r.Plan != nil || r.State != "running" || r.ExecutionBoundary == nil || validID(runID) != nil {
		return j, Request{}, ErrPlanChanged
	}
	c, err := s.Load(id)
	if err != nil {
		return j, Request{}, err
	}
	b := r.ExecutionBoundary
	if b.ConversationID != id || c.HeadRevision != b.HeadRevision || c.SnapshotID != b.SnapshotID || planHash([]byte(c.SourcePath)) != b.SourceSHA256 || b.HeadRevision != "" && !c.UpdatedAt.Equal(b.UpdatedAt) {
		return j, Request{}, ErrPlanChanged
	}
	ref := PlanEditReference{ParentID: parent, TurnID: turnID, Instruction: instruction}
	turn, err := s.planEditMessage(id, *r, ref)
	if err != nil || turn.State != "editing_plan" {
		return j, Request{}, ErrPlanChanged
	}
	var input struct {
		Source     string `json:"source"`
		RunOptions struct {
			Uploads, Attachments []string
			ReplyToUpdateID      string
		} `json:"run_options"`
	}
	if json.Unmarshal(turn.Options, &input) != nil || input.Source != c.SourcePath || input.RunOptions.ReplyToUpdateID != "" || len(input.RunOptions.Uploads)+len(input.RunOptions.Attachments) == 0 {
		return j, Request{}, errors.New("review transition requires the correction's explicit file selection")
	}
	var frozen struct {
		ConnectorIDs []string
		MaxSteps     int
		Code         struct{ MaxSteps int }
	}
	if json.Unmarshal(r.Options, &frozen) != nil {
		return j, Request{}, ErrPlanChanged
	}
	original := append(json.RawMessage(nil), r.Options...)
	gated, err := promotePlanOptions(original)
	if err != nil {
		return j, Request{}, err
	}
	after, uploads, attachments, err := mergePlanInputs(gated, input.RunOptions.Uploads, input.RunOptions.Attachments)
	if err != nil {
		return j, Request{}, err
	}
	r.Options = after
	p := newTaskPlan(c, *r, frozen.ConnectorIDs, PlanBudget{MaxSteps: frozen.MaxSteps, CodeMaxSteps: frozen.Code.MaxSteps})
	p.Revision = 1
	p.Phase = "revising"
	p.DrainingRun = runID
	p.Edit = &PlanEdit{PlanEditReference: ref, State: "preparing"}
	p.InputChanges = []PlanInputChange{{ParentID: parent, TurnID: turnID, BeforeOptions: original, AddedUploads: uploads, AddedAttachments: attachments, AfterSHA256: optionsHash(after), Promoted: true}}
	p.Digest = p.contentDigest()
	r.Plan = p
	r.State = "queued"
	r.UpdatedAt = time.Now().UTC()
	r.Issue = ""
	saved, err := s.SaveRequests(id, j)
	return saved, *r, err
}
