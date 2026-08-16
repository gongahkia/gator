// Package run orchestrates one isolated native coding-agent execution.
package run

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/harness"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workspace"
	"github.com/gongahkia/gator/internal/worktree"
)

// Request configures a single coding task. Every listed verification argv is
// both exposed to the agent and required to succeed before completion.
type Request struct {
	RepositoryPath string
	Task           string
	Provider       string
	Model          string
	BaseURL        string
	RunID          string
	MaxSteps       int
	Verification   [][]string
	System         string
	StateDir       string
	OnEvent        agent.EventSink
	ThreadID       string
	Mode           Mode
	// AllowExternalCLI is required for delegated vendor CLIs because their
	// tool permission model is separate from Gator's native allowlist.
	AllowExternalCLI bool
}

// Mode controls the native agent tool surface for a turn. Plan mode is
// intentionally enforced by omitting mutation and command tools.
type Mode uint8

const (
	ExecuteMode Mode = iota
	PlanMode
)

func (m Mode) String() string {
	switch m {
	case ExecuteMode:
		return "execute"
	case PlanMode:
		return "plan"
	default:
		return "unknown"
	}
}

// Outcome preserves the reviewable artifacts of a completed or failed run.
type Outcome struct {
	Worktree  worktree.Worktree
	StatePath string
	ThreadID  string
	Result    agent.Result
	Events    []agent.Event
}

// Executor combines the provider-independent loop with an isolated worktree.
type Executor struct {
	Model    agent.Model
	Harness  harness.Runner
	Now      func() time.Time
	StateDir string
}

// Execute creates a new detached worktree and runs the native agent inside it.
// A worktree is retained even on failure so a developer can inspect recovery
// state rather than losing a partially completed patch.
func (e Executor) Execute(ctx context.Context, request Request) (Outcome, error) {
	if err := e.validateRequest(request); err != nil {
		return Outcome{}, err
	}
	runID := request.RunID
	if runID == "" {
		generated, err := newID(e.now())
		if err != nil {
			return Outcome{}, err
		}
		runID = generated
	}
	request.RunID = runID
	if request.ThreadID == "" {
		request.ThreadID = runID
	}
	isolated, err := worktree.Create(ctx, request.RepositoryPath, runID)
	if err != nil {
		return Outcome{}, err
	}
	return e.execute(ctx, isolated, request, nil, "")
}

// Resume continues a retained worktree from a private local session. It starts
// a new run record and re-requires final status, diff, and verification evidence.
func (e Executor) Resume(ctx context.Context, previous journal.Session, statePath, continuation string, request Request) (Outcome, error) {
	if strings.TrimSpace(continuation) == "" {
		return Outcome{}, errors.New("resume task is required")
	}
	request.RepositoryPath = previous.Repository
	request.Task = previous.Task
	request.Provider = previous.Provider
	request.Model = previous.Model
	request.BaseURL = previous.BaseURL
	request.Verification = previous.Verification
	if request.MaxSteps == 0 {
		request.MaxSteps = previous.MaxSteps
	}
	if request.MaxSteps == 0 {
		request.MaxSteps = defaultResumeMaxSteps
	}
	if request.ThreadID == "" {
		request.ThreadID = previous.ThreadID
		if request.ThreadID == "" {
			request.ThreadID = filepath.Base(statePath)
		}
	}
	if err := e.validateRequest(request); err != nil {
		return Outcome{}, err
	}
	generated, err := newID(e.now())
	if err != nil {
		return Outcome{}, err
	}
	request.RunID = generated
	isolated, err := worktree.OpenExisting(ctx, previous.Repository, previous.WorktreePath, generated)
	if err != nil {
		return Outcome{}, err
	}
	history := append([]agent.Message(nil), previous.Messages...)
	history = append(history, agent.Message{Role: agent.RoleUser, Content: "Continue the original task with this developer instruction:\n" + continuation})
	return e.execute(ctx, isolated, request, history, statePath)
}

const defaultResumeMaxSteps = 24

func (e Executor) validateRequest(request Request) error {
	if (e.Model == nil && e.Harness == nil) || (e.Model != nil && e.Harness != nil) {
		return errors.New("exactly one agent model or CLI harness is required")
	}
	if strings.TrimSpace(request.Task) == "" {
		return errors.New("run task is required")
	}
	if err := validateVerification(request.Verification); err != nil {
		return err
	}
	if strings.TrimSpace(request.Provider) == "" {
		return errors.New("run provider is required")
	}
	if e.Harness != nil && !request.AllowExternalCLI {
		return errors.New("external CLI harness requires explicit approval")
	}
	if request.Mode != ExecuteMode && request.Mode != PlanMode {
		return fmt.Errorf("unsupported run mode %d", request.Mode)
	}
	if request.Mode == PlanMode && e.Harness != nil {
		return errors.New("enforced Plan mode is unavailable for delegated CLI providers; choose a native provider or switch to Execute")
	}
	return nil
}

