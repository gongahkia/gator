package worksession

import (
	"encoding/json"
	"errors"
	"path/filepath"
)

// PreparationBoundary is captured by the native owner before any Work runner
// starts. An absent bundle alone never establishes this fact.
func (s Store) PreparationBoundary(id, request, text, source string, options json.RawMessage) (PlanBinding, error) {
	j, err := s.LoadRequests(id)
	if err != nil {
		return PlanBinding{}, err
	}
	c, err := s.Load(id)
	if err != nil {
		return PlanBinding{}, err
	}
	actual, err := filepath.Abs(source)
	if err != nil || actual != c.SourcePath {
		return PlanBinding{}, ErrPlanChanged
	}
	for _, r := range j.Requests {
		if r.ID != request {
			continue
		}
		if r.State != "running" || r.Text != text || optionsHash(options) != optionsHash(r.Options) {
			return PlanBinding{}, ErrPlanChanged
		}
		if r.Plan != nil {
			if err := s.CheckPlanDispatch(id, r); err != nil {
				return PlanBinding{}, err
			}
		} else {
			b := r.ExecutionBoundary
			if b == nil || b.ConversationID != id || b.RequestID != r.ID || b.OptionsSHA256 != optionsHash(r.Options) || b.PromptSHA256 != planHash([]byte(text)) || b.SourceSHA256 != planHash([]byte(c.SourcePath)) || b.HeadRevision != c.HeadRevision || b.SnapshotID != c.SnapshotID || b.HeadRevision != "" && !b.UpdatedAt.Equal(c.UpdatedAt) {
				return PlanBinding{}, ErrPlanChanged
			}
		}
		return newTaskPlan(c, r, nil, PlanBudget{}).Binding, nil
	}
	return PlanBinding{}, ErrPlanChanged
}

func validatePreparation(r Request) error {
	if r.Recovery == nil || r.Recovery.Preparation == nil {
		return nil
	}
	b := r.Recovery.Preparation
	if validID(b.ConversationID) != nil || b.RequestID != r.ID || b.UpdatedAt.IsZero() || !planDigest(b.SourceSHA256) || !planDigest(b.OptionsSHA256) || b.PromptSHA256 != planHash([]byte(r.Text)) {
		return errors.New("invalid preparation failure identity")
	}
	if r.WorkID != "" || r.RevisionID != "" {
		return errors.New("a preparation failure cannot claim a completed Work attempt")
	}
	matching := optionsHash(r.Options) == b.OptionsSHA256
	if r.Plan != nil {
		for _, change := range r.Plan.InputChanges {
			matching = matching || optionsHash(change.BeforeOptions) == b.OptionsSHA256
		}
	}
	if !matching {
		return errors.New("preparation failure differs from retained original options")
	}
	return nil
}
func (s Store) checkPreparationContext(id string, r Request) error {
	if err := validatePreparation(r); err != nil {
		return err
	}
	if r.Recovery == nil || r.Recovery.Preparation == nil {
		return errors.New("no native preparation failure is retained")
	}
	b := r.Recovery.Preparation
	c, err := s.Load(id)
	if err != nil {
		return err
	}
	if b.ConversationID != id || b.HeadRevision != c.HeadRevision || b.SnapshotID != c.SnapshotID || b.SourceSHA256 != planHash([]byte(c.SourcePath)) || !b.UpdatedAt.Equal(c.UpdatedAt) {
		return ErrPlanChanged
	}
	return nil
}
