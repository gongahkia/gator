package tui

import (
	"context"
	"crypto/sha256"
	"errors"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/diffview"
	"github.com/gongahkia/gator/internal/journal"
	modelprovider "github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/review"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/terminal"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workspace"
)

func (m Model) startRun() (tea.Model, tea.Cmd) {
	task := strings.TrimSpace(m.task.Value())
	if task == "" {
		return m.startFailure("Describe a task before starting a run.")
	}
	if m.delegateRuntime != "" {
		return m.startDelegatedRun(task)
	}
	if m.config.NewExecutor == nil {
		return m.startFailure("No model provider is configured for this Gator build.")
	}
	references, err := resolveContextReferences(task, m.config.RepositoryPath)
	if err != nil {
		return m.startFailure(err.Error())
	}
	images, attachments, err := promptAttachments(m.config.RepositoryPath, references)
	if err != nil {
		return m.startFailure(err.Error())
	}
	if m.attachmentConfirmed && !sameAttachmentPreviews(m.attachmentPreview, attachmentPreviews(images, attachments)) {
		m.attachmentConfirmed = false
		m.attachmentPreview = attachmentPreviews(images, attachments)
		m.screen = attachmentConfirmScreen
		m.notice = notice{text: "Attachment contents changed after preview. Review and confirm the current bytes before sending.", kind: noticeInfo}
		return m, nil
	}
	var verification [][]string
	if m.runMode == gatorrun.ExecuteMode {
		var err error
		verification, err = parseVerification(m.verification.Value())
		if err != nil {
			return m.startFailure(err.Error())
		}
	}
	modelName := strings.TrimSpace(m.model.Value())
	providerName := strings.TrimSpace(m.provider.Value())
	if providerName == "" {
		return m.startFailure("Choose a provider before starting a run.")
	}
	resolvedProvider, resolvedModel, customProvider, providerErr := m.resolveProviderAndModel(providerName, modelName)
	if providerErr != nil {
		return m.startFailure(providerErr.Error())
	}
	if hasPDFAttachment(attachments) && (customProvider || !modelprovider.SupportsPDFAttachments(modelprovider.Provider(resolvedProvider))) {
		return m.startFailure("PDF attachments require the OpenAI Responses, Anthropic Messages, or Gemini provider.")
	}
	providerName = resolvedProvider
	modelName = resolvedModel
	m.refreshPreflight()
	if len(m.preflight) > 0 {
		return m.startFailure("Resolve configuration before starting:\n" + strings.Join(m.preflight, "\n"))
	}
	var executor gatorrun.Executor
	if m.resumeStatePath == "" {
		var executorErr error
		executor, executorErr = m.config.NewExecutor(providerName, modelName, m.config.BaseURL)
		if executorErr != nil {
			return m.startFailure(executorErr.Error())
		}
	}

	if len(images) > 0 || len(attachments) > 0 {
		if !m.attachmentConfirmed {
			m.attachmentPreview = attachmentPreviews(images, attachments)
			m.screen = attachmentConfirmScreen
			m.notice = notice{text: "Review attachment transmission before starting.", kind: noticeInfo}
			return m, nil
		}
		m.attachmentConfirmed = false
		m.attachmentPreview = nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	stream := &executionStream{
		events:    make(chan agent.Event, 32),
		done:      make(chan executionDone, 1),
		cancel:    cancel,
		steering:  make(chan string, maxQueuedInputs),
		approvals: make(chan commandApprovalRequest, 1),
		terminals: make(chan terminal.Attachment, 2),
	}
	m.execution = stream
	m.events = nil
	m.appendChat(chatEntry{author: chatUser, text: task})
	m.task.Reset()
	m.task.Placeholder = "Enter steers this run · Tab queues the next turn..."
	m.outcome = nil
	m.runErr = nil
	m.diff = ""
	m.focusedDiff = ""
	m.focusedDiffInfo = diffview.Focused{}
	m.diffMode = focusedDiffDisplay
	m.diffOffset = 0
	m.diffErr = nil
	m.diffTruncated = false
	m.diffStats = diffStats{}
	m.reviewSnapshot = review.Snapshot{}
	m.reviewLoaded = false
	m.reviewScope = review.All
	m.reviewPane = reviewFilesPane
	m.reviewFileIndex, m.reviewHunkIndex, m.reviewLineIndex = 0, 0, 0
	m.reviewRangeFrom = -1
	m.reviewMutation = nil
	m.reviewRequestOn = false
	m.lastRunCancelled = false
	m.screen = runningScreen
	m.focus = taskField
	_ = m.focusField()
	if m.runMode == gatorrun.PlanMode {
		m.notice = notice{text: "Creating an isolated worktree for read-only planning...", kind: noticeInfo}
	} else {
		m.notice = notice{text: "Creating an isolated worktree...", kind: noticeInfo}
	}

	request := gatorrun.Request{
		RepositoryPath:   m.config.RepositoryPath,
		Task:             taskWithContextReferences(task, references),
		Provider:         providerName,
		Model:            modelName,
		BaseURL:          m.config.BaseURL,
		MaxSteps:         m.effort.maxSteps(m.config.MaxSteps),
		Verification:     verification,
		Scopes:           contextReferencePaths(references),
		StateDir:         m.config.StateDir,
		ThreadID:         m.threadID,
		Images:           images,
		Attachments:      attachments,
		Mode:             m.runMode,
		ForceCompaction:  m.forceCompaction,
		TerminalRegistry: m.terminalRegistry,
		LSPRegistry:      m.lspRegistry,
		OnEvent: func(event agent.Event) {
			select {
			case stream.events <- event:
			case <-ctx.Done():
			}
		},
		Steering: stream.steering,
		Approve:  stream.approve,
		OnTerminalAttachment: func(attachment terminal.Attachment) {
			select {
			case stream.terminals <- attachment:
			case <-ctx.Done():
			}
		},
	}

	if m.forkStatePath != "" {
		previous, loadErr := journal.LoadSession(m.forkStatePath)
		if loadErr != nil {
			cancel()
			m.execution = nil
			m.screen = composeScreen
			return m.startFailure("Load retained fork: " + loadErr.Error())
		}
		if _, snapshotErr := journal.LoadSnapshot(m.forkStatePath); snapshotErr != nil {
			cancel()
			m.execution = nil
			m.screen = composeScreen
			return m.startFailure("Fork retained run: " + snapshotErr.Error())
		}
		retainedProvider, retainedModel, retainedCustom, providerErr := m.resolveProviderAndModel(previous.Provider, previous.Model)
		if providerErr != nil {
			cancel()
			m.execution = nil
			m.screen = composeScreen
			return m.startFailure(providerErr.Error())
		}
		if hasPDFAttachment(attachments) && (retainedCustom || !modelprovider.SupportsPDFAttachments(modelprovider.Provider(retainedProvider))) {
			cancel()
			m.execution = nil
			m.screen = composeScreen
			return m.startFailure("PDF attachments require the OpenAI Responses, Anthropic Messages, or Gemini provider.")
		}
		providerName = retainedProvider
		modelName = retainedModel
		request.Model = modelName
		request.Provider = providerName
		request.BaseURL = previous.BaseURL
		request.Verification = previous.Verification
		var executorErr error
		executor, executorErr = m.config.NewExecutor(providerName, modelName, previous.BaseURL)
		if executorErr != nil {
			cancel()
			m.execution = nil
			m.screen = composeScreen
			return m.startFailure(executorErr.Error())
		}
		m.beginRunActivity(request.Verification)
		m.forceCompaction = false
		go executeFork(ctx, stream, executor, previous, m.forkStatePath, task, request)
	} else if m.resumeStatePath != "" {
		previous, loadErr := journal.LoadSession(m.resumeStatePath)
		if loadErr != nil {
			cancel()
			m.execution = nil
			m.screen = composeScreen
			return m.startFailure("Load retained run: " + loadErr.Error())
		}
		if strings.TrimSpace(previous.Provider) == "" {
			cancel()
			m.execution = nil
			m.screen = composeScreen
			return m.startFailure("The retained run does not record a provider and cannot be continued safely.")
		}
		retainedProvider, retainedModel, retainedCustom, providerErr := m.resolveProviderAndModel(previous.Provider, previous.Model)
		if providerErr != nil {
			cancel()
			m.execution = nil
			m.screen = composeScreen
			return m.startFailure(providerErr.Error())
		}
		if hasPDFAttachment(attachments) && (retainedCustom || !modelprovider.SupportsPDFAttachments(modelprovider.Provider(retainedProvider))) {
			cancel()
			m.execution = nil
			m.screen = composeScreen
			return m.startFailure("PDF attachments require the OpenAI Responses, Anthropic Messages, or Gemini provider.")
		}
		providerName = retainedProvider
		modelName = retainedModel
		request.Model = modelName
		request.Provider = providerName
		request.BaseURL = previous.BaseURL
		request.Verification = previous.Verification
		var executorErr error
		executor, executorErr = m.config.NewExecutor(providerName, modelName, previous.BaseURL)
		if executorErr != nil {
			cancel()
			m.execution = nil
			m.screen = composeScreen
			return m.startFailure(executorErr.Error())
		}
		m.beginRunActivity(request.Verification)
		m.forceCompaction = false
		go executeResume(ctx, stream, executor, previous, m.resumeStatePath, task, request)
	} else {
		m.beginRunActivity(request.Verification)
		m.forceCompaction = false
		go executeNew(ctx, stream, executor, request)
	}
	return m, waitForExecution(stream)
}

// startDelegatedRun intentionally hands terminal control to the installed
// harness rather than adapting its credential or trying to simulate its
// session. Gator still creates the isolated worktree and runs verification.
func (m Model) startDelegatedRun(task string) (tea.Model, tea.Cmd) {
	if m.resumeStatePath != "" || m.forkStatePath != "" {
		return m.startFailure("A provider-owned harness always starts a new isolated worktree; it cannot continue or fork a native Gator thread.")
	}
	if m.runMode != gatorrun.ExecuteMode {
		return m.startFailure(delegatedRuntimeLabel(m.delegateRuntime) + " supports Execute mode only. Run /execute before sending a task.")
	}
	verification, err := parseVerification(m.verification.Value())
	if err != nil {
		return m.startFailure(err.Error())
	}
	references, err := resolveContextReferences(task, m.config.RepositoryPath)
	if err != nil {
		return m.startFailure(err.Error())
	}
	if m.config.NewDelegateCommand == nil {
		return m.startFailure("This Gator build cannot start a provider-owned harness from the TUI.")
	}
	delegate, err := m.config.NewDelegateCommand(m.delegateRuntime, taskWithContextReferences(task, references), strings.TrimSpace(m.model.Value()), verification, m.config.RepositoryPath)
	if err != nil {
		return m.startFailure(err.Error())
	}
	if delegate.Process == nil {
		return m.startFailure("This Gator build returned an invalid provider-owned harness command.")
	}
	m.appendChat(chatEntry{author: chatUser, text: task})
	m.task.Reset()
	m.task.Placeholder = "Send another delegated task..."
	m.commandOutput = "Running " + delegatedRuntimeLabel(m.delegateRuntime) + " in a fresh isolated worktree. Terminal control returns here when it finishes."
	m.notice = notice{text: "Starting " + delegatedRuntimeLabel(m.delegateRuntime) + " in this terminal...", kind: noticeInfo}
	m.persistDraft()
	runtime := m.delegateRuntime
	return m, tea.ExecProcess(delegate.Process, func(err error) tea.Msg {
		output := ""
		if delegate.Output != nil {
			output = delegate.Output()
		}
		return delegatedRunDoneMsg{runtime: runtime, output: output, err: err}
	})
}

// startFailure leaves the current task editable and records the complete
// failure in the scrollable conversation. The composer notice remains a
// compact status signal, while the transcript is the place to inspect long
// provider and configuration errors.
func (m Model) startFailure(message string) (tea.Model, tea.Cmd) {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "Unable to start the run."
	}
	m.notice = notice{text: message, kind: noticeError}
	m.appendChat(chatEntry{author: chatSystem, text: "Unable to start run: " + message, isError: true})
	return m, nil
}