func (e Executor) execute(ctx context.Context, isolated worktree.Worktree, request Request, initialMessages []agent.Message, parentStatePath string) (Outcome, error) {
	projectInstructions, err := loadProjectInstructions(isolated.Repository)
	if err != nil {
		return Outcome{Worktree: isolated}, err
	}
	stateDir := request.StateDir
	if stateDir == "" {
		stateDir = e.StateDir
	}
	runJournal, record, err := journal.Open(isolated.Repository, request.RunID, isolated.Path, stateDir, e.now())
	if err != nil {
		return Outcome{Worktree: isolated}, err
	}
	defer runJournal.Close()

	var events []agent.Event
	var journalErr error
	emit := func(event agent.Event) {
		events = append(events, event)
		if journalErr == nil {
			journalErr = runJournal.Append(event)
		}
		if request.OnEvent != nil {
			request.OnEvent(event)
		}
	}
	var result agent.Result
	if e.Harness != nil {
		result, err = e.runHarness(ctx, isolated, request, projectInstructions, initialMessages, emit)
	} else {
		runTools := tools.Default(isolated.Root, tools.CommandPolicy{Allowed: request.Verification})
		system := systemPrompt(joinInstructions(projectInstructions, request.System), request.Verification)
		var check func([]agent.Message) error
		if request.Mode == PlanMode {
			runTools = tools.ReadOnly(isolated.Root)
			system = planSystemPrompt(joinInstructions(projectInstructions, request.System))
		} else {
			check = completionCheck(request.Verification)
		}
		runner := agent.Runner{
			Model: e.Model,
			Tools: runTools,
			Now:   e.Now,
		}
		result, err = runner.Run(ctx, agent.RunOptions{
			Task:            request.Task,
			System:          system,
			InitialMessages: initialMessages,
			MaxSteps:        request.MaxSteps,
			OnEvent:         emit,
			CompletionCheck: check,
		})
	}
	outcome := Outcome{Worktree: isolated, StatePath: record.StatePath, ThreadID: request.ThreadID, Result: result, Events: events}
	session := journal.Session{
		Version:         2,
		Repository:      isolated.Repository,
		WorktreePath:    isolated.Path,
		Provider:        request.Provider,
		Model:           request.Model,
		BaseURL:         request.BaseURL,
		Task:            request.Task,
		MaxSteps:        request.MaxSteps,
		Verification:    request.Verification,
		ThreadID:        request.ThreadID,
		Mode:            request.Mode.String(),
		Messages:        result.Messages,
		ParentStatePath: parentStatePath,
	}
	if sessionErr := runJournal.SaveSession(session); sessionErr != nil && journalErr == nil {
		journalErr = sessionErr
	}
	if journalErr == nil {
		if threadErr := e.saveThread(stateDir, session, record.StatePath); threadErr != nil {
			journalErr = threadErr
		}
	}
	status := "completed"
	if err != nil {
		status = "failed"
	}
	if finishErr := runJournal.Finish(status, result.FinalText, e.now()); finishErr != nil && journalErr == nil {
		journalErr = finishErr
	}
	if journalErr != nil {
		if err != nil {
			return outcome, fmt.Errorf("agent run failed: %v; write run journal: %w", err, journalErr)
		}
		return outcome, fmt.Errorf("write run journal: %w", journalErr)
	}
	if err != nil {
		return outcome, err
	}
	return outcome, nil
}

