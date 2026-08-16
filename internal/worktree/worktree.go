// Package worktree creates isolated Git worktrees for coding-agent runs.
package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gongahkia/gator/internal/workspace"
)

var runIDPattern = regexp.MustCompile(`\A[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}\z`)

// Worktree is one isolated checkout retained for agent review and recovery.
type Worktree struct {
	ID         string
	Repository string
	Path       string
	BaseCommit string
	Root       workspace.Root
}

// Create adds a detached worktree at a sibling location of the source
// repository. Keeping it outside the active checkout prevents the run's files
// and metadata from being confused with the developer's own changes.
func Create(ctx context.Context, repositoryPath, runID string) (Worktree, error) {
	if !runIDPattern.MatchString(runID) {
		return Worktree{}, fmt.Errorf("invalid run id %q", runID)
	}
	repository, err := repositoryRoot(ctx, repositoryPath)
	if err != nil {
		return Worktree{}, err
	}
	base := filepath.Join(filepath.Dir(repository), filepath.Base(repository)+"-gator-runs")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return Worktree{}, fmt.Errorf("create Gator worktree directory: %w", err)
	}
	path := filepath.Join(base, runID)
	if _, err := os.Lstat(path); err == nil {
		return Worktree{}, fmt.Errorf("Gator worktree already exists for run %q", runID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Worktree{}, fmt.Errorf("inspect Gator worktree path: %w", err)
	}

	command := exec.CommandContext(ctx, "git", "worktree", "add", "--detach", path, "HEAD")
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err != nil {
		return Worktree{}, commandError("create isolated Git worktree", err, output)
	}
	root, err := workspace.Open(path)
	if err != nil {
		return Worktree{}, fmt.Errorf("open created Gator worktree: %w", err)
	}
	baseCommit, err := revision(ctx, root.Path())
	if err != nil {
		return Worktree{}, err
	}
	return Worktree{ID: runID, Repository: repository, Path: root.Path(), BaseCommit: baseCommit, Root: root}, nil
}

// OpenExisting validates a retained worktree before a resumed agent run uses
// it. The caller supplies its original repository identity from local session
// state; Git confirms that the target is still a worktree.
func OpenExisting(ctx context.Context, repository, path, runID string) (Worktree, error) {
	if !runIDPattern.MatchString(runID) {
		return Worktree{}, fmt.Errorf("invalid run id %q", runID)
	}
	root, err := workspace.Open(path)
	if err != nil {
		return Worktree{}, fmt.Errorf("open retained worktree: %w", err)
	}
	command := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	command.Dir = root.Path()
	output, err := command.CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "true" {
		if err != nil {
			return Worktree{}, commandError("validate retained Git worktree", err, output)
		}
		return Worktree{}, errors.New("retained path is not a Git worktree")
	}
	baseCommit, err := revision(ctx, root.Path())
	if err != nil {
		return Worktree{}, err
	}
	return Worktree{ID: runID, Repository: repository, Path: root.Path(), BaseCommit: baseCommit, Root: root}, nil
}

func revision(ctx context.Context, directory string) (string, error) {
	command := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		return "", commandError("read worktree base revision", err, output)
	}
	revision := strings.TrimSpace(string(output))
	if revision == "" {
		return "", errors.New("Git did not return a worktree base revision")
	}
	return revision, nil
}

func repositoryRoot(ctx context.Context, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("repository path is required")
	}
	command := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	command.Dir = path
	output, err := command.CombinedOutput()
	if err != nil {
		return "", commandError("find Git repository root", err, output)
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", errors.New("Git did not return a repository root")
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve Git repository root: %w", err)
	}
	return canonical, nil
}

func commandError(action string, err error, output []byte) error {
	message := strings.TrimSpace(string(output))
	if message == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, message)
}
