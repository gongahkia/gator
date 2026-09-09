package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/browser"
	"github.com/gongahkia/gator/internal/instructions"
	"github.com/gongahkia/gator/internal/patch"
	"github.com/gongahkia/gator/internal/projectcapture"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/workrun"
)

const maxCodeSubagentSteps = 32

func (b *nativeWorkBackend) codeDelegate(stateDir string) workrun.CodeDelegate {
	return func(ctx context.Context, request workrun.CodeRequest) (workrun.CodeResult, error) {
		policy := request.Policy.Sandbox.Normalize()
		policy.WritablePaths = append([]string(nil), request.Policy.Scopes...)
		if err := policy.Validate(); err != nil {
			return workrun.CodeResult{}, err
		}
		ctx = sandbox.WithEnvelope(ctx, policy)
		repository, err := prepareCodeSnapshotRepository(ctx, request.SourcePath, request.ScratchPath, request.ID, request.Project)
		if err != nil {
			return workrun.CodeResult{}, err
		}

		originalCommand := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
		originalCommand.Dir = repository
		original, err := originalCommand.Output()
		if err != nil {
			return workrun.CodeResult{}, err
		}
		if len(request.Baseline) > 0 {
			if err := patch.ApplySnapshot(ctx, repository, request.Baseline); err != nil {
				return workrun.CodeResult{}, err
			}
			for _, args := range [][]string{{"add", "--all"}, {"-c", "user.name=Gator", "-c", "user.email=gator@invalid", "commit", "--quiet", "--allow-empty", "-m", "selected staged candidate"}} {
				command := exec.CommandContext(ctx, "git", args...)
				command.Dir = repository
				if output, err := command.CombinedOutput(); err != nil {
					return workrun.CodeResult{}, fmt.Errorf("stage baseline: %w: %s", err, output)
				}
			}
		}
		baselineCommand := exec.CommandContext(ctx, "git", "rev-parse", "HEAD^{tree}")
		baselineCommand.Dir = repository
		baselineTree, err := baselineCommand.Output()
		if err != nil {
			return workrun.CodeResult{}, err
		}
		steps := request.Policy.MaxSteps
		if steps <= 0 {
			steps = request.MaxSteps
		}
		if steps <= 0 || steps > maxCodeSubagentSteps {
			steps = maxCodeSubagentSteps
		}
		digest := sha256.Sum256([]byte(request.ParentRunID + "\x00" + request.ID))
		runID := "code-" + hex.EncodeToString(digest[:8])
		code := b.code
		if request.Policy.BrowserSession != "" {
			if !request.Policy.HasCapability(workrun.CodeCapabilityBrowser) || request.Policy.Sandbox.Normalize().Network != sandbox.AllowNetwork {
				return workrun.CodeResult{}, errors.New("Code browser session requires explicit browser and network grants")
			}
			store, err := browser.Open(stateDir)
			if err != nil {
				return workrun.CodeResult{}, err
			}
			client, err := browser.NewClient(store, request.Policy.BrowserSession)
			if err != nil {
				return workrun.CodeResult{}, err
			}
			session, err := client.Session(ctx, request.Policy.BrowserSession)
			if err != nil {
				return workrun.CodeResult{}, err
			}
			if len(session.SelectedTabs) == 0 {
				return workrun.CodeResult{}, errors.New("Code browser session has no selected tabs")
			}
			code.Browser = client
		}
		if !request.Policy.HasCapability(workrun.CodeCapabilityHooks) {
			code.HookTrusts = nil
		}
		if !request.Policy.HasCapability(workrun.CodeCapabilityMCP) {
			code.MCPTrusts = nil
		}
		usage := &agent.Budget{Limits: agent.Limits{ModelRequests: 4096}}
		code.Model = agent.WithBudget(agent.WithBudget(code.Model, request.Budget), usage)
		code.Sandbox = request.Policy.Sandbox.Normalize()
		verification := mergeCodeVerification(request.Policy.Verification)
		omitted := []string{instructions.OmitDelegateWriter, instructions.OmitDelegateReadOnly}
		for _, capability := range []struct{ name, omit string }{
			{workrun.CodeCapabilityLSP, instructions.OmitLSP},
			{workrun.CodeCapabilityMCP, instructions.OmitMCP},
			{workrun.CodeCapabilityExtension, instructions.OmitExtension},
			{workrun.CodeCapabilityHTTP, instructions.OmitHTTP},
			{workrun.CodeCapabilityBrowser, instructions.OmitBrowser},
			{workrun.CodeCapabilityTerminal, instructions.OmitTerminal},
		} {
			if !request.Policy.HasCapability(capability.name) {
				omitted = append(omitted, capability.omit)
			}
		}
		outcome, runErr := code.Execute(ctx, gatorrun.Request{
			RepositoryPath: repository,
			OnEvent:        request.OnEvent,
			TrustIdentity: func() string {
				if request.Project != nil {
					return request.Project.Origin
				}
				return ""
			}(),
			Task:                    request.Task,
			Provider:                b.provider,
			Model:                   b.model,
			BaseURL:                 b.baseURL,
			RunID:                   runID,
			ThreadID:                runID,
			StateDir:                stateDir,
			MaxSteps:                steps,
			Verification:            verification,
			Scopes:                  append([]string(nil), request.Policy.Scopes...),
			WritePaths:              append([]string(nil), request.Policy.Scopes...),
			Profile:                 request.Policy.Profile,
			Setup:                   cloneCodeCommands(request.Policy.Setup),
			AllowedCommands:         cloneCodeCommands(request.Policy.AllowedCommands),
			AllowedCommandPrefixes:  cloneCodeCommands(request.Policy.AllowedCommandPrefixes),
			Approve:                 request.Approve,
			BrowserSession:          request.Policy.BrowserSession,
			Mode:                    gatorrun.ExecuteMode,
			DisableWriterDelegation: true,
			RolePolicy: instructions.ProfilePolicy{
				MaxSteps: steps, Omit: omitted,
			},
			System: `You are the internal Gator Code specialist called by the user-facing Gator manager. Implement only the bounded coding assignment against an isolated checkout of the parent's frozen source snapshot. The manager receives your summary and patch, not your full context. Never broaden the task, access live source, or claim the patch was applied. Use only capabilities explicitly present in this delegation and report exact changed paths and verification evidence.`,
		})
		baseline := sha256.Sum256([]byte(strings.TrimSpace(string(baselineTree))))
		result := workrun.CodeResult{BaselineSHA256: hex.EncodeToString(baseline[:]), Usage: usage.Usage(), Summary: strings.TrimSpace(outcome.Result.FinalText), Steps: outcome.Result.Steps}
		if outcome.Worktree.Path != "" {
			result.Patch, err = patch.Export(ctx, outcome.Worktree.Path, strings.TrimSpace(string(original)))
			if err == nil {
				result.ChangedPaths, err = patch.Paths(ctx, outcome.Worktree.Path, outcome.Worktree.BaseCommit)
			}
		}

		if len(request.Policy.Scopes) > 0 {
			for _, path := range result.ChangedPaths {
				allowed := false
				for _, scope := range request.Policy.Scopes {
					scope = strings.TrimSuffix(filepath.ToSlash(filepath.Clean(scope)), "/")
					if path == scope || strings.HasPrefix(path, scope+"/") {
						allowed = true
					}
				}
				if !allowed {
					runErr = fmt.Errorf("Code changed path %q outside the assigned path envelope", path)
				}
			}
		}
		if err != nil && runErr != nil {
			return result, fmt.Errorf("code specialist run: %v; export patch: %w", runErr, err)
		}
		if err != nil {
			return result, fmt.Errorf("export code specialist patch: %w", err)
		}
		if runErr != nil {
			return result, fmt.Errorf("code specialist run: %w", runErr)
		}
		return result, nil
	}
}

