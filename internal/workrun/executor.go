package workrun

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workspace"
)

// Execute creates a private non-Git workspace, runs the bounded agent loop,
// and seals a filesystem-backed manifest on both success and ordinary agent
// failure. The retained output can therefore be reviewed after an interrupted
// or unsuccessful attempt.
func (e Executor) Execute(ctx context.Context, request Request) (Outcome, error) {
	now := e.Now
	if now == nil {
		now = time.Now
	}
	request, err := e.normalizeAndValidate(request)
	if err != nil {
		return Outcome{}, err
	}
	if request.RunID == "" {
		request.RunID, err = newID(now())
		if err != nil {
			return Outcome{}, err
		}
	}
	startedAt := now()
	work, err := workspace.CreateWork(request.SourcePath, request.StateDir, request.RunID, startedAt)
	if err != nil {
		return Outcome{}, err
	}
	outcome := Outcome{Work: work}

	surface, err := tools.WorkFiles(work.Source, work.Output, request.Contract, request.Mode != action.Inspect)
	if err != nil {
		return outcome, err
	}
	events := make([]agent.Event, 0, 32)
	emit := func(event agent.Event) {
		events = append(events, event)
		if request.OnEvent != nil {
			request.OnEvent(event)
		}
	}
	runner := agent.Runner{Model: e.Model, Tools: surface, Now: now}
	var check func([]agent.Message) error
	if request.Mode != action.Inspect {
		check = outcomeCompletionCheck(work.Output, request.Contract)
	}
	result, runErr := runner.Run(ctx, agent.RunOptions{
		Task:            request.Objective,
		System:          systemPrompt(request),
		MaxSteps:        request.MaxSteps,
		OnEvent:         emit,
		Steering:        request.Steering,
		CompletionCheck: check,
	})
	outcome.Result = result
	outcome.Events = append([]agent.Event(nil), events...)

	failure := ""
	if runErr != nil {
		failure = runErr.Error()
	}
	manifest, sealErr := artifact.Seal(work.Output, request.Contract, artifact.SealOptions{
		RunID: request.RunID, Objective: request.Objective, Source: work.Source,
		Failure: failure, StartedAt: startedAt, FinishedAt: now(),
	})
	if sealErr != nil {
		if runErr != nil {
			return outcome, fmt.Errorf("work execution failed: %v; seal artifacts: %w", runErr, sealErr)
		}
		return outcome, fmt.Errorf("seal work artifacts: %w", sealErr)
	}
	outcome.Manifest = manifest
	if err := artifact.WriteManifest(work.ManifestPath, manifest); err != nil {
		if runErr != nil {
			return outcome, fmt.Errorf("work execution failed: %v; write manifest: %w", runErr, err)
		}
		return outcome, fmt.Errorf("write work manifest: %w", err)
	}
	if runErr != nil {
		return outcome, runErr
	}
	if manifest.Status != artifact.Completed {
		return outcome, errors.New("work outcome contract did not pass")
	}
	return outcome, nil
}

func (e Executor) normalizeAndValidate(request Request) (Request, error) {
	if e.Model == nil {
		return Request{}, errors.New("work agent model is required")
	}
	request.Objective = strings.TrimSpace(request.Objective)
	if request.Objective == "" || len(request.Objective) > 64*1024 || strings.ContainsRune(request.Objective, 0) {
		return Request{}, errors.New("work objective is missing or invalid")
	}
	if strings.TrimSpace(request.SourcePath) == "" {
		return Request{}, errors.New("work source directory is required")
	}
	if request.Mode == "" {
		request.Mode = action.Draft
	}
	if err := request.Mode.Validate(); err != nil {
		return Request{}, err
	}
	request.Contract = request.Contract.Normalize()
	if err := request.Contract.Validate(); err != nil {
		return Request{}, fmt.Errorf("validate outcome contract: %w", err)
	}
	if request.Mode == action.Inspect {
		if len(request.Contract.Artifacts) != 0 || request.Contract.ExternalActions != action.Forbid {
			return Request{}, errors.New("inspect mode requires an inspection-only contract")
		}
	} else if len(request.Contract.Artifacts) == 0 {
		return Request{}, errors.New("draft and act modes require at least one artifact")
	}
	if request.Mode == action.Draft && request.Contract.ExternalActions == action.Approve {
		return Request{}, errors.New("draft mode cannot approve external actions")
	}
	stateDir := strings.TrimSpace(request.StateDir)
	if stateDir == "" {
		stateDir = strings.TrimSpace(e.StateDir)
	}
	if stateDir == "" {
		return Request{}, errors.New("work state directory is required")
	}
	request.StateDir = stateDir
	if request.MaxSteps < 0 {
		return Request{}, errors.New("work max steps must not be negative")
	}
	return request, nil
}

func outcomeCompletionCheck(root workspace.Root, contract artifact.Contract) func([]agent.Message) error {
	return func(_ []agent.Message) error {
		inspection, err := artifact.Inspect(root, contract)
		if err != nil {
			return fmt.Errorf("inspect staged artifacts: %w", err)
		}
		if inspection.Passed {
			return nil
		}
		failures := make([]string, 0, 3)
		for _, result := range inspection.Validations {
			if result.Passed {
				continue
			}
			message := result.Path + " failed " + string(result.Kind)
			if result.Diagnostic != "" {
				message += ": " + result.Diagnostic
			}
			failures = append(failures, message)
			if len(failures) == 3 {
				break
			}
		}
		if len(failures) == 0 {
			return errors.New("the outcome contract has not passed")
		}
		return errors.New(strings.Join(failures, "; "))
	}
}

func newID(now time.Time) (string, error) {
	entropy := make([]byte, 6)
	if _, err := rand.Read(entropy); err != nil {
		return "", fmt.Errorf("generate work id: %w", err)
	}
	return "work-" + now.UTC().Format("20060102T150405") + "-" + hex.EncodeToString(entropy), nil
}
