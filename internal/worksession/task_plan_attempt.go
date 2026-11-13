package worksession

import (
	"errors"
	"math"
	"path/filepath"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/actionlog"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
)

// PlanAttempt is immutable prior execution evidence bound into later reviews.
// Its original manifest and files stay in the existing revision store.
type PlanAttempt struct {
	RunID          string                       `json:"run_id"`
	RevisionID     string                       `json:"revision_id"`
	ManifestSHA256 string                       `json:"manifest_sha256"`
	Execution      artifact.ExecutionAccounting `json:"execution"`
	Usage          agent.Usage                  `json:"usage"`
	CodeSteps      int                          `json:"code_steps"`
	WallSeconds    int                          `json:"wall_seconds"`
	Uncertain      bool                         `json:"uncertain"`
}

func (s Store) checkPlanAttemptEffects(p *TaskPlan) error {
	if len(p.Attempts) == 0 {
		return nil
	}
	log, err := actionlog.Open(filepath.Dir(filepath.Dir(s.root)))
	if err != nil {
		return err
	}
	for _, attempt := range p.Attempts {
		entries, err := log.List(attempt.RunID)
		if err != nil {
			return err
		}
		if attempt.Uncertain && len(entries) == 0 {
			return errors.New("inspect the original uncertain action receipt before continuing this task")
		}
		for _, entry := range entries {
			if entry.Live || entry.Action.Status == action.Unknown {
				return errors.New("resolve the original run's uncertain external action before approving replacement work")
			}
		}
	}
	return nil
}

func validatePlanAttempts(attempts []PlanAttempt) error {
	if len(attempts) > 32 {
		return errors.New("too many retained plan attempts")
	}
	seen := map[string]bool{}
	for _, a := range attempts {
		e := a.Execution
		if a.Usage.InputTokens > math.MaxInt64-a.Usage.OutputTokens {
			return errors.New("invalid retained token accounting")
		}
		if validID(a.RunID) != nil || validID(a.RevisionID) != nil || !planDigest(a.ManifestSHA256) || seen[a.RunID] || e.MaxSteps < 1 || e.Steps < 0 || e.Steps > e.MaxSteps || e.CodeMaxSteps < 0 || e.Limits.ModelRequests < 1 || e.Limits.ModelRequests > 4096 || e.Limits.WallSeconds < 1 || e.Limits.WallSeconds > 86400 || e.Limits.Tokens < 0 || a.Usage.ModelRequests < 0 || a.Usage.ModelRequests > 4096 || a.Usage.InputTokens < 0 || a.Usage.OutputTokens < 0 || a.CodeSteps < 0 || a.CodeSteps > 100000 || a.WallSeconds < 0 || a.WallSeconds > 86401 {
			return errors.New("invalid retained plan attempt")
		}
		seen[a.RunID] = true
	}
	return nil
}

// RemainingExecutionBudget cannot turn zero remaining work into default limits.
func (p TaskPlan) RemainingExecutionBudget() (PlanBudget, agent.Limits, error) {
	budget := p.Budget
	limits := agent.Limits{}
	if len(p.Attempts) == 0 {
		return budget, limits, nil
	}
	if err := validatePlanAttempts(p.Attempts); err != nil {
		return budget, limits, err
	}
	first := p.Attempts[0].Execution
	limits = first.Limits
	if budget.MaxSteps == 0 {
		budget.MaxSteps = first.MaxSteps
	}
	if budget.CodeMaxSteps == 0 {
		budget.CodeMaxSteps = first.CodeMaxSteps
	}
	for _, a := range p.Attempts {
		budget.MaxSteps -= a.Execution.Steps
		budget.CodeMaxSteps -= a.CodeSteps
		limits.ModelRequests -= a.Usage.ModelRequests
		limits.WallSeconds -= a.WallSeconds
		if limits.Tokens > 0 {
			limits.Tokens -= a.Usage.InputTokens + a.Usage.OutputTokens
			if limits.Tokens <= 0 {
				return budget, limits, errors.New("original token budget exhausted")
			}
		}
	}
	if budget.MaxSteps <= 0 || budget.CodeMaxSteps < 0 || limits.ModelRequests <= 0 || limits.WallSeconds <= 0 {
		return budget, limits, errors.New("original execution budget exhausted")
	}
	return budget, limits, nil
}

// RetainDrainedPlanRun accounts only the exact run and its newly retained head.
// It never executes or approves anything. Uncertain effects remain visible in
// the immutable attempt; execution admission must reconcile them separately.
func (s Store) RetainDrainedPlanRun(id, request string, revision uint64, digest, runID, revisionID string) (RequestJournal, error) {
	j, index, err := s.taskPlanRequest(id, request, revision, digest)
	if err != nil {
		return j, err
	}
	r := &j.Requests[index]
	p := r.Plan.Clone()
	pending := r.State == "queued" && p.Phase == "revising" && p.Edit != nil && p.Edit.State == "preparing"
	cancelled := r.State == "discarded" && p.Phase == "interrupted" && p.Edit != nil && p.Edit.State == "cancelled"
	if p.DrainingRun == "" || p.DrainingRun != runID || !pending && !cancelled || len(p.Attempts) >= 32 {
		return j, ErrPlanChanged
	}
	retained, err := s.LoadRevision(id, revisionID)
	if err != nil {
		return j, err
	}
	c, err := s.Load(id)
	if err != nil {
		return j, err
	}
	if c.HeadRevision != retained.ID || !c.UpdatedAt.Equal(retained.CreatedAt) || retained.ParentRevisionID != p.Binding.HeadRevision || retained.Objective != r.Text || planHash([]byte(c.SourcePath)) != p.Binding.SourceSHA256 || retained.SnapshotID != c.SnapshotID {
		return j, ErrPlanChanged
	}
	bundle, err := artifact.OpenBundle(retained.BundlePath)
	if err != nil {
		return j, err
	}
	if err := artifact.VerifyBundle(bundle); err != nil {
		return j, err
	}
	m := bundle.Manifest
	if m.RunID != runID || m.Objective != r.Text || m.Execution == nil {
		return j, errors.New("original execution has no verified budget accounting")
	}
	a := PlanAttempt{RunID: runID, RevisionID: revisionID, ManifestSHA256: bundle.ManifestSHA256, Execution: *m.Execution, Usage: m.Usage, WallSeconds: int(math.Ceil(m.FinishedAt.Sub(m.StartedAt).Seconds()))}
	for _, sub := range m.Subagents {
		if sub.Agent == "code" {
			a.CodeSteps += sub.Steps
		}
	}
	for _, effect := range m.Actions {
		a.Uncertain = a.Uncertain || effect.Status == action.Unknown
	}
	for _, old := range p.Attempts {
		if old.RunID == runID {
			return j, ErrPlanChanged
		}
	}
	p.Attempts = append(p.Attempts, a)
	p.DrainingRun = ""
	p.Binding.HeadRevision, p.Binding.SnapshotID, p.Binding.UpdatedAt = c.HeadRevision, c.SnapshotID, c.UpdatedAt
	p.Digest = p.contentDigest()
	r.Plan = p
	r.WorkID, r.RevisionID = runID, revisionID
	r.UpdatedAt = time.Now().UTC()
	return s.SaveRequests(id, j)
}
