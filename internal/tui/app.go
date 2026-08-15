// Package tui provides Gator's interactive terminal application.
package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/journal"
	modelprovider "github.com/gongahkia/gator/internal/model"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workspace"
)

const defaultMaxSteps = 24

// Config supplies the local configuration and provider factory for an
// interactive session. NewExecutor is injected so the UI stays independent of
// any particular model provider and can be tested without a network request.
type Config struct {
	RepositoryPath string
	Provider       string
	Model          string
	BaseURL        string
	Verification   [][]string
	MaxSteps       int
	StateDir       string
	NewExecutor    func(provider, model, baseURL string) (gatorrun.Executor, error)
}

type screen uint8

const (
	composeScreen screen = iota
	runningScreen
	reviewScreen
)

type field uint8

const (
	taskField field = iota
	verificationField
	providerField
	modelField
)

type noticeKind uint8

const (
	noticeInfo noticeKind = iota
	noticeError
	noticeSuccess
)

type notice struct {
	text string
	kind noticeKind
}

type timelineEntry struct {
	step int
	text string
	kind agent.EventKind
}

type executionStream struct {
	events chan agent.Event
	done   chan executionDone
	cancel context.CancelFunc
}

type executionDone struct {
	outcome gatorrun.Outcome
	err     error
}

type agentEventMsg struct {
	event agent.Event
}

type executionDoneMsg struct {
	done executionDone
}

type diffLoadedMsg struct {
	diff      string
	truncated bool
	err       error
}

// Model is the Bubble Tea state model for Gator's terminal experience.
type Model struct {
	config Config

	screen        screen
	focus         field
	width         int
	height        int
	task          textarea.Model
	verification  textarea.Model
	provider      textinput.Model
	model         textinput.Model
	notice        notice
	commandOutput string
	commandIndex  int
	events        []timelineEntry
	execution     *executionStream
	cancelling    bool

	outcome         *gatorrun.Outcome
	runErr          error
	diff            string
	diffTruncated   bool
	diffErr         error
	resumeStatePath string
}

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	labelStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	keyStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
	panelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
)

// New creates a terminal model in task-composition mode.
func New(config Config) Model {
	if strings.TrimSpace(config.RepositoryPath) == "" {
		config.RepositoryPath = "."
	}
	if strings.TrimSpace(config.Provider) == "" {
		config.Provider = "openai"
	}
	if config.MaxSteps == 0 {
		config.MaxSteps = defaultMaxSteps
	}

	task := textarea.New()
	task.Placeholder = "Describe the bug fix or feature you want to build..."
	task.Prompt = ""
	task.ShowLineNumbers = false
	task.CharLimit = 16 * 1024
	task.SetHeight(7)
	task.SetWidth(76)
	_ = task.Focus()

	verification := textarea.New()
	verification.Placeholder = "one allowed verification command per line"
	verification.Prompt = ""
	verification.ShowLineNumbers = false
	verification.CharLimit = 4 * 1024
	verification.SetHeight(3)
	verification.SetWidth(76)
	verification.SetValue(formatVerification(config.Verification))
	verification.Blur()

	model := textinput.New()
	model.Prompt = ""
	model.Placeholder = "model"
	model.CharLimit = 128
	model.Width = 60
	model.SetValue(config.Model)
	model.Blur()

	provider := textinput.New()
	provider.Prompt = ""
	provider.Placeholder = "provider"
	provider.CharLimit = 64
	provider.Width = 60
	provider.SetValue(config.Provider)
	provider.Blur()

	return Model{
		config:       config,
		screen:       composeScreen,
		task:         task,
		verification: verification,
		provider:     provider,
		model:        model,
		notice: notice{
			text: "Gator works in an isolated Git worktree. Review remains explicit.",
			kind: noticeInfo,
		},
	}
}

