package run

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/harness"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workspace"
	"github.com/gongahkia/gator/internal/worktree"
)

func (e Executor) runHarness(ctx context.Context, isolated worktree.Worktree, request Request, projectInstructions string, history []agent.Message, emit agent.EventSink) (agent.Result, error) {
	task := request.Task
	if continuation := continuationTask(history); continuation != "" {
		task += "\n\nDeveloper continuation:\n" + continuation
	}
	delegated, err := e.Harness.Run(ctx, harness.Request{
		Root:     isolated.Root.Path(),
		Task:     task,
		System:   joinInstructions(projectInstructions, request.System),
		Model:    request.Model,
		MaxSteps: request.MaxSteps,
		OnEvent:  emit,
	})
	result := agent.Result{FinalText: delegated.FinalText, Steps: 1}
	if err != nil {
		return result, err
	}
	if err := verifyHarnessOutcome(ctx, isolated.Root, request.Verification, emit, e.now); err != nil {
		return result, err
	}
	emit(agent.Event{Kind: agent.EventRunFinished, At: e.now(), Step: 1, Text: result.FinalText})
	return result, nil
}

func continuationTask(history []agent.Message) string {
	for index := len(history) - 1; index >= 0; index-- {
		message := history[index]
		if message.Role != agent.RoleUser {
			continue
		}
		const prefix = "Continue the original task with this developer instruction:\n"
		if strings.HasPrefix(message.Content, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(message.Content, prefix))
		}
	}
	return ""
}

func verifyHarnessOutcome(ctx context.Context, root workspace.Root, verification [][]string, emit agent.EventSink, now func() time.Time) error {
	if now == nil {
		now = time.Now
	}
	for index, argv := range verification {
		arguments, err := json.Marshal(struct {
			Argv []string `json:"argv"`
		}{Argv: argv})
		if err != nil {
			return fmt.Errorf("encode verification command: %w", err)
		}
		call := agent.ToolCall{ID: fmt.Sprintf("harness-verify-%d", index+1), Name: "run_command", Arguments: arguments}
		emit(agent.Event{Kind: agent.EventToolCalled, At: now(), Step: 1, ToolCall: &call})
		result, err := tools.RunAllowedCommand(ctx, root, tools.CommandPolicy{Allowed: verification}, argv)
		finished := agent.Event{Kind: agent.EventToolFinished, At: now(), Step: 1, ToolCall: &call, ToolResult: result.Output}
		if err != nil {
			finished.ToolError = err.Error()
			emit(finished)
			return err
		}
		if result.ExitCode != 0 {
			finished.ToolError = fmt.Sprintf("verification exited %d", result.ExitCode)
			emit(finished)
			return fmt.Errorf("required verification failed: %s (exit %d): %s", strings.Join(argv, " "), result.ExitCode, compactCommandOutput(result.Output))
		}
		emit(finished)
	}
	if err := inspectHarnessWorktree(ctx, root, "git_status", now, emit); err != nil {
		return err
	}
	return inspectHarnessWorktree(ctx, root, "git_diff", now, emit)
}

func inspectHarnessWorktree(ctx context.Context, root workspace.Root, name string, now func() time.Time, emit agent.EventSink) error {
	call := agent.ToolCall{ID: "harness-" + name, Name: name}
	emit(agent.Event{Kind: agent.EventToolCalled, At: now(), Step: 1, ToolCall: &call})
	var (
		result agent.ToolResult
		err    error
	)
	switch name {
	case "git_status":
		result, err = (tools.GitStatus{Root: root}).Execute(ctx, json.RawMessage(`{}`))
	case "git_diff":
		result, err = (tools.GitDiff{Root: root}).Execute(ctx, json.RawMessage(`{}`))
	default:
		err = fmt.Errorf("unknown harness inspection %q", name)
	}
	finished := agent.Event{Kind: agent.EventToolFinished, At: now(), Step: 1, ToolCall: &call, ToolResult: result.Content}
	if err != nil {
		finished.ToolError = err.Error()
	}
	emit(finished)
	return err
}

func compactCommandOutput(output string) string {
	output = strings.Join(strings.Fields(output), " ")
	if output == "" {
		return "no output"
	}
	if len(output) > 500 {
		return output[:499] + "…"
	}
	return output
}
