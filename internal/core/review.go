package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"
	"time"
)

func (e *Engine) VerifyAll(ctx context.Context, runID string, output, errors io.Writer) ([]Evidence, error) {
	run, err := e.store.GetRun(runID)
	if err != nil {
		return nil, err
	}
	policy, _, err := LoadPolicy(e.workspace.Root)
	if err != nil {
		return nil, err
	}
	if len(policy.Verification.Commands) == 0 {
		return []Evidence{}, fmt.Errorf("effective policy has no configured verification commands")
	}
	evidence := make([]Evidence, 0, len(policy.Verification.Commands))
	for _, command := range policy.Verification.Commands {
		result, err := e.verify(ctx, run, command, output, errors)
		if err != nil {
			return evidence, err
		}
		evidence = append(evidence, result)
		if !result.Passed {
			run.Verification.State = "failed"
			break
		}
	}
	if run.Verification.State != "failed" {
		run.Verification.State = "passed"
	}
	for _, result := range evidence {
		run.Verification.EvidenceIDs = append(run.Verification.EvidenceIDs, result.ID)
	}
	_, diff, diffErr := WorkspaceDiff(ctx, run.Workspace)
	if diffErr == nil {
		run.Verification.DiffSHA256 = diff
	}
	run.UpdatedAt = time.Now().UTC()
	if err := e.store.PutRun(run); err != nil {
		return evidence, err
	}
	if err := e.event(run, "review.verified", run.Verification.State, "configured verification completed", nil, evidence[len(evidence)-1].ID); err != nil {
		return evidence, err
	}
	if run.Verification.State == "failed" {
		return evidence, fmt.Errorf("verification failed")
	}
	return evidence, nil
}

func (e *Engine) verify(ctx context.Context, run Run, spec CommandSpec, output, errors io.Writer) (Evidence, error) {
	started := time.Now().UTC()
	command := exec.CommandContext(ctx, spec.Argv[0], spec.Argv[1:]...)
	command.Dir, command.Stdout, command.Stderr = run.Workspace.Root, output, errors
	err := command.Run()
	finished := time.Now().UTC()
	code := exitCode(err)
	_, diff, diffErr := WorkspaceDiff(ctx, run.Workspace)
	if diffErr != nil {
		return Evidence{}, diffErr
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", spec.ID, code, diff)))
	value := Evidence{
		SchemaVersion: SchemaVersion, ID: e.id("evidence"), RunID: run.ID, Kind: "verification_command", CommandID: spec.ID,
		Argv: append([]string(nil), spec.Argv...), ExitCode: code, Passed: err == nil, StartedAt: started, FinishedAt: finished,
		OutputSHA256: hex.EncodeToString(digest[:]), BaseSHA: run.Workspace.Head, DiffSHA256: diff,
	}
	if err := e.store.PutEvidence(value); err != nil {
		return Evidence{}, err
	}
	return value, nil
}
