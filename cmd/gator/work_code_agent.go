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

	"github.com/gongahkia/gator/internal/instructions"
	"github.com/gongahkia/gator/internal/patch"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/workrun"
)

const maxCodeSubagentSteps = 16

func (b *nativeWorkBackend) codeDelegate(stateDir string) workrun.CodeDelegate {
	return func(ctx context.Context, request workrun.CodeRequest) (workrun.CodeResult, error) {
		repository, err := prepareCodeSnapshotRepository(ctx, request.SourcePath, request.ScratchPath, request.ID)
		if err != nil {
			return workrun.CodeResult{}, err
		}
		steps := request.MaxSteps
		if steps <= 0 || steps > maxCodeSubagentSteps {
			steps = maxCodeSubagentSteps
		}
		digest := sha256.Sum256([]byte(request.ParentRunID + "\x00" + request.ID))
		runID := "code-" + hex.EncodeToString(digest[:8])
		code := b.code
		outcome, runErr := code.Execute(ctx, gatorrun.Request{
			RepositoryPath:          repository,
			Task:                    request.Task,
			Provider:                b.provider,
			Model:                   b.model,
			BaseURL:                 b.baseURL,
			RunID:                   runID,
			ThreadID:                runID,
			StateDir:                stateDir,
			MaxSteps:                steps,
			Verification:            [][]string{{"git", "diff", "--check"}},
			Mode:                    gatorrun.ExecuteMode,
			DisableWriterDelegation: true,
			RolePolicy: instructions.ProfilePolicy{
				Sandbox: "strict", Network: "deny", MaxSteps: steps,
				Omit: []string{
					instructions.OmitLSP, instructions.OmitMCP, instructions.OmitExtension,
					instructions.OmitHTTP, instructions.OmitBrowser, instructions.OmitTerminal,
					instructions.OmitDelegateWriter, instructions.OmitDelegateReadOnly,
				},
			},
			System: `You are the Gator Code specialist called by Gator Work. Implement only the bounded coding assignment against an isolated checkout of the parent's frozen source snapshot. The user-facing manager receives your summary and patch, not your full context. Do not broaden the task, access live source, use network services, or claim the patch was applied. Inspect the final diff and report exact changed paths and verification evidence.`,
		})
		result := workrun.CodeResult{Summary: strings.TrimSpace(outcome.Result.FinalText), Steps: outcome.Result.Steps}
		if outcome.Worktree.Path != "" {
			result.Patch, err = patch.Export(ctx, outcome.Worktree.Path, outcome.Worktree.BaseCommit)
			if err == nil {
				result.ChangedPaths, err = patch.Paths(ctx, outcome.Worktree.Path, outcome.Worktree.BaseCommit)
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

func prepareCodeSnapshotRepository(ctx context.Context, source, scratch, id string) (string, error) {
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
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"add", "--all"},
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