func attachmentPreviews(images []agent.Image, attachments []agent.Attachment) []attachmentPreview {
	previews := make([]attachmentPreview, 0, len(images)+len(attachments))
	for _, image := range images {
		previews = append(previews, attachmentPreview{name: image.Name, mediaType: image.MediaType, bytes: len(image.Data), digest: sha256.Sum256(image.Data)})
	}
	for _, attachment := range attachments {
		previews = append(previews, attachmentPreview{name: attachment.Name, mediaType: attachment.MediaType, bytes: len(attachment.Data), digest: sha256.Sum256(attachment.Data)})
	}
	return previews
}

func sameAttachmentPreviews(first, second []attachmentPreview) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}

func hasPDFAttachment(attachments []agent.Attachment) bool {
	for _, attachment := range attachments {
		if attachment.MediaType == "application/pdf" {
			return true
		}
	}
	return false
}

func (stream *executionStream) approve(ctx context.Context, argv []string) (tools.CommandDecision, error) {
	if stream == nil {
		return tools.CommandDeny, errors.New("execution stream is not active")
	}
	reply := make(chan tools.CommandDecision, 1)
	select {
	case stream.approvals <- commandApprovalRequest{argv: append([]string(nil), argv...), reply: reply}:
	case <-ctx.Done():
		return tools.CommandDeny, ctx.Err()
	}
	select {
	case decision := <-reply:
		return decision, nil
	case <-ctx.Done():
		return tools.CommandDeny, ctx.Err()
	}
}

