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
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/worktree"
)

// Request configures a single coding task. Every listed verification argv is
// both exposed to the agent and required to succeed before completion.
type Request struct {
	RepositoryPath string
	Task           string
	Model          string
	RunID          string
	MaxSteps       int
	Verification   [][]string
	System         string
	StateDir       string
	OnEvent        agent.EventSink
}

// Outcome preserves the reviewable artifacts of a completed or failed run.
type Outcome struct {
	Worktree  worktree.Worktree
	StatePath string
	Result    agent.Result
	Events    []agent.Event
}

// Executor combines the provider-independent loop with an isolated worktree.
type Executor struct {
	Model    agent.Model
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
	request.Model = previous.Model
	request.Verification = previous.Verification
	if request.MaxSteps == 0 {
		request.MaxSteps = previous.MaxSteps
	}
	if request.MaxSteps == 0 {
		request.MaxSteps = defaultResumeMaxSteps
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
	if e.Model == nil {
		return errors.New("agent model is required")
	}
	if strings.TrimSpace(request.Task) == "" {
		return errors.New("run task is required")
	}
	if err := validateVerification(request.Verification); err != nil {
		return err
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
	runner := agent.Runner{
		Model: e.Model,
		Tools: tools.Default(isolated.Root, tools.CommandPolicy{
			Allowed: request.Verification,
		}),
		Now: e.Now,
	}
	result, err := runner.Run(ctx, agent.RunOptions{
		Task:            request.Task,
		System:          systemPrompt(joinInstructions(projectInstructions, request.System), request.Verification),
		InitialMessages: initialMessages,
		MaxSteps:        request.MaxSteps,
		OnEvent:         emit,
		CompletionCheck: completionCheck(request.Verification),
	})
	outcome := Outcome{Worktree: isolated, StatePath: record.StatePath, Result: result, Events: events}
	if sessionErr := runJournal.SaveSession(journal.Session{
		Version:         1,
		Repository:      isolated.Repository,
		WorktreePath:    isolated.Path,
		Model:           request.Model,
		Task:            request.Task,
		MaxSteps:        request.MaxSteps,
		Verification:    request.Verification,
		Messages:        result.Messages,
		ParentStatePath: parentStatePath,
	}); sessionErr != nil && journalErr == nil {
		journalErr = sessionErr
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
