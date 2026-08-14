package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
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
	output, truncated, err := runGit(ctx, t.Root.Path(), "diff", "--no-ext-diff", "--binary")
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
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = directory
	output := &limitedBuffer{limit: maxGitOutputBytes}
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	if err != nil {
		message := strings.TrimSpace(output.String())
		if message == "" {
			return "", output.truncated, fmt.Errorf("run git %q: %w", arguments, err)
		}
		return "", output.truncated, fmt.Errorf("run git %q: %w: %s", arguments, err, message)
	}
	return output.String(), output.truncated, nil
}