func (m *Model) resolvePendingApproval(decision tools.CommandDecision) {
	if m.pendingApproval == nil {
		return
	}
	select {
	case m.pendingApproval.reply <- decision:
	default:
	}
	m.pendingApproval = nil
}

func executeNew(ctx context.Context, stream *executionStream, executor gatorrun.Executor, request gatorrun.Request) {
	outcome, err := executor.Execute(ctx, request)
	close(stream.events)
	stream.done <- executionDone{outcome: outcome, err: err}
}

func executeResume(ctx context.Context, stream *executionStream, executor gatorrun.Executor, previous journal.Session, statePath, continuation string, request gatorrun.Request) {
	outcome, err := executor.Resume(ctx, previous, statePath, continuation, request)
	close(stream.events)
	stream.done <- executionDone{outcome: outcome, err: err}
}

func executeFork(ctx context.Context, stream *executionStream, executor gatorrun.Executor, previous journal.Session, statePath, instruction string, request gatorrun.Request) {
	outcome, err := executor.Fork(ctx, previous, statePath, instruction, request)
	close(stream.events)
	stream.done <- executionDone{outcome: outcome, err: err}
}

func waitForExecution(stream *executionStream) tea.Cmd {
	if stream == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case event, ok := <-stream.events:
			if ok {
				return agentEventMsg{event: event}
			}
			return executionDoneMsg{done: <-stream.done}
		case request, ok := <-stream.approvals:
			if ok {
				return commandApprovalMsg{argv: request.argv, reply: request.reply}
			}
			return executionDoneMsg{done: <-stream.done}
		case attachment := <-stream.terminals:
			return terminalManagerMsg{attachment: attachment}
		case <-time.After(time.Second):
			return activityTickMsg{}
		}
	}
}

