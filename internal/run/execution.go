package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/hooks"
	"github.com/gongahkia/gator/internal/instructions"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/lsp"
	"github.com/gongahkia/gator/internal/mcp"
	"github.com/gongahkia/gator/internal/patch"
	"github.com/gongahkia/gator/internal/terminal"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/worktree"
)

func (e Executor) execute(ctx context.Context, isolated worktree.Worktree, request Request, initialMessages []agent.Message, parentStatePath string) (Outcome, error) {
	projectInstructionSet, err := instructions.LoadWithProfile(isolated.Repository, request.Scopes, request.Profile)
	if err != nil {
		return Outcome{Worktree: isolated}, err
	}
	hookEngine, err := hooks.Load(isolated.Path, isolated.Repository, e.hookTrust(isolated.Repository))
	if err != nil {
		return Outcome{Worktree: isolated}, fmt.Errorf("load project hooks: %w", err)
	}
	mcpSet, err := mcp.LoadWithCredentials(ctx, isolated.Path, e.mcpTrust(isolated.Repository), e.MCPCredentials)
	if err != nil {
		return Outcome{Worktree: isolated}, fmt.Errorf("load project MCP servers: %w", err)
	}
	defer mcpSet.Close()
	lspSet, err := lsp.Load(isolated.Path, e.lspTrust(isolated.Repository))
	if err != nil {
		return Outcome{Worktree: isolated}, fmt.Errorf("load project LSP servers: %w", err)
	}
	lspManager := lspSet.NewManager()
	defer lspManager.Close()
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
	var eventMu sync.Mutex
	var journalErr error
	emit := func(event agent.Event) {
		eventMu.Lock()
		defer eventMu.Unlock()
		events = append(events, event)
		if journalErr == nil {
			journalErr = runJournal.Append(event)
		}
		if request.OnEvent != nil {
			request.OnEvent(event)
		}
	}
	hookEngine.Emit = func(status hooks.Status) {
		decision := "allowed"
		if !status.Allowed {
			decision = "denied"
		}
		message := status.Hook + " " + string(status.Event) + " " + decision
		emit(agent.Event{Kind: agent.EventHook, At: e.now(), Text: message})
	}
	if hookEngine.Configured() && !hookEngine.Trusted() {
		emit(agent.Event{Kind: agent.EventHook, At: e.now(), Text: "project hooks are disabled because their bundle hash is not explicitly trusted"})
	}
	if mcpSet.Configured() && !mcpSet.Trusted() {
		emit(agent.Event{Kind: agent.EventHook, At: e.now(), Text: "project MCP servers are disabled because their bundle hash is not explicitly trusted"})
	}
	if lspSet.Configured() && !lspSet.Trusted() {
		emit(agent.Event{Kind: agent.EventHook, At: e.now(), Text: "project LSP servers are disabled because their bundle hash is not explicitly trusted"})
	}
	var result agent.Result
	remembered := tools.NewCommandMemory(request.AllowedCommands)
	executionPolicy := e.Sandbox.Normalize()
	if err := executionPolicy.Validate(); err != nil {
		return Outcome{Worktree: isolated, StatePath: record.StatePath, ThreadID: request.ThreadID}, fmt.Errorf("validate execution policy: %w", err)
	}
	commandPolicy := tools.CommandPolicy{
		Allowed:    request.Verification,
		Remembered: remembered,
		Approve:    request.Approve,
		OnEvent:    emit,
		Sandbox:    executionPolicy,
		BeforeCommand: func(ctx context.Context, argv []string, verification bool) error {
			if !verification || request.Mode != ExecuteMode {
				return nil
			}
			return hookEngine.Run(ctx, hooks.Verification, "run_command", map[string]any{"argv": argv})
		},
	}
	runTools := tools.Default(isolated.Root, commandPolicy)
	terminalManager := terminal.New(terminal.Config{
		Root:   isolated.Root,
		Policy: executionPolicy,
		Now:    e.Now,
		OnExit: func(task terminal.Task) {
			emit(agent.Event{Kind: agent.EventTerminal, At: e.now(), Text: terminalStatusText(task)})
		},
		OnDeveloperInput: func(task terminal.Task, input terminal.DeveloperInput) {
			digest := input.SHA256
			if len(digest) > 8 {
				digest = digest[:8]
			}
			emit(agent.Event{Kind: agent.EventTerminal, At: e.now(), Text: fmt.Sprintf("%s developer input (%d bytes sha256:%s)", task.ID, input.Bytes, digest)})
		},
	})
	if request.OnTerminalAttachment != nil {
		request.OnTerminalAttachment(terminalManager.Attachment())
		defer request.OnTerminalAttachment(nil)
	}
	defer terminalManager.Close()
	roleSet, err := instructions.LoadRoles(isolated.Repository)
	if err != nil {
		return Outcome{Worktree: isolated}, fmt.Errorf("load project agent roles: %w", err)
	}
	readonlyScout := newReadOnlyScoutTool(e.Model, isolated.Root, projectInstructionSet.Content, roleSet, request.MaxSteps, e.Now, emit)
	if request.Mode == ExecuteMode {
		runTools = append(runTools, extensions.Tools(isolated.Root)...)
		runTools = append(runTools, lspManager.Tools(func(ctx context.Context, server, operation, path string) error {
			argv := []string{"lsp", server, operation, path}
			if remembered.Allows(argv) {
				return nil
			}
			toolName := "lsp_" + server + "_" + operation
			emit(agent.Event{Kind: agent.EventCommandApprovalRequested, At: e.now(), ToolCall: &agent.ToolCall{Name: toolName}, Text: strings.Join(argv, " "), Argv: argv})
			if request.Approve == nil {
				emit(agent.Event{Kind: agent.EventCommandApprovalResolved, At: e.now(), ToolCall: &agent.ToolCall{Name: toolName}, Text: tools.CommandDeny.String(), Argv: argv})
				return fmt.Errorf("LSP %s requires developer approval", operation)
			}
			decision, err := request.Approve(ctx, argv)
			if err != nil {
				emit(agent.Event{Kind: agent.EventCommandApprovalResolved, At: e.now(), ToolCall: &agent.ToolCall{Name: toolName}, Text: tools.CommandDeny.String(), Argv: argv})
				return err
			}
			emit(agent.Event{Kind: agent.EventCommandApprovalResolved, At: e.now(), ToolCall: &agent.ToolCall{Name: toolName}, Text: decision.String(), Argv: argv})
			if decision == tools.CommandAllowAlways {
				remembered.Remember(argv)
			}
			if decision != tools.CommandAllowOnce && decision != tools.CommandAllowAlways {
				return fmt.Errorf("LSP %s from %s denied by developer", operation, server)
			}
			return nil
		})...)
		runTools = append(runTools, mcpSet.Tools(func(ctx context.Context, server, tool string) error {
			argv := []string{"mcp", server, tool}
			if remembered.Allows(argv) {
				return nil
			}
			emit(agent.Event{Kind: agent.EventCommandApprovalRequested, At: e.now(), ToolCall: &agent.ToolCall{Name: "mcp_" + server + "_" + tool}, Text: strings.Join(argv, " "), Argv: argv})
			if request.Approve == nil {
				emit(agent.Event{Kind: agent.EventCommandApprovalResolved, At: e.now(), ToolCall: &agent.ToolCall{Name: "mcp_" + server + "_" + tool}, Text: tools.CommandDeny.String(), Argv: argv})
				return errors.New("MCP tool requires developer approval")
			}
			decision, err := request.Approve(ctx, argv)
			if err != nil {
				emit(agent.Event{Kind: agent.EventCommandApprovalResolved, At: e.now(), ToolCall: &agent.ToolCall{Name: "mcp_" + server + "_" + tool}, Text: tools.CommandDeny.String(), Argv: argv})
				return err
			}
			emit(agent.Event{Kind: agent.EventCommandApprovalResolved, At: e.now(), ToolCall: &agent.ToolCall{Name: "mcp_" + server + "_" + tool}, Text: decision.String(), Argv: argv})
			if decision == tools.CommandAllowAlways {
				remembered.Remember(argv)
			}
			if decision != tools.CommandAllowOnce && decision != tools.CommandAllowAlways {
				return fmt.Errorf("MCP tool %s/%s denied by developer", server, tool)
			}
			return nil
		})...)
		runTools = append(runTools, tools.TerminalTools(terminalManager, commandPolicy)...)
		runTools = append(runTools, tools.HTTPTools(commandPolicy, e.HTTP)...)
		runTools = append(runTools, readonlyScout)
		if !request.DisableWriterDelegation {
			runTools = append(runTools, newWriterTools(e, isolated, request, remembered, runJournal, roleSet, e.Now, emit)...)
		}
	}
	system := systemPrompt(joinInstructions(joinInstructions(projectInstructionSet.Content, extensionInstructions), request.System), request.Verification)
	var check func([]agent.Message) error
	if request.Mode == PlanMode {
		runTools = append(tools.ReadOnly(isolated.Root), readonlyScout)
		system = planSystemPrompt(joinInstructions(projectInstructionSet.Content, request.System))
	} else {
		check = completionCheck(request.Verification)
	}
	originalMessageCount := len(initialMessages)
	compactionContext, cancelCompaction := context.WithTimeout(ctx, 45*time.Second)
	lifecycle := func(event string, payload map[string]any) error {
		if request.Mode != ExecuteMode {
			return nil
		}
		return hookEngine.Run(compactionContext, hooks.Event(event), "", payload)
	}
	compactedMessages, _, compacted, compactErr := compactMessagesWithLifecycle(compactionContext, e.Model, initialMessages, request.ForceCompaction, lifecycle)
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
	var beforeTool func(context.Context, agent.ToolCall) error
	var afterTool func(context.Context, agent.ToolCall, agent.ToolResult, error) error
	if request.Mode == ExecuteMode {
		beforeTool = func(ctx context.Context, call agent.ToolCall) error {
			return hookEngine.Run(ctx, hooks.PreToolUse, call.Name, map[string]any{"tool_call_id": call.ID, "arguments": call.Arguments})
		}
		afterTool = func(ctx context.Context, call agent.ToolCall, _ agent.ToolResult, toolErr error) error {
			return hookEngine.Run(ctx, hooks.PostToolUse, call.Name, map[string]any{"tool_call_id": call.ID, "success": toolErr == nil})
		}
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
		BeforeTool:      beforeTool,
		AfterTool:       afterTool,
	})
	terminalManager.Close()
	eventMu.Lock()
	eventSnapshot := append([]agent.Event(nil), events...)
	eventMu.Unlock()
	outcome := Outcome{Worktree: isolated, StatePath: record.StatePath, ThreadID: request.ThreadID, Result: result, Events: eventSnapshot}
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
		Profile:             request.Profile,
		ThreadID:            request.ThreadID,
		Mode:                request.Mode.String(),
		HooksHash:           hookEngine.Hash(),
		LSPHash:             lspSet.Hash(),
		MCPHash:             mcpSet.Hash(),
		Messages:            result.Messages,
		ParentStatePath:     parentStatePath,
		ForkedFromStatePath: request.ForkedFrom,
		AllowedCommands:     remembered.Snapshot(),
	}
	if request.Mode == ExecuteMode {
		if hookErr := hookEngine.Run(ctx, hooks.SessionSave, "", map[string]any{"run_id": request.RunID, "thread_id": request.ThreadID}); hookErr != nil && journalErr == nil {
			journalErr = hookErr
		}
	}
	if journalErr == nil {
		if sessionErr := runJournal.SaveSession(session); sessionErr != nil {
			journalErr = sessionErr
		}
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

func terminalStatusText(task terminal.Task) string {
	if task.ExitCode == nil {
		return task.ID + " exited"
	}
	if task.Error != "" {
		return fmt.Sprintf("%s exited with code %d: %s", task.ID, *task.ExitCode, task.Error)
	}
	return fmt.Sprintf("%s exited with code %d", task.ID, *task.ExitCode)
}

func (e Executor) hookTrust(repository string) string {
	for _, trust := range e.HookTrusts {
		if trust.Repository == repository {
			return trust.Hash
		}
	}
	return ""
}

func (e Executor) mcpTrust(repository string) string {
	for _, trust := range e.MCPTrusts {
		if trust.Repository == repository {
			return trust.Hash
		}
	}
	return ""
}

func (e Executor) lspTrust(repository string) string {
	for _, trust := range e.LSPTrusts {
		if trust.Repository == repository {
			return trust.Hash
		}
	}
	return ""
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
