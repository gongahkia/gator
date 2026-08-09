package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runGit(ctx context.Context, cwd string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = cwd
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func DiscoverWorkspace(ctx context.Context, cwd string) (Workspace, error) {
	if cwd == "" {
		return Workspace{}, fmt.Errorf("workspace directory is required")
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return Workspace{}, fmt.Errorf("resolve workspace: %w", err)
	}
	rootValue, err := runGit(ctx, abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return Workspace{}, fmt.Errorf("Gator requires a Git workspace: %w", err)
	}
	root := strings.TrimSpace(rootValue)
	branchValue, err := runGit(ctx, root, "branch", "--show-current")
	if err != nil {
		return Workspace{}, err
	}
	headValue, err := runGit(ctx, root, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return Workspace{}, err
	}
	status, err := runGit(ctx, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return Workspace{}, err
	}
	commonValue, err := runGit(ctx, root, "rev-parse", "--git-common-dir")
	if err != nil {
		return Workspace{}, err
	}
	common := strings.TrimSpace(commonValue)
	if !filepath.IsAbs(common) {
		common = filepath.Join(root, common)
	}
	kind := "project"
	if filepath.Clean(common) != filepath.Join(root, ".git") {
		kind = "worktree"
	}
	return Workspace{
		Root: root, Branch: strings.TrimSpace(branchValue), Head: strings.TrimSpace(headValue),
		Dirty: status != "", ChangeCount: len(strings.Split(strings.TrimSuffix(status, "\x00"), "\x00")), Kind: kind,
	}, nil
}

func WorkspaceDiff(ctx context.Context, workspace Workspace) (string, string, error) {
	diff, err := runGit(ctx, workspace.Root, "diff", "--no-ext-diff", "HEAD")
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256([]byte(diff))
	return diff, hex.EncodeToString(digest[:]), nil
}

func CreateWorktree(ctx context.Context, workspace Workspace, id string) (Workspace, error) {
	if id == "" || strings.ContainsAny(id, "/\\\x00") {
		return Workspace{}, fmt.Errorf("invalid worktree id")
	}
	parent := filepath.Join(filepath.Dir(workspace.Root), "gator-worktrees")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return Workspace{}, fmt.Errorf("create Gator worktree directory: %w", err)
	}
	target := filepath.Join(parent, id)
	if _, err := os.Lstat(target); err == nil {
		return Workspace{}, fmt.Errorf("Gator worktree path already exists: %s", target)
	} else if !os.IsNotExist(err) {
		return Workspace{}, fmt.Errorf("inspect Gator worktree path: %w", err)
	}
	branch := "gator/" + id
	if _, err := runGit(ctx, workspace.Root, "worktree", "add", "-b", branch, target, workspace.Head); err != nil {
		return Workspace{}, fmt.Errorf("create isolated Gator worktree: %w", err)
	}
	created, err := DiscoverWorkspace(ctx, target)
	if err != nil {
		_ = RemoveWorktree(context.Background(), workspace, target, branch)
		return Workspace{}, err
	}
	return created, nil
}

// RemoveWorktree is deliberately only used to roll back a failed creation. Finished
// experiments retain their worktree and branch for human inspection.
func RemoveWorktree(ctx context.Context, workspace Workspace, path, branch string) error {
	if _, err := runGit(ctx, workspace.Root, "worktree", "remove", "--force", path); err != nil {
		return err
	}
	_, err := runGit(ctx, workspace.Root, "branch", "--delete", "--force", branch)
	return err
}