func loadDiff(root workspace.Root) tea.Cmd {
	return func() tea.Msg {
		diff, truncated, err := tools.ReviewDiff(context.Background(), root)
		return diffLoadedMsg{diff: diff, truncated: truncated, err: err}
	}
}

func loadReview(outcome gatorrun.Outcome) tea.Cmd {
	return func() tea.Msg {
		if outcome.Worktree.Path == "" {
			return reviewLoadedMsg{err: errors.New("retained worktree is unavailable")}
		}
		baseCommit := ""
		if outcome.StatePath != "" {
			session, err := journal.LoadSession(outcome.StatePath)
			if err != nil {
				return reviewLoadedMsg{err: err}
			}
			baseCommit = session.BaseCommit
		}
		snapshot, err := review.Load(context.Background(), outcome.Worktree.Path, baseCommit)
		return reviewLoadedMsg{snapshot: snapshot, err: err}
	}
}

func (m *Model) prepareContinuation() (tea.Model, tea.Cmd) {
	if m.outcome == nil || m.outcome.StatePath == "" {
		m.notice = notice{text: "This run has no resume record.", kind: noticeError}
		return *m, nil
	}
	m.screen = composeScreen
	m.task.Reset()
	m.task.Placeholder = "Send a follow-up..."
	m.focus = taskField
	m.refreshPreflight()
	return *m, m.focusField()
}

