package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

type ExperimentRequest struct {
	Objective       string
	Provider        string
	BaselinePolicy  string
	CandidatePolicy string
	Execute         bool
}

func LoadPolicyFile(path string) (Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, err
	}
	var policy Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		return Policy{}, err
	}
	if err := policy.Validate(); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

// ComparePolicies creates separate Gator worktrees. It never writes to the
// active checkout. With Execute false it validates and records two reproducible
// plans; Execute requires a real provider and configured verification commands
// before the result can be conclusive.
func (e *Engine) ComparePolicies(ctx context.Context, request ExperimentRequest, input io.Reader, output, errors io.Writer) (ExperimentResult, error) {
	if request.Objective == "" || request.Provider == "" || request.BaselinePolicy == "" || request.CandidatePolicy == "" {
		return ExperimentResult{}, fmt.Errorf("experiment requires objective, provider, baseline policy, and candidate policy")
	}
	baseline, err := LoadPolicyFile(request.BaselinePolicy)
	if err != nil {
		return ExperimentResult{}, fmt.Errorf("load baseline policy: %w", err)
	}
	candidate, err := LoadPolicyFile(request.CandidatePolicy)
	if err != nil {
		return ExperimentResult{}, fmt.Errorf("load candidate policy: %w", err)
	}
	baselineRef := PolicyRef{SchemaVersion: baseline.SchemaVersion, Version: baseline.Version, Source: request.BaselinePolicy}
	candidateRef := PolicyRef{SchemaVersion: candidate.SchemaVersion, Version: candidate.Version, Source: request.CandidatePolicy}
	baselineRun, err := e.runWithPolicy(ctx, RunRequest{Objective: request.Objective, Provider: request.Provider, Role: "writer", Worktree: true, Execute: request.Execute}, baseline, baselineRef, input, output, errors)
	if err != nil && request.Execute {
		// The failed run is still an experiment observation; compare it with a
		// candidate rather than discarding evidence due to one provider failure.
		baselineRun, _ = e.store.GetRun(baselineRun.ID)
	}
	candidateRun, candidateErr := e.runWithPolicy(ctx, RunRequest{Objective: request.Objective, Provider: request.Provider, Role: "writer", Worktree: true, Execute: request.Execute}, candidate, candidateRef, input, output, errors)
	if candidateErr != nil && request.Execute {
		candidateRun, _ = e.store.GetRun(candidateRun.ID)
	}
	if baselineRun.ID == "" || candidateRun.ID == "" {
		return ExperimentResult{}, fmt.Errorf("experiment could not create both isolated runs")
	}
	verdict, reason := compareRuns(baselineRun, candidateRun, request.Execute)
	result := ExperimentResult{
		SchemaVersion: SchemaVersion, ID: e.id("experiment"), Objective: redactObjective(request.Objective), Provider: request.Provider,
		BaselineRunID: baselineRun.ID, CandidateRunID: candidateRun.ID, BaselinePolicy: baseline.Version,
		CandidatePolicy: candidate.Version, Verdict: verdict, Reason: reason, CreatedAt: time.Now().UTC(),
	}
	if err := e.store.PutExperiment(result); err != nil {
		return ExperimentResult{}, err
	}
	return result, nil
}

func compareRuns(baseline, candidate Run, executed bool) (string, string) {
	if !executed {
		return "inconclusive", "both policies produced isolated dry-run plans; execute with configured verification for evidence"
	}
	if baseline.Verification.State != "passed" || candidate.Verification.State != "passed" {
		return "inconclusive", "a policy comparison requires passed deterministic verification for both runs"
	}
	baselineDuration := baseline.FinishedAt.Sub(baseline.StartedAt)
	candidateDuration := candidate.FinishedAt.Sub(candidate.StartedAt)
	if candidateDuration < baselineDuration {
		return "candidate_better", "both runs passed verification and candidate completed faster"
	}
	if baselineDuration < candidateDuration {
		return "baseline_better", "both runs passed verification and baseline completed faster"
	}
	return "tie", "both runs passed verification with equal observed duration"
}