func mergeCodeVerification(configured [][]string) [][]string {
	result := [][]string{{"git", "diff", "--check"}}
	seen := map[string]struct{}{strings.Join(result[0], "\x00"): {}}
	for _, command := range configured {
		key := strings.Join(command, "\x00")
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, append([]string(nil), command...))
	}
	return result
}

func cloneCodeCommands(commands [][]string) [][]string {
	result := make([][]string, 0, len(commands))
	for _, command := range commands {
		result = append(result, append([]string(nil), command...))
	}
	return result
}

func prepareCodeSnapshotRepository(ctx context.Context, source, scratch, id string, configuration ...*projectcapture.Bundle) (string, error) {
	if strings.TrimSpace(source) == "" || strings.TrimSpace(scratch) == "" || strings.TrimSpace(id) == "" || strings.ContainsAny(id, `/\\`) {
		return "", errors.New("code specialist snapshot paths are invalid")
	}
	repository := filepath.Join(scratch, id+"-source")
	if err := os.Mkdir(repository, 0o700); err != nil {
		return "", fmt.Errorf("create code specialist source: %w", err)
	}
	if err := copyCodeSnapshot(source, repository); err != nil {
		return "", err
	}
	if len(configuration) > 0 && configuration[0] != nil {
		if err := configuration[0].Install(repository); err != nil {
			return "", err
		}
	}
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"add", "--all", "--force"},
		{"-c", "user.name=Gator", "-c", "user.email=gator@invalid", "commit", "--quiet", "--allow-empty", "--message", "frozen Work source"},
	} {
		command := exec.CommandContext(ctx, "git", arguments...)
		command.Dir = repository
		output, err := command.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("prepare code specialist repository with git %s: %w: %s", strings.Join(arguments, " "), err, strings.TrimSpace(string(output)))
		}
	}
	return repository, nil
}

func copyCodeSnapshot(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("frozen code source contains symlink %q", relative)
		}
		if entry.IsDir() {
			return os.Mkdir(target, 0o700)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("frozen code source contains non-regular file %q", relative)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm()|0o200)
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeOutputErr := output.Close()
		closeInputErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeOutputErr != nil {
			return closeOutputErr
		}
		return closeInputErr
	})
}