func (m *Model) beginContinuation(statePath string) (tea.Model, tea.Cmd) {
	session, err := journal.LoadSession(statePath)
	if err != nil {
		m.notice = notice{text: "Load retained run: " + err.Error(), kind: noticeError}
		return *m, nil
	}
	m.delegateRuntime = ""
	m.resumeStatePath = statePath
	m.config.RepositoryPath = session.Repository
	m.threadID = session.ThreadID
	if m.threadID == "" {
		m.threadID = filepath.Base(statePath)
	}
	m.runMode = gatorrun.ExecuteMode
	if session.Mode == gatorrun.PlanMode.String() {
		m.runMode = gatorrun.PlanMode
	}
	m.task.Reset()
	m.queue = nil
	m.task.Placeholder = "Describe the next instruction for this retained worktree..."
	m.chat = nil
	m.chatIndex = 0
	m.appendChat(chatEntry{author: chatSystem, text: "Continuing a retained thread. The worktree and model context are available; earlier terminal events are not replayed here."})
	m.verification.SetValue(formatVerification(session.Verification))
	m.provider.SetValue(session.Provider)
	m.model.SetValue(session.Model)
	m.screen = composeScreen
	m.focus = taskField
	if len(session.AttachmentManifest) > 0 {
		m.notice = notice{text: "Continuing thread " + m.threadID + ". Prior attachment bytes were not retained; re-add @ files to send them again.", kind: noticeInfo}
	} else {
		m.notice = notice{text: "Continuing thread " + m.threadID + " in its retained worktree.", kind: noticeInfo}
	}
	m.refreshPreflight()
	return *m, m.focusField()
}

func (m *Model) beginFork(statePath string) (tea.Model, tea.Cmd) {
	session, err := journal.LoadSession(statePath)
	if err != nil {
		m.notice = notice{text: "Load retained fork: " + err.Error(), kind: noticeError}
		return *m, nil
	}
	if _, err := journal.LoadSnapshot(statePath); err != nil {
		m.notice = notice{text: "Fork retained run: " + err.Error(), kind: noticeError}
		return *m, nil
	}
	m.delegateRuntime = ""
	m.forkStatePath = statePath
	m.resumeStatePath = ""
	m.forceCompaction = false
	m.threadID = ""
	m.config.RepositoryPath = session.Repository
	m.runMode = gatorrun.ExecuteMode
	if session.Mode == gatorrun.PlanMode.String() {
		m.runMode = gatorrun.PlanMode
	}
	m.task.Reset()
	m.queue = nil
	m.task.Placeholder = "Describe the alternate direction for this fork..."
	m.chat = nil
	m.chatIndex = 0
	m.appendChat(chatEntry{author: chatSystem, text: "Forking a retained turn into a new isolated worktree. The original thread remains unchanged."})
	m.verification.SetValue(formatVerification(session.Verification))
	m.provider.SetValue(session.Provider)
	m.model.SetValue(session.Model)
	m.screen = composeScreen
	m.focus = taskField
	m.notice = notice{text: "Forking turn from " + session.ThreadID + ". Describe the alternate direction.", kind: noticeInfo}
	m.refreshPreflight()
	return *m, m.focusField()
}

