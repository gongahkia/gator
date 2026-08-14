package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workspace"
)

const maxGitOutputBytes = 128 * 1024

// GitStatus reports the exact worktree status without exposing general Git
// command execution to the model.
type GitStatus struct {
	Root workspace.Root
}

func (t GitStatus) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "git_status",
		Description: "Show the isolated worktree's concise Git status.",
		Parameters:  schema(`{"type":"object","additionalProperties":false}`),
	}
}

func (t GitStatus) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct{}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	output, truncated, err := runGit(ctx, t.Root.Path(), "status", "--short")
	if err != nil {
		return agent.ToolResult{}, err
	}
	result, err := success(struct {
		Status    string `json:"status"`
		Truncated bool   `json:"truncated"`
	}{Status: output, Truncated: truncated})
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: result}, nil
}

// GitDiff reports the current worktree diff for review and iterative edits.
type GitDiff struct {
	Root workspace.Root
}

func (t GitDiff) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "git_diff",
		Description: "Show the current isolated worktree diff. Use it before declaring a task complete.",
		Parameters:  schema(`{"type":"object","additionalProperties":false}`),
	}
}

func (t GitDiff) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct{}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	output, truncated, err := worktreeDiff(ctx, t.Root)
	if err != nil {
		return agent.ToolResult{}, err
	}
	result, err := success(struct {
		Diff      string `json:"diff"`
		Truncated bool   `json:"truncated"`
	}{Diff: output, Truncated: truncated})
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: result}, nil
}

func runGit(ctx context.Context, directory string, arguments ...string) (string, bool, error) {
	output, truncated, _, err := runGitCommand(ctx, directory, arguments...)
	if err == nil {
		return output, truncated, nil
	}
	if output == "" {
		return "", truncated, fmt.Errorf("run git %q: %w", arguments, err)
	}
	return "", truncated, fmt.Errorf("run git %q: %w: %s", arguments, err, strings.TrimSpace(output))
}

func worktreeDiff(ctx context.Context, root workspace.Root) (string, bool, error) {
	tracked, truncated, err := runGit(ctx, root.Path(), "diff", "--no-ext-diff", "--binary")
	if err != nil || truncated {
		return tracked, truncated, err
	}
	untracked, err := untrackedFiles(ctx, root)
	if err != nil {
		return "", false, err
	}
	var diff strings.Builder
	diff.WriteString(tracked)
	for _, relative := range untracked {
		if diff.Len() >= maxGitOutputBytes {
			return diff.String()[:maxGitOutputBytes], true, nil
		}
		path, err := root.ResolveFile(relative)
		if err != nil {
			return "", false, err
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		output, wasTruncated, exitCode, commandErr := runGitCommand(ctx, root.Path(), "diff", "--no-index", "--binary", "--", "/dev/null", path)
		if commandErr != nil && exitCode != 1 {
			return "", false, fmt.Errorf("diff untracked file %q: %w", relative, commandErr)
		}
		remaining := maxGitOutputBytes - diff.Len()
		if len(output) > remaining {
			diff.WriteString(output[:remaining])
			return diff.String(), true, nil
		}
		diff.WriteString(output)
		if wasTruncated {
			return diff.String(), true, nil
		}
	}
	return diff.String(), false, nil
}

func untrackedFiles(ctx context.Context, root workspace.Root) ([]string, error) {
	output, _, err := runGit(ctx, root.Path(), "status", "--porcelain=v1", "--untracked-files=all", "-z")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range strings.Split(output, "\x00") {
		if !strings.HasPrefix(entry, "?? ") {
			continue
		}
		path := strings.TrimPrefix(entry, "?? ")
		if filepath.Clean(path) == ".gator" || strings.HasPrefix(filepath.ToSlash(filepath.Clean(path)), ".gator/") {
			continue
		}
		files = append(files, path)
	}
	return files, nil
}

func runGitCommand(ctx context.Context, directory string, arguments ...string) (string, bool, int, error) {
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = directory
	output := &limitedBuffer{limit: maxGitOutputBytes}
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return output.String(), output.truncated, exitError.ExitCode(), err
		}
		return output.String(), output.truncated, -1, err
	}
	return output.String(), output.truncated, 0, nil
}
