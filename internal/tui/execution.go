package tui

import (
	"context"
	"crypto/sha256"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/journal"
	modelprovider "github.com/gongahkia/gator/internal/model"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workspace"
)

func (m Model) startRun() (tea.Model, tea.Cmd) {
	task := strings.TrimSpace(m.task.Value())
	if task == "" {
		m.notice = notice{text: "Describe a task before starting a run.", kind: noticeError}
		return m, nil
	}
	if m.config.NewExecutor == nil {
		m.notice = notice{text: "No model provider is configured for this Gator build.", kind: noticeError}
		return m, nil
	}
	references, err := resolveContextReferences(task, m.config.RepositoryPath)
	if err != nil {
		m.notice = notice{text: err.Error(), kind: noticeError}
		return m, nil
	}
	images, err := imageAttachments(m.config.RepositoryPath, references)
	if err != nil {
		m.notice = notice{text: err.Error(), kind: noticeError}
		return m, nil
	}
	imageBytes := 0
	for _, image := range images {
		imageBytes += len(image.Data)
	}
	attachments, err := documentAttachments(m.config.RepositoryPath, references, imageBytes, len(images))
	if err != nil {
		m.notice = notice{text: err.Error(), kind: noticeError}
		return m, nil
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
			m.notice = notice{text: err.Error(), kind: noticeError}
			return m, nil
		}
	}
	modelName := strings.TrimSpace(m.model.Value())
	providerName := strings.TrimSpace(m.provider.Value())
	if providerName == "" {
		m.notice = notice{text: "Choose a provider before starting a run.", kind: noticeError}
		return m, nil
	}
	provider, providerErr := modelprovider.ParseProvider(providerName)
	if providerErr != nil {
		m.notice = notice{text: providerErr.Error(), kind: noticeError}
		return m, nil
	}
	if hasPDFAttachment(attachments) && !modelprovider.SupportsPDFAttachments(provider) {
		m.notice = notice{text: "PDF attachments require the OpenAI Responses, Anthropic Messages, or Gemini provider.", kind: noticeError}
		return m, nil
	}
	modelName = modelprovider.EffectiveModel(provider, modelName)
	m.refreshPreflight()
	if len(m.preflight) > 0 {
		m.notice = notice{text: "Resolve configuration before starting: " + m.preflight[0], kind: noticeError}
		return m, nil
	}
	var executor gatorrun.Executor
	if m.resumeStatePath == "" {
		var executorErr error
		executor, executorErr = m.config.NewExecutor(providerName, modelName, m.config.BaseURL)
		if executorErr != nil {
			m.notice = notice{text: executorErr.Error(), kind: noticeError}
			return m, nil
		}
		if m.runMode == gatorrun.PlanMode && executor.Harness != nil {
			m.notice = notice{text: "Plan mode is enforced only for native providers. Switch to Execute or choose a native provider.", kind: noticeError}
			return m, nil
		}
		if (len(images) > 0 || len(attachments) > 0) && executor.Harness != nil {
			m.notice = notice{text: "File attachments require a native provider; delegated CLI providers cannot receive attachment bytes from Gator.", kind: noticeError}
			return m, nil
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
		events: make(chan agent.Event, 32),
		done:   make(chan executionDone, 1),
		cancel: cancel,
	}
	m.execution = stream
	m.events = nil
	m.appendChat(chatEntry{author: chatUser, text: task})
	m.task.Reset()
	m.task.Placeholder = "Waiting for Gator..."
	m.outcome = nil
	m.runErr = nil
	m.diff = ""
	m.diffErr = nil
	m.diffTruncated = false
	m.screen = runningScreen
	if m.runMode == gatorrun.PlanMode {
		m.notice = notice{text: "Creating an isolated worktree for read-only planning...", kind: noticeInfo}
	} else {
		m.notice = notice{text: "Creating an isolated worktree...", kind: noticeInfo}
	}

	request := gatorrun.Request{
		RepositoryPath: m.config.RepositoryPath,
		Task:           taskWithContextReferences(task, references),
		Provider:       providerName,
		Model:          modelName,
		BaseURL:        m.config.BaseURL,
		MaxSteps:       m.config.MaxSteps,
		Verification:   verification,
		StateDir:       m.config.StateDir,
		ThreadID:       m.threadID,
		Images:         images,
		Attachments:    attachments,
		Mode:           m.runMode,
		OnEvent: func(event agent.Event) {
			select {
			case stream.events <- event:
			case <-ctx.Done():
			}
		},
		AllowExternalCLI: true,
	}

	if m.resumeStatePath != "" {
		previous, loadErr := journal.LoadSession(m.resumeStatePath)
		if loadErr != nil {
			cancel()
			m.execution = nil
			m.screen = composeScreen
			m.notice = notice{text: "Load retained run: " + loadErr.Error(), kind: noticeError}
			return m, nil
		}
		if strings.TrimSpace(previous.Provider) == "" {
			cancel()
			m.execution = nil
			m.screen = composeScreen
			m.notice = notice{text: "The retained run does not record a provider and cannot be continued safely.", kind: noticeError}
			return m, nil
		}
		retainedProvider, providerErr := modelprovider.ParseProvider(previous.Provider)
		if providerErr != nil {
			cancel()
			m.execution = nil
			m.screen = composeScreen
			m.notice = notice{text: providerErr.Error(), kind: noticeError}
			return m, nil
		}
		if hasPDFAttachment(attachments) && !modelprovider.SupportsPDFAttachments(retainedProvider) {
			cancel()
			m.execution = nil
			m.screen = composeScreen
			m.notice = notice{text: "PDF attachments require the OpenAI Responses, Anthropic Messages, or Gemini provider.", kind: noticeError}
			return m, nil
		}
		providerName = previous.Provider
		modelName = modelprovider.EffectiveModel(retainedProvider, previous.Model)
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
			m.notice = notice{text: executorErr.Error(), kind: noticeError}
			return m, nil
		}
		if m.runMode == gatorrun.PlanMode && executor.Harness != nil {
			cancel()
			m.execution = nil
			m.screen = composeScreen
			m.notice = notice{text: "Plan mode is enforced only for native providers. Switch to Execute or choose a native provider.", kind: noticeError}
			return m, nil
		}
		if (len(images) > 0 || len(attachments) > 0) && executor.Harness != nil {
			cancel()
			m.execution = nil
			m.screen = composeScreen
			m.notice = notice{text: "File attachments require a native provider; delegated CLI providers cannot receive attachment bytes from Gator.", kind: noticeError}
			return m, nil
		}
		go executeResume(ctx, stream, executor, previous, m.resumeStatePath, task, request)
	} else {
		go executeNew(ctx, stream, executor, request)
	}
	return m, waitForExecution(stream)
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

