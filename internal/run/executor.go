// Package run orchestrates one isolated native coding-agent execution.
package run

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/patch"
	"github.com/gongahkia/gator/internal/worktree"
)

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
	if request.BaseCommit == "" {
		request.BaseCommit = isolated.BaseCommit
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
	if request.BaseCommit == "" {
		request.BaseCommit = previous.BaseCommit
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
	history = append(history, agent.Message{Role: agent.RoleUser, Content: "Continue the original task with this developer instruction:\n" + continuation, Images: request.Images, Attachments: request.Attachments})
	return e.execute(ctx, isolated, request, history, statePath)
}

// Fork restores one retained turn into a new worktree and continues from that
// exact conversation state. The source worktree remains untouched, so a fork
// is safe for trying an alternate approach to an earlier turn.
func (e Executor) Fork(ctx context.Context, previous journal.Session, statePath, continuation string, request Request) (Outcome, error) {
	if strings.TrimSpace(continuation) == "" {
		return Outcome{}, errors.New("fork task is required")
	}
	if strings.TrimSpace(previous.BaseCommit) == "" {
		return Outcome{}, errors.New("retained run has no base commit and cannot be forked")
	}
	snapshot, err := journal.LoadSnapshot(statePath)
	if err != nil {
		return Outcome{}, fmt.Errorf("load retained fork snapshot: %w", err)
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
	generated, err := newID(e.now())
	if err != nil {
		return Outcome{}, err
	}
	request.RunID = generated
	request.ThreadID = generated
	request.BaseCommit = previous.BaseCommit
	request.ForkedFrom = statePath
	if err := e.validateRequest(request); err != nil {
		return Outcome{}, err
	}
	isolation, err := worktree.CreateAtRevision(ctx, previous.Repository, generated, previous.BaseCommit)
	if err != nil {
		return Outcome{}, err
	}
	if err := patch.ApplySnapshot(ctx, isolation.Path, snapshot); err != nil {
		return Outcome{Worktree: isolation}, fmt.Errorf("restore retained fork snapshot: %w", err)
	}
	history := append([]agent.Message(nil), previous.Messages...)
	history = append(history, agent.Message{Role: agent.RoleUser, Content: "Continue the original task from this fork with this developer instruction:\n" + continuation, Images: request.Images, Attachments: request.Attachments})
	return e.execute(ctx, isolation, request, history, "")
}

// Clone duplicates a retained branch into a new worktree. Its restoration and
// history semantics intentionally match Fork; the distinction is the user
// workflow: clone starts from the current branch, while fork starts from a
// turn selected in the session tree.
func (e Executor) Clone(ctx context.Context, previous journal.Session, statePath, continuation string, request Request) (Outcome, error) {
	return e.Fork(ctx, previous, statePath, continuation, request)
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
	if strings.TrimSpace(request.Provider) == "" {
		return errors.New("run provider is required")
	}
	if request.Mode != ExecuteMode && request.Mode != PlanMode {
		return fmt.Errorf("unsupported run mode %d", request.Mode)
	}
	return nil
}
