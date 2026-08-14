package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workspace"
)

const maxPatchBytes = 512 * 1024

// ApplyPatch applies one unified diff inside the isolated run worktree. Git
// itself rejects paths outside the working tree; no unsafe-path option is used.
type ApplyPatch struct {
	Root workspace.Root
}

func (t ApplyPatch) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "apply_patch",
		Description: "Apply a unified diff to the isolated run worktree. Include tests with feature changes.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"required":["patch"],"properties":{"patch":{"type":"string","minLength":1}}}`),
	}
}

func (t ApplyPatch) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Patch string `json:"patch"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	if strings.TrimSpace(arguments.Patch) == "" {
		return agent.ToolResult{}, errors.New("patch is required")
	}
	if len(arguments.Patch) > maxPatchBytes {
		return agent.ToolResult{}, fmt.Errorf("patch exceeds the %d-byte limit", maxPatchBytes)
	}
	if err := runGitApply(ctx, t.Root.Path(), "--check", arguments.Patch); err != nil {
		return agent.ToolResult{}, fmt.Errorf("patch check failed: %w", err)
	}
	if err := runGitApply(ctx, t.Root.Path(), "--apply", arguments.Patch); err != nil {
		return agent.ToolResult{}, fmt.Errorf("apply checked patch: %w", err)
	}
	result, err := success(struct {
		Applied bool `json:"applied"`
	}{Applied: true})
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: result}, nil
}

func runGitApply(ctx context.Context, directory, mode, patch string) error {
	arguments := []string{"apply", "--whitespace=nowarn"}
	if mode == "--check" {
		arguments = append(arguments, "--check")
	}
	arguments = append(arguments, "-")
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = directory
	command.Stdin = strings.NewReader(patch)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, message)
	}
	return nil
}
