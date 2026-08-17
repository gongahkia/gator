// Package patch exports and applies the complete reviewed state of a retained
// Gator worktree. It is deliberately invoked outside the agent tool loop.
package patch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/workspace"
)

const maxPatchBytes = 16 * 1024 * 1024

// Result describes a verified patch transfer without exposing its contents.
type Result struct {
	Bytes int
}

// Export produces a portable binary Git patch for all tracked and untracked
// regular files in a retained worktree. It skips untracked Gator metadata.
func Export(ctx context.Context, sourcePath, baseCommit string) ([]byte, error) {
	source, err := workspace.Open(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("open source worktree: %w", err)
	}
	base := "HEAD"
	if strings.TrimSpace(baseCommit) != "" {
		base = baseCommit
	}
	tracked, err := gitOutput(ctx, source.Path(), "diff", "--no-ext-diff", "--binary", base)
	if err != nil {
		return nil, err
	}
	patch := append([]byte(nil), tracked...)
	untracked, err := untrackedFiles(ctx, source)
	if err != nil {
		return nil, err
	}
	for _, relative := range untracked {
		path, err := source.ResolveFile(relative)
		if err != nil {
			return nil, fmt.Errorf("resolve untracked file %q: %w", relative, err)
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		fragment, exitCode, err := gitDiffNoIndex(ctx, source.Path(), relative)
		if err != nil && exitCode != 1 {
			return nil, fmt.Errorf("export untracked file %q: %w", relative, err)
		}
		if len(patch)+len(fragment) > maxPatchBytes {
			return nil, fmt.Errorf("review patch exceeds the %d MiB export limit", maxPatchBytes/(1024*1024))
		}
		patch = append(patch, fragment...)
	}
	if len(patch) > maxPatchBytes {
		return nil, fmt.Errorf("review patch exceeds the %d MiB export limit", maxPatchBytes/(1024*1024))
	}
	return patch, nil
}

// Check verifies that a patch can transfer to a clean target checkout without
// modifying it. A successful check is the compatibility criterion for apply.
func Check(ctx context.Context, sourcePath, baseCommit, targetPath string) (Result, error) {
	patch, target, err := prepare(ctx, sourcePath, baseCommit, targetPath)
	if err != nil {
		return Result{}, err
	}
	if err := applyPatch(ctx, target.Path(), patch, true); err != nil {
		return Result{}, err
	}
	return Result{Bytes: len(patch)}, nil
}

// Apply transfers a reviewed worktree patch to a clean compatible target
// checkout. Callers must make this explicit; normal Gator runs never invoke it.
func Apply(ctx context.Context, sourcePath, baseCommit, targetPath string) (Result, error) {
	patch, target, err := prepare(ctx, sourcePath, baseCommit, targetPath)
	if err != nil {
		return Result{}, err
	}
	if err := applyPatch(ctx, target.Path(), patch, true); err != nil {
		return Result{}, err
	}
	if err := applyPatch(ctx, target.Path(), patch, false); err != nil {
		return Result{}, err
	}
	return Result{Bytes: len(patch)}, nil
}

// ApplySnapshot restores a previously exported worktree snapshot into an
// already-clean isolated worktree. Forking uses this narrower primitive rather
// than the public Apply flow because the target is created from the snapshot's
// recorded base commit and is never the developer's active checkout.
func ApplySnapshot(ctx context.Context, targetPath string, snapshot []byte) error {
	if len(snapshot) == 0 {
		return nil
	}
	target, err := workspace.Open(targetPath)
	if err != nil {
		return fmt.Errorf("open fork worktree: %w", err)
	}
	if err := requireClean(ctx, target.Path()); err != nil {
		return fmt.Errorf("fork worktree is not clean: %w", err)
	}
	if err := applyPatch(ctx, target.Path(), snapshot, true); err != nil {
		return err
	}
	if err := applyPatch(ctx, target.Path(), snapshot, false); err != nil {
		return err
	}
	return nil
}

func prepare(ctx context.Context, sourcePath, baseCommit, targetPath string) ([]byte, workspace.Root, error) {
	source, err := workspace.Open(sourcePath)
	if err != nil {
		return nil, workspace.Root{}, fmt.Errorf("open source worktree: %w", err)
	}
	target, err := workspace.Open(targetPath)
	if err != nil {
		return nil, workspace.Root{}, fmt.Errorf("open target checkout: %w", err)
	}
	if source.Path() == target.Path() {
		return nil, workspace.Root{}, errors.New("source worktree and target checkout must differ")
	}
	if err := requireClean(ctx, target.Path()); err != nil {
		return nil, workspace.Root{}, err
	}
	patch, err := Export(ctx, source.Path(), baseCommit)
	if err != nil {
		return nil, workspace.Root{}, err
	}
	if len(patch) == 0 {
		return nil, workspace.Root{}, errors.New("retained worktree has no patch to export")
	}
	return patch, target, nil
}

func requireClean(ctx context.Context, target string) error {
	status, err := gitOutput(ctx, target, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return fmt.Errorf("inspect target checkout: %w", err)
	}
	if strings.TrimSpace(string(status)) != "" {
		return errors.New("target checkout must be clean before applying a Gator patch")
	}
	return nil
}

func untrackedFiles(ctx context.Context, root workspace.Root) ([]string, error) {
	status, err := gitOutput(ctx, root.Path(), "status", "--porcelain=v1", "--untracked-files=all", "-z")
	if err != nil {
		return nil, fmt.Errorf("list untracked files: %w", err)
	}
	files := make([]string, 0)
	for _, entry := range strings.Split(string(status), "\x00") {
		if !strings.HasPrefix(entry, "?? ") {
			continue
		}
		relative := strings.TrimPrefix(entry, "?? ")
		clean := filepath.ToSlash(filepath.Clean(relative))
		if clean == ".gator" || strings.HasPrefix(clean, ".gator/") {
			continue
		}
		files = append(files, relative)
	}
	return files, nil
}

func gitDiffNoIndex(ctx context.Context, directory, relative string) ([]byte, int, error) {
	command := exec.CommandContext(ctx, "git", "diff", "--no-index", "--binary", "--", "/dev/null", relative)
	command.Dir = directory
	return runCommand(command)
}

func gitOutput(ctx context.Context, directory string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = directory
	output, exitCode, err := runCommand(command)
	if err != nil {
		if len(output) > 0 {
			return nil, fmt.Errorf("git %s (exit %d): %w: %s", strings.Join(arguments, " "), exitCode, err, strings.TrimSpace(string(output)))
		}
		return nil, fmt.Errorf("git %s: %w", strings.Join(arguments, " "), err)
	}
	return output, nil
}

func applyPatch(ctx context.Context, directory string, patch []byte, checkOnly bool) error {
	arguments := []string{"apply"}
	if checkOnly {
		arguments = append(arguments, "--check")
	}
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = directory
	command.Stdin = bytes.NewReader(patch)
	output, _, err := runCommand(command)
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return fmt.Errorf("patch is not compatible with the clean target checkout: %w", err)
		}
		return fmt.Errorf("patch is not compatible with the clean target checkout: %w: %s", err, message)
	}
	return nil
}

func runCommand(command *exec.Cmd) ([]byte, int, error) {
	var output boundedBuffer
	output.limit = maxPatchBytes
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()
	if output.truncated {
		return output.Bytes(), -1, fmt.Errorf("Git output exceeds the %d MiB limit", maxPatchBytes/(1024*1024))
	}
	if err == nil {
		return output.Bytes(), 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return output.Bytes(), exitError.ExitCode(), err
	}
	return output.Bytes(), -1, err
}

type boundedBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(value), nil
	}
	if len(value) > remaining {
		_, _ = b.Buffer.Write(value[:remaining])
		b.truncated = true
		return len(value), nil
	}
	return b.Buffer.Write(value)
}
