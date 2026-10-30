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
		value, err := cliagent.Run(ctx, cliagent.Request{Config: cfg, Source: work.Source.Path(), Scratch: work.Scratch.Path(), ID: i.ID, Task: i.Task, Edit: edit, Scopes: request.Code.Scopes, Baseline: baseline, OnEvent: i.OnEvent})
		result := orchestrator.Result{Summary: value.Summary, BaselineSHA256: value.BaselineSHA256, Usage: value.Usage, Steps: value.Turns}
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
	return specialist
}
