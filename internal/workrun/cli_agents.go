package workrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/cliagent"
	"github.com/gongahkia/gator/internal/orchestrator"
	"github.com/gongahkia/gator/internal/workspace"
	"path/filepath"
)

func (e Executor) externalCLISpecialist(request Request, work workspace.Work, cfg cliagent.Config) orchestrator.Specialist {
	edit := cfg.AllowEdits && request.Mode != action.Inspect
	description := fmt.Sprintf("Run the configured %s CLI in an isolated source copy; separate provider billing, bounded runtime, no live steering. Returns findings", cfg.Kind)
	persistent := cfg.Kind == "codex" || cfg.Kind == "pi"
	if persistent {
		description = fmt.Sprintf("Work with %s in a retained isolated conversation. Use start_agent with continue to send follow-ups to the same native session, or steer_agent during active work. Separate provider billing, bounded runtime. Returns findings", cfg.Kind)
	}
	if edit {
		description += " and a reviewable patch; original files remain unchanged."
	} else {
		description += "; read-only for this run."
	}
	specialist := orchestrator.HostedSpecialist(cfg.Role(), description, func(ctx context.Context, i orchestrator.Invocation) (orchestrator.Result, error) {
		baseline, err := request.integration.baseline(i.Baseline)
		if err != nil {
			return orchestrator.Result{}, err
		}
		assignment := cliagent.Request{Config: cfg, Source: work.Source.Path(), Scratch: work.Scratch.Path(), ID: i.ID, Task: i.Task, Edit: edit, Scopes: request.Code.Scopes, Baseline: baseline, OnEvent: i.OnEvent}
		var value cliagent.Result
		if persistent {
			owner := request.ConversationID
			if owner == "" {
				owner = "run:" + request.RunID
			}
			capture := request.SnapshotID
			if capture == "" {
				capture = work.Source.Path()
			}
			store, pathErr := filepath.EvalSymlinks(request.StateDir)
			if pathErr != nil {
				return orchestrator.Result{}, pathErr
			}
			value, err = cliagent.RunSession(ctx, cliagent.SessionRequest{Request: assignment, Store: filepath.Join(store, "gator", "cli-sessions"), Owner: owner, Capture: capture, SessionID: i.SessionID, Steering: i.Steering, Controls: i.Controls, OnState: i.OnState})
		} else {
			value, err = cliagent.Run(ctx, assignment)
		}
		result := orchestrator.Result{SessionID: value.SessionID, Summary: value.Summary, BaselineSHA256: value.BaselineSHA256, Usage: value.Usage, Steps: value.Turns}
		if err != nil {
			return result, err
		}
		if len(value.Patch) > 0 {
			path := filepath.ToSlash(filepath.Join("code", request.RunID+"-"+i.ID+"-"+cfg.Kind+".patch"))
			if err := artifact.WriteBinary(work.Output, request.Contract, path, value.Patch); err != nil {
				return result, err
			}
			hash := sha256.Sum256(value.Patch)
			result.ArtifactPath = path
			result.ArtifactSHA256 = hex.EncodeToString(hash[:])
			request.integration.mu.Lock()
			request.integration.patches[path] = result.ArtifactSHA256
			request.integration.mu.Unlock()
			result.Summary += "\nPatch artifact for review: " + path
		}
		return result, nil
	})
	specialist.Configuration = orchestrator.RoleConfiguration{Version: 1, Provider: cfg.Kind + "/" + cfg.Model, MaxSteps: 16}
	specialist.Sessions, specialist.Steering = persistent, persistent
	return specialist
}