func (m *Model) returnToComposer() {
	m.screen = composeScreen
	m.vimCommand = ""
	m.quitAfterRun = false
	m.resumeStatePath = ""
	m.forkStatePath = ""
	m.forceCompaction = false
	m.threadID = ""
	m.recentAll = false
	m.delegateRuntime = ""
	m.runMode = gatorrun.ExecuteMode
	m.task.Reset()
	m.queue = nil
	m.task.Placeholder = "Message Gator..."
	m.chat = nil
	m.chatIndex = 0
	m.appendChat(chatEntry{author: chatSystem, text: "New isolated thread. Gator works in a separate Git worktree; use /permissions for the active policy."})
	m.focus = taskField
	_ = m.focusField()
	m.commandOutput = ""
	m.notice = notice{text: "Ready for a new isolated task.", kind: noticeInfo}
	m.refreshPreflight()
}

// persistDraft keeps the editable new-run form recoverable without storing it
// in the repository or exposing it in the event journal. Continuations use a
// private run session instead, so they do not overwrite the new-run draft.
func (m *Model) persistDraft() {
	if m.resumeStatePath != "" || m.forkStatePath != "" || strings.TrimSpace(m.config.StateDir) == "" {
		return
	}
	err := journal.SaveDraft(m.config.StateDir, journal.Draft{
		Repository:   m.config.RepositoryPath,
		Task:         m.task.Value(),
		Verification: m.verification.Value(),
		Provider:     strings.TrimSpace(m.provider.Value()),
		Model:        strings.TrimSpace(m.model.Value()),
	})
	m.draftErr = err
}

// refreshPreflight validates the same provider factory used by Ctrl+R. It
// deliberately performs no model request: construction only checks local
// configuration, credential presence, and base URL requirements.
func (m *Model) refreshPreflight() {
	if m.delegateRuntime != "" {
		issues := make([]string, 0, 3)
		if m.resumeStatePath != "" || m.forkStatePath != "" {
			issues = append(issues, "provider-owned harnesses cannot continue or fork a native Gator thread")
		}
		if strings.TrimSpace(m.task.Value()) == "" {
			issues = append(issues, "describe a task")
		}
		if m.runMode != gatorrun.ExecuteMode {
			issues = append(issues, delegatedRuntimeLabel(m.delegateRuntime)+" supports Execute mode only; run /execute")
		} else if _, err := parseVerification(m.verification.Value()); err != nil {
			issues = append(issues, err.Error())
		}
		if m.config.NewDelegateCommand == nil {
			issues = append(issues, "this Gator build cannot start a provider-owned harness from the TUI")
		}
		m.preflight = issues
		return
	}
	if m.config.NewExecutor == nil {
		m.preflight = nil
		return
	}
	issues := make([]string, 0, 3)
	if m.resumeStatePath != "" || m.forkStatePath != "" {
		statePath := m.resumeStatePath
		if statePath == "" {
			statePath = m.forkStatePath
		}
		session, err := journal.LoadSession(statePath)
		if err != nil {
			m.preflight = []string{"load retained run: " + err.Error()}
			return
		}
		providerName, modelName, _, err := m.resolveProviderAndModel(session.Provider, session.Model)
		if err != nil {
			m.preflight = []string{err.Error()}
			return
		}
		if strings.TrimSpace(m.task.Value()) == "" {
			issues = append(issues, "describe a continuation instruction")
		}
		if _, err := m.config.NewExecutor(providerName, modelName, session.BaseURL); err != nil {
			issues = append(issues, err.Error())
		}
		m.preflight = issues
		return
	}
	if strings.TrimSpace(m.task.Value()) == "" {
		issues = append(issues, "describe a task")
	}
	if m.runMode == gatorrun.ExecuteMode {
		if _, err := parseVerification(m.verification.Value()); err != nil {
			issues = append(issues, err.Error())
		}
	}
	providerName, modelName, _, err := m.resolveProviderAndModel(m.provider.Value(), m.model.Value())
	if err != nil {
		issues = append(issues, err.Error())
		m.preflight = issues
		return
	}
	if _, err := m.config.NewExecutor(providerName, modelName, m.config.BaseURL); err != nil {
		issues = append(issues, err.Error())
	}
	m.preflight = issues
}