func (e Executor) saveThread(stateDir string, session journal.Session, statePath string) error {
	now := e.now()
	thread := journal.Thread{
		Version:       1,
		ID:            session.ThreadID,
		Repository:    session.Repository,
		WorktreePath:  session.WorktreePath,
		Provider:      session.Provider,
		Model:         session.Model,
		BaseURL:       session.BaseURL,
		Task:          session.Task,
		MaxSteps:      session.MaxSteps,
		Verification:  session.Verification,
		HeadStatePath: statePath,
		TurnCount:     1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if previous, err := journal.LoadThread(stateDir, session.Repository, session.ThreadID); err == nil {
		thread.CreatedAt = previous.CreatedAt
		thread.TurnCount = previous.TurnCount + 1
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return journal.SaveThread(stateDir, thread)
}

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
		call := agent.ToolCall{ID: fmt.Sprintf("harness-verify-%d", index+1), Name: "run_command"}
		emit(agent.Event{Kind: agent.EventToolCalled, At: now(), Step: 1, ToolCall: &call})
		result, err := tools.RunAllowedCommand(ctx, root, tools.CommandPolicy{Allowed: verification}, argv)
		finished := agent.Event{Kind: agent.EventToolFinished, At: now(), Step: 1, ToolCall: &call}
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
	var err error
	switch name {
	case "git_status":
		_, err = (tools.GitStatus{Root: root}).Execute(ctx, json.RawMessage(`{}`))
	case "git_diff":
		_, err = (tools.GitDiff{Root: root}).Execute(ctx, json.RawMessage(`{}`))
	default:
		err = fmt.Errorf("unknown harness inspection %q", name)
	}
	finished := agent.Event{Kind: agent.EventToolFinished, At: now(), Step: 1, ToolCall: &call}
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

const maxProjectInstructionsBytes = 64 * 1024

func loadProjectInstructions(repository string) (string, error) {
	path := filepath.Join(repository, "AGENTS.md")
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("stat AGENTS.md: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("AGENTS.md must be a regular file")
	}
	if info.Size() > maxProjectInstructionsBytes {
		return "", fmt.Errorf("AGENTS.md exceeds the %d-byte limit", maxProjectInstructionsBytes)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read AGENTS.md: %w", err)
	}
	return string(contents), nil
}

func joinInstructions(project, request string) string {
	project = strings.TrimSpace(project)
	request = strings.TrimSpace(request)
	if project == "" {
		return request
	}
	if request == "" {
		return project
	}
	return project + "\n\nAdditional run instructions:\n" + request
}

func systemPrompt(additional string, verification [][]string) string {
	prompt := `You are Gator, a careful coding agent working in an isolated Git worktree.

Treat the user task as an implementation request, not a request for advice. Explore before changing code. For feature work, make the smallest coherent multi-file change, add or update focused tests, and use apply_patch rather than describing a patch in prose. Do not use paths outside the workspace. Inspect git_status and git_diff before completion. Report what changed, which verification commands passed or failed, and any remaining uncertainty. Never claim a command passed unless its tool result shows exit code 0.`
	if len(verification) > 0 {
		prompt += "\n\nThe following verification commands are required before completion:\n"
		for _, command := range verification {
			prompt += "- " + strings.Join(command, " ") + "\n"
		}
	}
	if strings.TrimSpace(additional) != "" {
		prompt += "\n\nRepository instructions:\n" + strings.TrimSpace(additional)
	}
	return prompt
}

func planSystemPrompt(additional string) string {
	prompt := `You are Gator in enforced Plan mode inside an isolated Git worktree.
Explore the repository and produce a concise implementation plan. You can inspect files, search, and inspect Git state, but you cannot edit files or run commands. Do not claim that you changed or verified anything. Identify the relevant files, intended changes, tests to add or run after execution, and any uncertainty that needs developer input.`
	if strings.TrimSpace(additional) != "" {
		prompt += "\n\nRepository instructions:\n" + strings.TrimSpace(additional)
	}
	return prompt
}

func completionCheck(verification [][]string) func([]agent.Message) error {
	return func(messages []agent.Message) error {
		start := 0
		for index, message := range messages {
			if message.Role == agent.RoleUser {
				start = index
			}
		}
		seen := make(map[string]bool)
		passed := make(map[string]bool)
		for _, message := range messages[start:] {
			if message.Role != agent.RoleTool {
				continue
			}
			seen[message.ToolName] = true
			if message.ToolName == "run_command" {
				argv, exitCode, ok := commandResult(message.Content)
				if ok && exitCode == 0 {
					passed[commandKey(argv)] = true
				}
			}
		}
		if !seen["git_status"] {
			return errors.New("inspect the final worktree with git_status")
		}
		if !seen["git_diff"] {
			return errors.New("inspect the final patch with git_diff")
		}
		for _, command := range verification {
			if !passed[commandKey(command)] {
				return fmt.Errorf("run the required verification successfully: %s", strings.Join(command, " "))
			}
		}
		return nil
	}
}

func commandResult(content string) ([]string, int, bool) {
	var payload struct {
		OK     bool `json:"ok"`
		Result struct {
			Argv     []string `json:"argv"`
			ExitCode int      `json:"exit_code"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(content), &payload); err != nil || !payload.OK || len(payload.Result.Argv) == 0 {
		return nil, 0, false
	}
	return payload.Result.Argv, payload.Result.ExitCode, true
}

func validateVerification(verification [][]string) error {
	for _, command := range verification {
		if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
			return errors.New("verification commands must contain an argv program")
		}
	}
	return nil
}

func newID(now time.Time) (string, error) {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("generate run id: %w", err)
	}
	return "run-" + now.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(suffix[:]), nil
}

func (e Executor) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func commandKey(argv []string) string {
	return strings.Join(argv, "\x00")
}