// Init starts no external work until the developer explicitly starts a run.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles input and asynchronous agent events.
func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeInputs()
		return m, nil
	case agentEventMsg:
		m.events = append(m.events, renderEvent(msg.event))
		return m, waitForExecution(m.execution)
	case executionDoneMsg:
		m.execution = nil
		m.cancelling = false
		m.outcome = &msg.done.outcome
		m.runErr = msg.done.err
		m.screen = reviewScreen
		if msg.done.err != nil {
			m.notice = notice{text: "Run stopped: " + msg.done.err.Error(), kind: noticeError}
		} else {
			m.notice = notice{text: "Run complete. Inspect the diff and evidence before applying anything.", kind: noticeSuccess}
		}
		if msg.done.outcome.Worktree.Path == "" {
			return m, nil
		}
		return m, loadDiff(msg.done.outcome.Worktree.Root)
	case diffLoadedMsg:
		m.diff, m.diffTruncated, m.diffErr = msg.diff, msg.truncated, msg.err
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case composeScreen:
		return m.updateComposer(message)
	case runningScreen:
		return m.updateRunning(message)
	case reviewScreen:
		return m.updateReview(message)
	default:
		return m, nil
	}
}

func (m Model) updateComposer(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.focus == taskField && m.commandPaletteVisible() {
		switch message.String() {
		case "up", "ctrl+p":
			m.moveCommandSelection(-1)
			return m, nil
		case "down", "ctrl+n":
			m.moveCommandSelection(1)
			return m, nil
		case "enter", "tab":
			return m.executeSelectedCommand()
		case "esc":
			m.task.Reset()
			m.commandIndex = 0
			m.notice = notice{text: "Command palette dismissed.", kind: noticeInfo}
			return m, nil
		}
	}
	switch message.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "?":
		if m.focus == taskField && strings.TrimSpace(m.task.Value()) == "" {
			m.task.SetValue("/")
			m.commandIndex = 0
			return m, nil
		}
	case "ctrl+r":
		return m.startRun()
	case "tab", "shift+tab":
		if m.resumeStatePath != "" {
			m.notice = notice{text: "A continuation inherits the model and verification policy from its original run.", kind: noticeInfo}
			return m, nil
		}
		m.focus = nextField(m.focus, message.String() == "shift+tab")
		return m, m.focusField()
	}

	var command tea.Cmd
	switch m.focus {
	case taskField:
		m.task, command = m.task.Update(message)
		m.normalizeCommandSelection()
	case verificationField:
		m.verification, command = m.verification.Update(message)
	case modelField:
		m.model, command = m.model.Update(message)
	case providerField:
		m.provider, command = m.provider.Update(message)
	}
	return m, command
}

func (m Model) updateRunning(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "ctrl+c":
		if m.execution != nil && !m.cancelling {
			m.execution.cancel()
			m.cancelling = true
			m.notice = notice{text: "Cancellation requested. Waiting for the current operation to stop...", kind: noticeInfo}
		}
	}
	return m, nil
}

