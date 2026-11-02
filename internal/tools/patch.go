package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/hostproc"
	"github.com/gongahkia/gator/internal/workspace"
)

const maxPatchBytes = 512 * 1024

// ApplyPatch applies one unified diff inside the isolated run worktree. Git
// itself rejects paths outside the working tree; no unsafe-path option is used.
type ApplyPatch struct {
	AllowedPaths []string
	Root         workspace.Root
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
	if len(t.AllowedPaths) > 0 {
		command := hostproc.CommandContext(ctx, "git", "apply", "--numstat", "-z", "-")
		command.Dir = t.Root.Path()
		command.Stdin = strings.NewReader(arguments.Patch)
		data, err := command.Output()
		if err != nil {
			return agent.ToolResult{}, err
		}
		for _, entry := range strings.Split(string(data), "\x00") {
			if entry == "" {
				continue
			}
			fields := strings.SplitN(entry, "\t", 3)
			if len(fields) != 3 || fields[2] == "" {
				return agent.ToolResult{}, errors.New("scoped patches cannot rename paths")
			}
			path := fields[2]
			allowed := false
			for _, scope := range t.AllowedPaths {
				if path == scope || strings.HasPrefix(path, strings.TrimSuffix(scope, "/")+"/") {
					allowed = true
				}
			}
			if !allowed {
				return agent.ToolResult{}, fmt.Errorf("patch path %q is outside the assigned write envelope", path)
			}
		}
	}
	if err := runGitApply(ctx, t.Root.Path(), "--check", arguments.Patch); err != nil {
		return agent.ToolResult{}, fmt.Errorf("patch check failed: %w", err)
	}
	if err := runGitApply(ctx, t.Root.Path(), "--apply", arguments.Patch); err != nil {
		return agent.ToolResult{}, fmt.Errorf("apply checked patch: %w", err)
	}
	paths, unavailable := appliedPatchPaths(ctx, t.Root.Path(), arguments.Patch)
	result, err := success(struct {
		Applied          bool     `json:"applied"`
		ChangedPaths     []string `json:"changed_paths,omitempty"`
		PathsUnavailable bool     `json:"paths_unavailable,omitempty"`
	}{Applied: true, ChangedPaths: paths, PathsUnavailable: unavailable})
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
	command := hostproc.CommandContext(ctx, "git", arguments...)
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

// Optional display evidence from Git after successful application. Failure to
// project paths must never turn an already-applied patch into a retryable error.
func appliedPatchPaths(ctx context.Context, directory, patch string) ([]string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	command := hostproc.CommandContext(ctx, "git", "apply", "--numstat", "-z", "-")
	command.Dir = directory
	command.Stdin = strings.NewReader(patch)
	output := &limitedBuffer{limit: maxGitOutputBytes}
	command.Stdout = output
	command.Stderr = io.Discard
	if command.Run() != nil || output.truncated {
		return nil, true
	}
	paths := []string{}
	for _, entry := range strings.Split(output.String(), "\x00") {
		if entry == "" {
			continue
		}
		fields := strings.SplitN(entry, "\t", 3)
		if len(fields) != 3 || fields[2] == "" {
			return nil, true
		}
		if len(paths) >= 32 {
			return paths, true
		}
		paths = append(paths, fields[2])
	}
	return paths, false
}