func waitForExecution(stream *executionStream) tea.Cmd {
	if stream == nil {
		return nil
	}
	return func() tea.Msg {
		event, ok := <-stream.events
		if ok {
			return agentEventMsg{event: event}
		}
		return executionDoneMsg{done: <-stream.done}
	}
}

func loadDiff(root workspace.Root) tea.Cmd {
	return func() tea.Msg {
		diff, truncated, err := tools.ReviewDiff(context.Background(), root)
		return diffLoadedMsg{diff: diff, truncated: truncated, err: err}
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
	m.resumeStatePath = statePath
	m.threadID = session.ThreadID
	if m.threadID == "" {
		m.threadID = filepath.Base(statePath)
	}
	m.runMode = gatorrun.ExecuteMode
	if session.Mode == gatorrun.PlanMode.String() {
		m.runMode = gatorrun.PlanMode
	}
	m.task.Reset()
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

func (m *Model) returnToComposer() {
	m.screen = composeScreen
	m.resumeStatePath = ""
	m.threadID = ""
	m.runMode = gatorrun.ExecuteMode
	m.task.Reset()
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
	if m.resumeStatePath != "" || strings.TrimSpace(m.config.StateDir) == "" {
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
// configuration, credential presence, base URL requirements, and CLI paths.
func (m *Model) refreshPreflight() {
	if m.config.NewExecutor == nil {
		m.preflight = nil
		return
	}
	issues := make([]string, 0, 3)
	if m.resumeStatePath != "" {
		session, err := journal.LoadSession(m.resumeStatePath)
		if err != nil {
			m.preflight = []string{"load retained run: " + err.Error()}
			return
		}
		provider, err := modelprovider.ParseProvider(session.Provider)
		if err != nil {
			m.preflight = []string{err.Error()}
			return
		}
		if strings.TrimSpace(m.task.Value()) == "" {
			issues = append(issues, "describe a continuation instruction")
		}
		if _, err := m.config.NewExecutor(session.Provider, modelprovider.EffectiveModel(provider, session.Model), session.BaseURL); err != nil {
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
	providerName := strings.TrimSpace(m.provider.Value())
	provider, err := modelprovider.ParseProvider(providerName)
	if err != nil {
		issues = append(issues, err.Error())
		m.preflight = issues
		return
	}
	modelName := modelprovider.EffectiveModel(provider, m.model.Value())
	if _, err := m.config.NewExecutor(providerName, modelName, m.config.BaseURL); err != nil {
		issues = append(issues, err.Error())
	}
	m.preflight = issues
}