func (m Model) updateReview(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "n", "esc":
		m.returnToComposer()
		return m, nil
	case "c":
		return m.prepareContinuation()
	case "d":
		if m.outcome == nil || m.outcome.Worktree.Path == "" {
			m.notice = notice{text: "No retained worktree is available for diff review.", kind: noticeError}
			return m, nil
		}
		m.notice = notice{text: "Refreshing the current worktree diff...", kind: noticeInfo}
		return m, loadDiff(m.outcome.Worktree.Root)
	}
	return m, nil
}

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
	verification, err := parseVerification(m.verification.Value())
	if err != nil {
		m.notice = notice{text: err.Error(), kind: noticeError}
		return m, nil
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
	modelName = modelprovider.EffectiveModel(provider, modelName)
	var executor gatorrun.Executor
	if m.resumeStatePath == "" {
		var executorErr error
		executor, executorErr = m.config.NewExecutor(providerName, modelName, m.config.BaseURL)
		if executorErr != nil {
			m.notice = notice{text: executorErr.Error(), kind: noticeError}
			return m, nil
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	stream := &executionStream{
		events: make(chan agent.Event, 32),
		done:   make(chan executionDone, 1),
		cancel: cancel,
	}
	m.execution = stream
	m.events = nil
	m.outcome = nil
	m.runErr = nil
	m.diff = ""
	m.diffErr = nil
	m.diffTruncated = false
	m.screen = runningScreen
	m.notice = notice{text: "Creating an isolated worktree...", kind: noticeInfo}

	request := gatorrun.Request{
		RepositoryPath: m.config.RepositoryPath,
		Task:           taskWithContextReferences(task, references),
		Provider:       providerName,
		Model:          modelName,
		BaseURL:        m.config.BaseURL,
		MaxSteps:       m.config.MaxSteps,
		Verification:   verification,
		StateDir:       m.config.StateDir,
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
		go executeResume(ctx, stream, executor, previous, m.resumeStatePath, task, request)
	} else {
		go executeNew(ctx, stream, executor, request)
	}
	return m, waitForExecution(stream)
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
	session, err := journal.LoadSession(m.outcome.StatePath)
	if err != nil {
		m.notice = notice{text: "Load retained run: " + err.Error(), kind: noticeError}
		return *m, nil
	}
	m.resumeStatePath = m.outcome.StatePath
	m.task.Reset()
	m.task.Placeholder = "Describe the next instruction for this retained worktree..."
	m.verification.SetValue(formatVerification(session.Verification))
	m.provider.SetValue(session.Provider)
	m.model.SetValue(session.Model)
	m.screen = composeScreen
	m.focus = taskField
	m.notice = notice{text: "Continuing " + filepath.Base(m.resumeStatePath) + " in its retained worktree.", kind: noticeInfo}
	return *m, m.focusField()
}

func (m *Model) returnToComposer() {
	m.screen = composeScreen
	m.resumeStatePath = ""
	m.task.Reset()
	m.task.Placeholder = "Describe the bug fix or feature you want to build..."
	m.focus = taskField
	m.commandOutput = ""
	m.notice = notice{text: "Ready for a new isolated task.", kind: noticeInfo}
}

func (m *Model) focusField() tea.Cmd {
	m.task.Blur()
	m.verification.Blur()
	m.provider.Blur()
	m.model.Blur()
	switch m.focus {
	case taskField:
		return m.task.Focus()
	case verificationField:
		return m.verification.Focus()
	case providerField:
		return m.provider.Focus()
	case modelField:
		return m.model.Focus()
	default:
		return nil
	}
}

func (m *Model) resizeInputs() {
	width := m.width - 8
	if width < 28 {
		width = 28
	}
	m.task.SetWidth(width)
	m.verification.SetWidth(width)
	m.provider.Width = width
	m.model.Width = width
}

func (m Model) View() string {
	if m.width == 0 {
		return "Starting Gator..."
	}
	switch m.screen {
	case composeScreen:
		return m.composeView()
	case runningScreen:
		return m.runningView()
	case reviewScreen:
		return m.reviewView()
	default:
		return ""
	}
}

func (m Model) composeView() string {
	mode := "new isolated run"
	if m.resumeStatePath != "" {
		mode = "continue retained run"
	}
	verificationHint := "One allowed argv command per line. Each must pass before Gator accepts completion."
	providerHint := "Set GATOR_PROVIDER before launch or edit it here."
	modelHint := "Set GATOR_MODEL before launch or edit it here."
	if m.resumeStatePath != "" {
		verificationHint = "Inherited from the retained run to preserve its command policy."
		providerHint = "Inherited from the retained run to preserve provider continuity."
		modelHint = "Inherited from the retained run to preserve conversation continuity."
	}
	sections := []string{
		m.header(mode),
		m.fieldView("Task", "Explain the desired behavior and any constraints.", m.task.View()),
	}
	if palette := m.commandPaletteView(); palette != "" {
		sections = append(sections, palette, m.noticeView(), m.footer("up/down choose", "enter select", "esc dismiss", "ctrl+c quit"))
		return strings.Join(sections, "\n\n")
	}
	if references := m.contextReferencesView(); references != "" {
		sections = append(sections, references)
	}
	sections = append(sections,
		m.fieldView("Verification", verificationHint, m.verification.View()),
		m.fieldView("Provider", providerHint, m.provider.View()),
		m.fieldView("Model", modelHint, m.model.View()),
	)
	if m.commandOutput != "" {
		sections = append(sections, labelStyle.Render("Command result"), panelStyle.Width(max(28, m.width-4)).Render(m.commandOutput))
	}
	sections = append(sections,
		m.noticeView(),
		m.footer("? commands", "tab switch field", "ctrl+r start run", "ctrl+c quit"),
	)
	return strings.Join(sections, "\n\n")
}

func (m Model) runningView() string {
	status := "Gator is working in an isolated worktree."
	if m.cancelling {
		status = "Gator is stopping; the retained worktree will remain reviewable."
	}
	entries := m.events
	maxEntries := m.height - 11
	if maxEntries < 5 {
		maxEntries = 5
	}
	if len(entries) > maxEntries {
		entries = entries[len(entries)-maxEntries:]
	}
	lines := make([]string, 0, len(entries)+1)
	if len(entries) == 0 {
		lines = append(lines, dimStyle.Render("waiting for the first agent event..."))
	}
	for _, entry := range entries {
		lines = append(lines, entry.text)
	}
	sections := []string{
		m.header("live run"),
		status,
		panelStyle.Width(max(28, m.width-4)).Render(strings.Join(lines, "\n")),
		m.noticeView(),
		m.footer("ctrl+c stop after current operation"),
	}
	return strings.Join(sections, "\n\n")
}

func (m Model) reviewView() string {
	sections := []string{m.header("review")}
	if m.outcome != nil {
		sections = append(sections, labelStyle.Render("Retained worktree"), m.outcome.Worktree.Path)
		sections = append(sections, labelStyle.Render("Run record"), m.outcome.StatePath)
	}
	if m.runErr != nil {
		sections = append(sections, errorStyle.Render("Run result: "+m.runErr.Error()))
	} else if m.outcome != nil && strings.TrimSpace(m.outcome.Result.FinalText) != "" {
		sections = append(sections, panelStyle.Width(max(28, m.width-4)).Render(m.outcome.Result.FinalText))
	}
	sections = append(sections, labelStyle.Render("Current diff"), m.diffView(), m.noticeView())
	continueLabel := "c continue retained run"
	if m.outcome == nil || m.outcome.StatePath == "" {
		continueLabel = ""
	}
	sections = append(sections, m.footer("d refresh diff", continueLabel, "n new task", "q quit"))
	return strings.Join(sections, "\n\n")
}

func (m Model) header(mode string) string {
	repository := filepath.Base(filepath.Clean(m.config.RepositoryPath))
	return headerStyle.Render("Gator") + "  " + dimStyle.Render(mode+" · "+repository)
}

func (m Model) fieldView(label, hint, value string) string {
	return labelStyle.Render(label) + "\n" + dimStyle.Render(hint) + "\n" + panelStyle.Width(max(28, m.width-4)).Render(value)
}

func (m Model) diffView() string {
	if m.diffErr != nil {
		return errorStyle.Render("Unable to load diff: " + m.diffErr.Error())
	}
	if m.diff == "" {
		return dimStyle.Render("No changed files are currently visible in the retained worktree.")
	}
	lines := strings.Split(m.diff, "\n")
	maxLines := m.height - 19
	if maxLines < 5 {
		maxLines = 5
	}
	truncatedByView := len(lines) > maxLines
	if truncatedByView {
		lines = lines[:maxLines]
	}
	content := strings.Join(lines, "\n")
	if m.diffTruncated || truncatedByView {
		content += "\n" + dimStyle.Render("… diff preview truncated; inspect the retained worktree for the full patch.")
	}
	return panelStyle.Width(max(28, m.width-4)).Render(content)
}

func (m Model) noticeView() string {
	if strings.TrimSpace(m.notice.text) == "" {
		return ""
	}
	switch m.notice.kind {
	case noticeError:
		return errorStyle.Render(m.notice.text)
	case noticeSuccess:
		return okStyle.Render(m.notice.text)
	default:
		return dimStyle.Render(m.notice.text)
	}
}

func (m Model) footer(keys ...string) string {
	var rendered []string
	for _, key := range keys {
		if key == "" {
			continue
		}
		parts := strings.SplitN(key, " ", 2)
		if len(parts) == 1 {
			rendered = append(rendered, keyStyle.Render(parts[0]))
			continue
		}
		rendered = append(rendered, keyStyle.Render(parts[0])+" "+dimStyle.Render(parts[1]))
	}
	return strings.Join(rendered, "  ")
}

func (m Model) commandPaletteVisible() bool {
	return len(m.matchingCommands()) > 0
}

func (m Model) matchingCommands() []slashCommand {
	if m.focus != taskField {
		return nil
	}
	return matchingSlashCommands(m.task.Value())
}

func (m *Model) normalizeCommandSelection() {
	matches := m.matchingCommands()
	if len(matches) == 0 {
		m.commandIndex = 0
		return
	}
	if m.commandIndex >= len(matches) {
		m.commandIndex = len(matches) - 1
	}
}

func (m *Model) moveCommandSelection(delta int) {
	matches := m.matchingCommands()
	if len(matches) == 0 {
		m.commandIndex = 0
		return
	}
	m.commandIndex = (m.commandIndex + delta + len(matches)) % len(matches)
}

func (m Model) executeSelectedCommand() (tea.Model, tea.Cmd) {
	matches := m.matchingCommands()
	if len(matches) == 0 {
		return m, nil
	}
	command := matches[m.commandIndex]
	m.task.Reset()
	m.commandIndex = 0
	switch command.name {
	case "/clear":
		m.commandOutput = ""
		m.notice = notice{text: "Task cleared.", kind: noticeInfo}
	case "/help":
		m.commandOutput = commandHelp()
		m.notice = notice{text: "Commands operate locally and never start a run by themselves.", kind: noticeInfo}
	case "/model":
		m.commandOutput = ""
		m.focus = modelField
		m.notice = notice{text: "Edit the model, then Tab back to the task.", kind: noticeInfo}
		return m, m.focusField()
	case "/provider":
		m.commandOutput = ""
		m.focus = providerField
		m.notice = notice{text: "Edit the provider, then Tab back to the task.", kind: noticeInfo}
		return m, m.focusField()
	case "/permissions":
		m.commandOutput = m.permissionsStatus()
		if isExternalProvider(m.provider.Value()) {
			m.notice = notice{text: "This provider delegates tool permissions to its vendor CLI; Gator verifies the final worktree.", kind: noticeInfo}
		} else {
			m.notice = notice{text: "Verifier commands are the only commands the agent may run.", kind: noticeInfo}
		}
	case "/quit":
		return m, tea.Quit
	case "/review":
		if m.outcome == nil || m.outcome.Worktree.Path == "" {
			m.notice = notice{text: "No completed or retained run is available to review yet.", kind: noticeError}
			return m, nil
		}
		m.screen = reviewScreen
		return m, nil
	case "/status":
		m.commandOutput = m.sessionStatus()
		m.notice = notice{text: "Current configuration shown below.", kind: noticeInfo}
	case "/verify":
		m.commandOutput = ""
		m.focus = verificationField
		m.notice = notice{text: "Edit the allowed verification commands, one argv per line.", kind: noticeInfo}
		return m, m.focusField()
	case "/worktree":
		m.commandOutput = "Every new Gator run creates a detached worktree beside this repository. The agent can edit only that worktree; your active checkout stays unchanged."
		m.notice = notice{text: "Worktree isolation is always on for new runs.", kind: noticeInfo}
	}
	return m, nil
}

func (m Model) commandPaletteView() string {
	matches := m.matchingCommands()
	if len(matches) == 0 {
		if strings.HasPrefix(strings.TrimSpace(m.task.Value()), "/") {
			return errorStyle.Render("No Gator command matches this input.")
		}
		return ""
	}
	lines := make([]string, 0, len(matches))
	for index, command := range matches {
		prefix := "  "
		if index == m.commandIndex {
			prefix = "> "
		}
		lines = append(lines, prefix+keyStyle.Render(command.name)+"  "+dimStyle.Render(command.description))
	}
	return labelStyle.Render("Commands") + "\n" + panelStyle.Width(max(28, m.width-4)).Render(strings.Join(lines, "\n")) + "\n" + dimStyle.Render("up/down choose · enter run command · esc dismiss")
}

func (m Model) contextReferencesView() string {
	references := extractContextReferences(m.task.Value())
	if len(references) == 0 {
		return ""
	}
	values := make([]string, 0, len(references))
	for _, reference := range references {
		values = append(values, keyStyle.Render("@"+reference))
	}
	return labelStyle.Render("Context references") + "\n" + panelStyle.Width(max(28, m.width-4)).Render(strings.Join(values, "  ")) + "\n" + dimStyle.Render("Gator validates these paths before a run; the agent inspects them first.")
}

func (m Model) sessionStatus() string {
	verification, err := parseVerification(m.verification.Value())
	verificationText := "invalid: " + err.Error()
	if err == nil {
		verificationText = formatVerification(verification)
	}
	return "repository: " + m.config.RepositoryPath + "\nprovider: " + m.provider.Value() + "\nmodel: " + m.model.Value() + "\nmax steps: " + fmt.Sprint(m.config.MaxSteps) + "\nverification:\n" + verificationText
}

func (m Model) permissionsStatus() string {
	verification, err := parseVerification(m.verification.Value())
	commands := "invalid verifier configuration: " + err.Error()
	if err == nil {
		commands = formatVerification(verification)
	}
	if isExternalProvider(m.provider.Value()) {
		return "writes: isolated run worktree only\nprovider: delegated CLI with its own permission policy\nGator runs required verification after the CLI exits:\n" + commands + "\nactive checkout: never edited by a normal run"
	}
	return "writes: isolated run worktree only\nreads: repository paths only\ncommands allowed:\n" + commands + "\nactive checkout: never edited by a normal run"
}

func commandHelp() string {
	lines := make([]string, 0, len(slashCommands))
	for _, command := range slashCommands {
		lines = append(lines, command.name+" — "+command.description)
	}
	return strings.Join(lines, "\n")
}

func renderEvent(event agent.Event) timelineEntry {
	prefix := fmt.Sprintf("[%02d]", event.Step)
	var text string
	switch event.Kind {
	case agent.EventTurnStarted:
		text = prefix + " thinking"
	case agent.EventTextDelta:
		text = prefix + " agent: " + compact(event.Text, 120)
	case agent.EventText:
		text = prefix + " agent: " + compact(event.Text, 120)
	case agent.EventToolCalled:
		text = prefix + " tool -> " + event.ToolCall.Name
	case agent.EventToolFinished:
		if event.ToolError == "" {
			text = prefix + " tool ok " + event.ToolCall.Name
		} else {
			text = prefix + " tool failed " + event.ToolCall.Name + ": " + compact(event.ToolError, 100)
		}
	case agent.EventCompletionBlocked:
		text = prefix + " evidence required: " + compact(event.Text, 120)
	case agent.EventHarnessStarted:
		text = prefix + " delegated CLI -> " + compact(event.Text, 80)
	case agent.EventHarnessFinished:
		if event.ToolError == "" {
			text = prefix + " delegated CLI ok " + compact(event.Text, 80)
		} else {
			text = prefix + " delegated CLI failed: " + compact(event.ToolError, 100)
		}
	case agent.EventRunFinished:
		text = prefix + " completion proposed"
	default:
		text = prefix + " event"
	}
	return timelineEntry{step: event.Step, text: text, kind: event.Kind}
}

func compact(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= limit {
		return value
	}
	return value[:limit-1] + "…"
}

func isExternalProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "codex", "claude", "copilot", "cursor":
		return true
	default:
		return false
	}
}

func nextField(current field, reverse bool) field {
	if reverse {
		if current == taskField {
			return modelField
		}
		return current - 1
	}
	if current == modelField {
		return taskField
	}
	return current + 1
}

func formatVerification(commands [][]string) string {
	lines := make([]string, 0, len(commands))
	for _, command := range commands {
		if len(command) > 0 {
			lines = append(lines, strings.Join(command, " "))
		}
	}
	return strings.Join(lines, "\n")
}

func parseVerification(value string) ([][]string, error) {
	var commands [][]string
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		argv := strings.Fields(line)
		if len(argv) == 0 {
			continue
		}
		commands = append(commands, argv)
	}
	if len(commands) == 0 {
		return nil, errors.New("add at least one verification command before starting a run")
	}
	return commands, nil
}
