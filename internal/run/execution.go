package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/instructions"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/patch"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/worktree"
)

func (e Executor) execute(ctx context.Context, isolated worktree.Worktree, request Request, initialMessages []agent.Message, parentStatePath string) (Outcome, error) {
	projectInstructionSet, err := instructions.Load(isolated.Repository, request.Scopes)
	if err != nil {
		return Outcome{Worktree: isolated}, err
	}
	extensions, err := e.Extensions.Load(isolated.Repository)
	if err != nil {
		return Outcome{Worktree: isolated}, fmt.Errorf("load extensions: %w", err)
	}
	extensionInstructions, err := extensions.Instructions()
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
	remembered := tools.NewCommandMemory(request.AllowedCommands)
	executionPolicy := e.Sandbox.Normalize()
	if err := executionPolicy.Validate(); err != nil {
		return Outcome{Worktree: isolated, StatePath: record.StatePath, ThreadID: request.ThreadID}, fmt.Errorf("validate execution policy: %w", err)
	}
	runTools := tools.Default(isolated.Root, tools.CommandPolicy{
		Allowed:    request.Verification,
		Remembered: remembered,
		Approve:    request.Approve,
		OnEvent:    emit,
		Sandbox:    executionPolicy,
	})
	if request.Mode == ExecuteMode {
		runTools = append(runTools, extensions.Tools(isolated.Root)...)
	}
	system := systemPrompt(joinInstructions(joinInstructions(projectInstructionSet.Content, extensionInstructions), request.System), request.Verification)
	var check func([]agent.Message) error
	if request.Mode == PlanMode {
		runTools = tools.ReadOnly(isolated.Root)
		system = planSystemPrompt(joinInstructions(projectInstructionSet.Content, request.System))
	} else {
		check = completionCheck(request.Verification)
	}
	originalMessageCount := len(initialMessages)
	compactionContext, cancelCompaction := context.WithTimeout(ctx, 45*time.Second)
	compactedMessages, _, compacted, compactErr := compactMessages(compactionContext, e.Model, initialMessages, request.ForceCompaction)
	cancelCompaction()
	if compactErr != nil {
		finishErr := runJournal.Finish("failed", "", e.now())
		if finishErr != nil {
			return Outcome{Worktree: isolated, StatePath: record.StatePath, ThreadID: request.ThreadID}, fmt.Errorf("compact retained context: %v; finish failed run journal: %w", compactErr, finishErr)
		}
		return Outcome{Worktree: isolated, StatePath: record.StatePath, ThreadID: request.ThreadID}, fmt.Errorf("compact retained context: %w", compactErr)
	}
	if compacted {
		initialMessages = compactedMessages
		emit(agent.Event{Kind: agent.EventContextCompacted, At: e.now(), Text: fmt.Sprintf("Compacted %d earlier messages before this run.", originalMessageCount-len(compactedMessages)+1)})
	}
	runner := agent.Runner{
		Model: e.Model,
		Tools: runTools,
		Now:   e.Now,
	}
	result, err = runner.Run(ctx, agent.RunOptions{
		Task:            request.Task,
		Images:          request.Images,
		Attachments:     request.Attachments,
		System:          system,
		InitialMessages: initialMessages,
		MaxSteps:        request.MaxSteps,
		OnEvent:         emit,
		Steering:        request.Steering,
		CompletionCheck: check,
	})
	outcome := Outcome{Worktree: isolated, StatePath: record.StatePath, ThreadID: request.ThreadID, Result: result, Events: events}
	snapshotContext, cancelSnapshot := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	snapshot, snapshotErr := patch.Export(snapshotContext, isolated.Path, request.BaseCommit)
	cancelSnapshot()
	if snapshotErr != nil && journalErr == nil {
		journalErr = fmt.Errorf("create fork snapshot: %w", snapshotErr)
	}
	if snapshotErr == nil && journalErr == nil {
		if snapshotErr := runJournal.SaveSnapshot(snapshot); snapshotErr != nil {
			journalErr = snapshotErr
		}
	}
	session := journal.Session{
		Version:             2,
		Repository:          isolated.Repository,
		WorktreePath:        isolated.Path,
		BaseCommit:          request.BaseCommit,
		Provider:            request.Provider,
		Model:               request.Model,
		BaseURL:             request.BaseURL,
		Task:                request.Task,
		MaxSteps:            request.MaxSteps,
		Verification:        request.Verification,
		Scopes:              request.Scopes,
		ThreadID:            request.ThreadID,
		Mode:                request.Mode.String(),
		Messages:            result.Messages,
		ParentStatePath:     parentStatePath,
		ForkedFromStatePath: request.ForkedFrom,
		AllowedCommands:     remembered.Snapshot(),
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
		ForkedFrom:    session.ForkedFromStatePath,
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
