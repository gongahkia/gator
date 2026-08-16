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
	helpScreen
	recentScreen
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
	dropdownIndex int
	contextIndex  int
	contextPaths  []string
	contextLoaded bool
	contextErr    error
	contextClosed bool
	preflight     []string
	draftErr      error
	helpReturn    screen
	recentThreads []journal.RecentThread
	recentIndex   int
	events        []timelineEntry
	execution     *executionStream
	cancelling    bool

	outcome         *gatorrun.Outcome
	runErr          error
	diff            string
	diffTruncated   bool
	diffErr         error
	resumeStatePath string
	threadID        string
	runMode         gatorrun.Mode
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

	application := Model{
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
	if strings.TrimSpace(config.StateDir) != "" {
		if draft, found, err := journal.LoadDraft(config.StateDir, config.RepositoryPath); err != nil {
			application.draftErr = err
			application.notice = notice{text: "Draft recovery unavailable: " + err.Error(), kind: noticeError}
		} else if found {
			application.task.SetValue(draft.Task)
			application.verification.SetValue(draft.Verification)
			application.provider.SetValue(draft.Provider)
			application.model.SetValue(draft.Model)
			application.notice = notice{text: "Restored the unfinished draft saved " + draft.UpdatedAt.Local().Format("Jan 2 15:04") + ".", kind: noticeInfo}
		}
	}
	application.refreshPreflight()
	return application
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
	case agentEventMsg:
		m.events = append(m.events, renderEvent(msg.event))
		return m, waitForExecution(m.execution)
		case executionDoneMsg:
		m.execution = nil
		m.cancelling = false
		m.outcome = &msg.done.outcome
		m.threadID = msg.done.outcome.ThreadID
		if msg.done.outcome.StatePath != "" {
			m.resumeStatePath = msg.done.outcome.StatePath
		}
		m.runErr = msg.done.err
		m.screen = reviewScreen
		if msg.done.err != nil {
			m.notice = notice{text: m.runMode.String() + " stopped: " + msg.done.err.Error(), kind: noticeError}
		} else {
			if m.runMode == gatorrun.PlanMode {
				m.notice = notice{text: "Plan ready. Continue this thread in Execute mode when you are ready to make changes.", kind: noticeSuccess}
			} else {
				m.notice = notice{text: "Run complete. Inspect the diff and evidence before applying anything.", kind: noticeSuccess}
			}
		}
		if msg.done.outcome.StatePath != "" && m.resumeStatePath == "" && strings.TrimSpace(m.config.StateDir) != "" {
			if err := journal.DeleteDraft(m.config.StateDir, m.config.RepositoryPath); err != nil {
				m.draftErr = err
			}
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
	if message.String() == "f1" {
		if m.screen == helpScreen {
			m.screen = m.helpReturn
		} else {
			m.helpReturn = m.screen
			m.screen = helpScreen
		}
		return m, nil
	}
	switch m.screen {
	case composeScreen:
		return m.updateComposer(message)
	case runningScreen:
		return m.updateRunning(message)
	case reviewScreen:
		return m.updateReview(message)
	case helpScreen:
		return m.updateHelp(message)
	case recentScreen:
		return m.updateRecentRuns(message)
	default:
		return m, nil
	}
}

func (m Model) updateComposer(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.focus == taskField && message.Type == tea.KeyCtrlAt {
		m.contextClosed = false
		m.normalizeContextSelection()
		if !m.contextCompletionVisible() {
			m.notice = notice{text: "Type @ followed by a repository path to open path suggestions.", kind: noticeInfo}
		}
		return m, nil
	}
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
	if m.focus == taskField && m.contextCompletionVisible() {
		switch message.String() {
		case "up", "ctrl+p":
			m.moveContextSelection(-1)
			return m, nil
		case "down", "ctrl+n":
			m.moveContextSelection(1)
			return m, nil
		case "enter", "tab":
			m.applySelectedContextCompletion()
			return m, nil
		case "esc":
			m.contextClosed = true
			m.notice = notice{text: "Path suggestions dismissed.", kind: noticeInfo}
			return m, nil
		}
	}
	if m.dropdownVisible() {
		switch message.String() {
		case "up", "ctrl+p":
			m.moveDropdownSelection(-1)
			return m, nil
		case "down", "ctrl+n":
			m.moveDropdownSelection(1)
			return m, nil
		case "enter":
			m.applySelectedDropdown()
			return m, nil
		case "tab":
			m.applySelectedDropdown()
			m.focus = nextField(m.focus, false)
			return m, m.focusField()
		}
	}
	switch message.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "ctrl+o":
		return m.openRecentRuns()
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
		m.contextClosed = false
		m.normalizeContextSelection()
		m.persistDraft()
		m.refreshPreflight()
	case verificationField:
		m.verification, command = m.verification.Update(message)
		m.persistDraft()
		m.refreshPreflight()
	case modelField:
		m.model, command = m.model.Update(message)
		m.normalizeDropdownSelection()
		m.persistDraft()
		m.refreshPreflight()
	case providerField:
		m.provider, command = m.provider.Update(message)
		m.normalizeDropdownSelection()
		m.persistDraft()
		m.refreshPreflight()
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

func (m Model) updateHelp(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc", "f1", "?", "q":
		m.screen = m.helpReturn
	}
	return m, nil
}

func (m Model) openRecentRuns() (tea.Model, tea.Cmd) {
	threads, err := journal.ListRecentThreads(m.config.StateDir, m.config.RepositoryPath, 20)
	if err != nil {
		m.notice = notice{text: "Load recent threads: " + err.Error(), kind: noticeError}
		return m, nil
	}
	if len(threads) == 0 {
		m.notice = notice{text: "No retained threads are available for this repository.", kind: noticeInfo}
		return m, nil
	}
	m.recentThreads = threads
	m.recentIndex = 0
	m.screen = recentScreen
	return m, nil
}

func (m Model) updateRecentRuns(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc", "q":
		m.screen = composeScreen
		return m, m.focusField()
	case "r", "ctrl+r":
		m.screen = composeScreen
		return m.openRecentRuns()
	case "up", "ctrl+p":
		if len(m.recentThreads) > 0 {
			m.recentIndex = (m.recentIndex - 1 + len(m.recentThreads)) % len(m.recentThreads)
		}
	case "down", "ctrl+n":
		if len(m.recentThreads) > 0 {
			m.recentIndex = (m.recentIndex + 1) % len(m.recentThreads)
		}
	case "enter":
		if len(m.recentThreads) == 0 {
			return m, nil
		}
		selected := m.recentThreads[m.recentIndex]
		if !selected.Available {
			m.notice = notice{text: "The selected retained worktree no longer exists.", kind: noticeError}
			return m, nil
		}
		return m.beginContinuation(selected.HeadStatePath)
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
	return m.beginContinuation(m.outcome.StatePath)
}

func (m *Model) beginContinuation(statePath string) (tea.Model, tea.Cmd) {
	session, err := journal.LoadSession(statePath)
	if err != nil {
		m.notice = notice{text: "Load retained run: " + err.Error(), kind: noticeError}
		return *m, nil
	}
	m.resumeStatePath = statePath
	m.task.Reset()
	m.task.Placeholder = "Describe the next instruction for this retained worktree..."
	m.verification.SetValue(formatVerification(session.Verification))
	m.provider.SetValue(session.Provider)
	m.model.SetValue(session.Model)
	m.screen = composeScreen
	m.focus = taskField
	m.notice = notice{text: "Continuing " + filepath.Base(m.resumeStatePath) + " in its retained worktree.", kind: noticeInfo}
	m.refreshPreflight()
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
	if _, err := parseVerification(m.verification.Value()); err != nil {
		issues = append(issues, err.Error())
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

func (m Model) preflightView() string {
	label := "Before starting"
	ready := "Ready to create an isolated worktree."
	if m.resumeStatePath != "" {
		label = "Before continuing"
		ready = "Ready to continue the retained worktree."
	}
	if len(m.preflight) == 0 {
		return labelStyle.Render("Run readiness") + "\n" + okStyle.Render(ready)
	}
	lines := make([]string, 0, len(m.preflight))
	for _, issue := range m.preflight {
		lines = append(lines, "• "+issue)
	}
	return labelStyle.Render(label) + "\n" + m.panel(errorStyle.Render(strings.Join(lines, "\n")))
}

func (m Model) draftWarningView() string {
	if m.draftErr == nil {
		return ""
	}
	return labelStyle.Render("Draft recovery") + "\n" + m.inline(errorStyle.Render(compact("The current draft could not be saved: "+m.draftErr.Error(), m.inlineWidth())))
}

func (m *Model) focusField() tea.Cmd {
	m.task.Blur()
	m.verification.Blur()
	m.provider.Blur()
	m.model.Blur()
	m.normalizeDropdownSelection()
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
	width := max(1, m.width-8)
	m.task.SetWidth(width)
	m.verification.SetWidth(width)
	m.provider.Width = width
	m.model.Width = width

	switch {
	case m.height < 20:
		m.task.SetHeight(2)
		m.verification.SetHeight(1)
	case m.height < 34:
		m.task.SetHeight(3)
		m.verification.SetHeight(1)
	case m.height < 48:
		m.task.SetHeight(5)
		m.verification.SetHeight(2)
	default:
		m.task.SetHeight(7)
		m.verification.SetHeight(3)
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return "Starting Gator..."
	}
	var view string
	switch m.screen {
	case composeScreen:
		view = m.composeView()
	case runningScreen:
		view = m.runningView()
	case reviewScreen:
		view = m.reviewView()
	case helpScreen:
		view = m.helpView()
	case recentScreen:
		view = m.recentRunsView()
	default:
		return ""
	}
	return m.fitToTerminal(view)
}

func (m Model) composeView() string {
	mode := "new isolated run"
	if m.resumeStatePath != "" {
		mode = "continue retained run"
	}
	if m.compactComposer() || (m.height < 52 && (m.commandPaletteVisible() || m.contextCompletionVisible() || m.dropdownVisible())) {
		return m.compactComposeView(mode)
	}
	verificationHint := "One allowed argv command per line. Each must pass before Gator accepts completion."
	providerHint := "Choose from the dropdown or type to filter providers."
	modelHint := "Choose a recommendation or type any model ID supported by the provider."
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
		sections = append(sections, palette, m.noticeView(), m.footer("up/down choose", "enter select", "esc dismiss", "f1 shortcuts", "ctrl+c quit"))
		return strings.Join(sections, "\n")
	}
	if references := m.contextReferencesView(); references != "" {
		sections = append(sections, references)
	}
	if completions := m.contextCompletionView(); completions != "" {
		sections = append(sections, completions)
	}
	providerSection := m.fieldView("Provider", providerHint, m.provider.View())
	modelSection := m.fieldView("Model", modelHint, m.model.View())
	if dropdown := m.dropdownView(); dropdown != "" {
		if m.focus == providerField {
			providerSection += "\n" + dropdown
		} else {
			modelSection += "\n" + dropdown
		}
	}
	sections = append(sections,
		m.fieldView("Verification", verificationHint, m.verification.View()),
		providerSection,
		modelSection,
	)
	if readiness := m.preflightView(); readiness != "" {
		sections = append(sections, readiness)
	}
	if warning := m.draftWarningView(); warning != "" {
		sections = append(sections, warning)
	}
	if m.commandOutput != "" {
		sections = append(sections, labelStyle.Render("Command result"), m.panel(m.commandOutput))
	}
	sections = append(sections,
		m.noticeView(),
		m.footer("? commands", "ctrl+o recent", "tab switch field", "ctrl+r start run", "f1 shortcuts", "ctrl+c quit"),
	)
	return strings.Join(sections, "\n")
}

func (m Model) compactComposeView(mode string) string {
	sections := []string{
		m.header(mode),
		m.fieldView("Task", "", m.task.View()),
	}
	if palette := m.commandPaletteView(); palette != "" {
		sections = append(sections, palette, m.noticeView(), m.footer("up/down choose", "enter select", "esc dismiss", "f1 shortcuts", "ctrl+c quit"))
		return strings.Join(sections, "\n")
	}
	if completions := m.contextCompletionView(); completions != "" {
		sections = append(sections, completions, m.noticeView(), m.footer("up/down choose", "enter insert", "esc dismiss", "f1 shortcuts", "ctrl+c quit"))
		return strings.Join(sections, "\n")
	}

	if m.focus != taskField {
		label, value := "", ""
		switch m.focus {
		case verificationField:
			label, value = "Verification", m.verification.View()
		case providerField:
			label, value = "Provider", m.provider.View()
		case modelField:
			label, value = "Model", m.model.View()
		}
		sections = append(sections, m.fieldView(label, "", value))
		if dropdown := m.dropdownView(); dropdown != "" {
			sections = append(sections, dropdown)
		}
	}

	sections = append(sections, m.composerSummary(), m.compactPreflightView())
	if m.commandOutput != "" {
		sections = append(sections, labelStyle.Render("Command result"), m.panel(compact(m.commandOutput, max(16, m.panelTextWidth()*3))))
	}
	if warning := m.draftWarningView(); warning != "" {
		sections = append(sections, warning)
	}
	sections = append(sections,
		m.noticeView(),
		m.footer("? commands", "ctrl+o recent", "tab switch field", "ctrl+r start run", "f1 shortcuts", "ctrl+c quit"),
	)
	return strings.Join(sections, "\n")
}

func (m Model) composerSummary() string {
	verification := "no commands"
	if commands, err := parseVerification(m.verification.Value()); err == nil && len(commands) > 0 {
		verification = fmt.Sprintf("%d command(s)", len(commands))
	} else if strings.TrimSpace(m.verification.Value()) != "" {
		verification = "needs attention"
	}
	provider := strings.TrimSpace(m.provider.Value())
	if provider == "" {
		provider = "not selected"
	}
	model := strings.TrimSpace(m.model.Value())
	if model == "" {
		model = "provider default"
	}
	text := "verify: " + verification + " · provider: " + provider + " · model: " + model
	return m.inline(dimStyle.Render(compact(text, m.inlineWidth())))
}

func (m Model) compactPreflightView() string {
	if len(m.preflight) == 0 {
		return m.inline(okStyle.Render("Ready to create an isolated worktree."))
	}
	label := "Before starting: "
	if m.resumeStatePath != "" {
		label = "Before continuing: "
	}
	return m.inline(errorStyle.Render(compact(label+m.preflight[0], m.inlineWidth())))
}

func (m Model) helpView() string {
	if m.compactLayout() {
		lines := []string{
			"F1  close this help",
			"Ctrl+R  start the configured run",
			"Ctrl+O  choose a retained run",
			"Tab / Shift+Tab  move between fields",
			"?  open commands; @  reference a path",
			"Ctrl+C  quit",
		}
		if m.constrainedLayout() {
			lines = []string{
				"F1  close this help",
				"Ctrl+R  start a run",
				"Tab  move between fields",
				"Ctrl+C  quit",
			}
		}
		return strings.Join([]string{
			m.header("keyboard shortcuts"),
			m.panel(strings.Join(lines, "\n")),
			m.footer("esc close help"),
		}, "\n")
	}
	sections := []string{
		m.header("keyboard shortcuts"),
		labelStyle.Render("Composer") + "\n" + m.panel(strings.Join([]string{
			"F1  show or close this help",
			"Ctrl+R  start the configured run",
			"Ctrl+O  choose a retained run",
			"Tab / Shift+Tab  move between fields",
			"?  open the / command menu from an empty task",
			"@  begin a repository-path reference",
			"Ctrl+Space (Ctrl+@)  reopen @ path suggestions",
			"Arrows + Enter or Tab  choose an open suggestion",
			"Ctrl+C  quit",
		}, "\n")),
		labelStyle.Render("Running") + "\n" + m.panel("F1  show this help\nCtrl+C  request cancellation and retain the worktree"),
		labelStyle.Render("Review") + "\n" + m.panel("F1  show this help\nc  continue the retained worktree\nn or Esc  start a new task\nd  refresh the diff\nq or Ctrl+C  quit"),
		m.footer("esc close help"),
	}
	return strings.Join(sections, "\n")
}

func (m Model) recentRunsView() string {
	start, end := m.visibleRange(len(m.recentRuns), m.recentIndex, m.recentRunLimit())
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		run := m.recentRuns[index]
		prefix := "  "
		if index == m.recentIndex {
			prefix = "> "
		}
		modelName := run.Model
		if modelName == "" {
			modelName = "provider default"
		}
		availability := okStyle.Render("path found")
		if !run.Available {
			availability = errorStyle.Render("worktree missing")
		}
		limit := 96
		if m.compactLayout() {
			limit = max(12, m.panelTextWidth()-22)
		}
		lines = append(lines, prefix+keyStyle.Render(run.Provider+" · "+modelName)+"  "+availability+"\n    "+dimStyle.Render(run.UpdatedAt.Local().Format("Jan 2 15:04"))+"  "+compact(run.Task, limit))
	}
	sections := []string{
		m.header("recent retained runs"),
		m.panel(strings.Join(lines, "\n\n")),
	}
	if !m.compactLayout() {
		sections = append(sections, dimStyle.Render("Run-record paths stay private; Gator validates a selected worktree before continuation."))
	}
	sections = append(sections, m.noticeView(), m.footer("up/down choose", "enter continue", "r refresh", "esc back", "f1 shortcuts"))
	return strings.Join(sections, "\n")
}

func (m Model) runningView() string {
	status := "Gator is working in an isolated worktree."
	if m.cancelling {
		status = "Gator is stopping; the retained worktree will remain reviewable."
	}
	entries := m.events
	maxEntries := max(1, m.height-7)
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
		m.inline(dimStyle.Render(compact(status, m.inlineWidth()))),
		m.panel(strings.Join(lines, "\n")),
		m.noticeView(),
		m.footer("ctrl+c stop after current operation", "f1 shortcuts"),
	}
	return strings.Join(sections, "\n")
}

func (m Model) reviewView() string {
	sections := []string{m.header("review")}
	if m.outcome != nil {
		if m.compactLayout() {
			sections = append(sections, m.inline(dimStyle.Render(compact("worktree: "+m.outcome.Worktree.Path, m.inlineWidth()))))
		} else {
			sections = append(sections, labelStyle.Render("Retained worktree"), m.inline(m.outcome.Worktree.Path))
			sections = append(sections, labelStyle.Render("Run record"), m.inline(m.outcome.StatePath))
		}
	}
	if m.runErr != nil {
		sections = append(sections, m.inline(errorStyle.Render(compact("Run result: "+m.runErr.Error(), m.inlineWidth()))))
	} else if m.outcome != nil && strings.TrimSpace(m.outcome.Result.FinalText) != "" {
		result := m.outcome.Result.FinalText
		if m.compactLayout() {
			result = compact(result, max(16, m.panelTextWidth()*2))
		}
		sections = append(sections, m.panel(result))
	}
	sections = append(sections, labelStyle.Render("Current diff"), m.diffView(), m.noticeView())
	continueLabel := "c continue retained run"
	if m.outcome == nil || m.outcome.StatePath == "" {
		continueLabel = ""
	}
	sections = append(sections, m.footer("d refresh diff", continueLabel, "n new task", "f1 shortcuts", "q quit"))
	return strings.Join(sections, "\n")
}

func (m Model) header(mode string) string {
	repository := filepath.Base(filepath.Clean(m.config.RepositoryPath))
	return m.inline(headerStyle.Render("Gator") + "  " + dimStyle.Render(mode+" · "+repository))
}

func (m Model) fieldView(label, hint, value string) string {
	sections := []string{m.inline(labelStyle.Render(label))}
	if hint != "" {
		sections = append(sections, m.inline(dimStyle.Render(compact(hint, m.inlineWidth()))))
	}
	sections = append(sections, m.panel(value))
	return strings.Join(sections, "\n")
}

func (m Model) diffView() string {
	if m.diffErr != nil {
		return errorStyle.Render("Unable to load diff: " + m.diffErr.Error())
	}
	if m.diff == "" {
		return dimStyle.Render("No changed files are currently visible in the retained worktree.")
	}
	lines := strings.Split(m.diff, "\n")
	maxLines := max(1, m.height-10)
	truncatedByView := len(lines) > maxLines
	if truncatedByView {
		lines = lines[:maxLines]
	}
	content := strings.Join(lines, "\n")
	if m.diffTruncated || truncatedByView {
		content += "\n" + dimStyle.Render("… diff preview truncated; inspect the retained worktree for the full patch.")
	}
	return m.panel(content)
}

func (m Model) noticeView() string {
	if strings.TrimSpace(m.notice.text) == "" {
		return ""
	}
	switch m.notice.kind {
	case noticeError:
		return m.inline(errorStyle.Render(compact(m.notice.text, m.inlineWidth())))
	case noticeSuccess:
		return m.inline(okStyle.Render(compact(m.notice.text, m.inlineWidth())))
	default:
		return m.inline(dimStyle.Render(compact(m.notice.text, m.inlineWidth())))
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
	return m.inline(strings.Join(rendered, "  "))
}

func (m Model) compactLayout() bool {
	if m.width == 0 || m.height == 0 {
		return false
	}
	return m.width < 72 || m.height < 40
}

func (m Model) compactComposer() bool {
	if m.width == 0 || m.height == 0 {
		return false
	}
	return m.width < 72 || m.height < 40
}

func (m Model) constrainedLayout() bool {
	if m.width == 0 || m.height == 0 {
		return false
	}
	return m.width < 48 || m.height < 18
}

func (m Model) panelTextWidth() int {
	return max(1, m.width-8)
}

func (m Model) inlineWidth() int {
	return max(1, m.width)
}

func (m Model) panel(value string) string {
	if m.width < 8 {
		return m.inline(value)
	}
	return panelStyle.Width(m.width - 4).Render(value)
}

func (m Model) inline(value string) string {
	if m.width <= 0 {
		return value
	}
	return lipgloss.NewStyle().MaxWidth(m.width).Render(value)
}

func (m Model) fitToTerminal(view string) string {
	if m.height <= 0 {
		return view
	}
	lines := strings.Split(view, "\n")
	if len(lines) <= m.height {
		return view
	}
	more := m.inline(dimStyle.Render("… resize terminal for more"))
	if m.height == 1 {
		return more
	}
	return strings.Join(append(lines[:m.height-1], more), "\n")
}

func (m Model) popupLimit() int {
	limit := 8
	switch {
	case m.height < 16:
		limit = 1
	case m.height < 22:
		limit = 3
	case m.height < 32:
		limit = 5
	}
	if m.width < 48 {
		limit = min(limit, 3)
	}
	return max(1, limit)
}

func (m Model) recentRunLimit() int {
	return max(1, min(6, max(1, m.height-6)/3))
}

func (m Model) visibleRange(total, selected, limit int) (int, int) {
	if total == 0 || limit <= 0 {
		return 0, 0
	}
	limit = min(limit, total)
	selected = min(max(0, selected), total-1)
	start := selected - limit/2
	start = max(0, min(start, total-limit))
	return start, start + limit
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
	defer m.persistDraft()
	defer m.refreshPreflight()
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
		m.notice = notice{text: "Choose a recommended model or type a model ID, then Tab back to the task.", kind: noticeInfo}
		return m, m.focusField()
	case "/provider":
		m.commandOutput = ""
		m.focus = providerField
		m.notice = notice{text: "Choose a provider or type to filter it, then Tab back to the task.", kind: noticeInfo}
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
	case "/recent":
		return m.openRecentRuns()
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
	start, end := m.visibleRange(len(matches), m.commandIndex, m.popupLimit())
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		command := matches[index]
		prefix := "  "
		if index == m.commandIndex {
			prefix = "> "
		}
		line := prefix + keyStyle.Render(command.name)
		if !m.compactLayout() {
			line += "  " + dimStyle.Render(command.description)
		}
		lines = append(lines, line)
	}
	return labelStyle.Render("Commands") + "\n" + m.panel(strings.Join(lines, "\n")) + "\n" + m.inline(dimStyle.Render("up/down choose · enter run command · esc dismiss"))
}

type dropdownOption struct {
	value       string
	label       string
	description string
	custom      bool
}

func providerDropdownOptions() []dropdownOption {
	descriptions := map[modelprovider.Provider]string{
		modelprovider.OpenAI:           "OpenAI Responses API",
		modelprovider.AzureOpenAI:      "Azure OpenAI Chat Completions",
		modelprovider.Anthropic:        "Anthropic Messages API",
		modelprovider.Gemini:           "Gemini GenerateContent API",
		modelprovider.Mistral:          "Mistral Chat Completions",
		modelprovider.XAI:              "xAI Chat Completions",
		modelprovider.Groq:             "Groq Chat Completions",
		modelprovider.OpenRouter:       "OpenRouter Chat Completions",
		modelprovider.Together:         "Together AI Chat Completions",
		modelprovider.Fireworks:        "Fireworks Chat Completions",
		modelprovider.DeepSeek:         "DeepSeek Chat Completions",
		modelprovider.OpenAICompatible: "custom Chat Completions endpoint",
		modelprovider.Codex:            "local Codex CLI subscription",
		modelprovider.Claude:           "local Claude Code subscription",
		modelprovider.Copilot:          "local GitHub Copilot CLI subscription",
		modelprovider.Cursor:           "local Cursor Agent CLI subscription",
	}
	options := make([]dropdownOption, 0, len(descriptions))
	for _, name := range modelprovider.Names() {
		provider, err := modelprovider.ParseProvider(name)
		if err != nil {
			continue
		}
		options = append(options, dropdownOption{value: name, description: descriptions[provider]})
	}
	return options
}

// modelDropdownOptions deliberately offers only model IDs Gator can recommend
// without guessing an arbitrary provider catalog. The text field remains
// editable for deployments, aliases, previews, and account-specific models.
func modelDropdownOptions(providerName string) []dropdownOption {
	provider, err := modelprovider.ParseProvider(providerName)
	if err != nil {
		return nil
	}
	customDescription := "type a model ID supported by this provider"
	if provider == modelprovider.AzureOpenAI {
		customDescription = "type the Azure deployment name"
	}
	if modelprovider.IsHarness(provider) {
		return []dropdownOption{
			{value: "", label: "provider default", description: "use the default configured in the vendor CLI"},
			{label: "custom model ID", description: "type a model selector supported by the vendor CLI", custom: true},
		}
	}
	options := []dropdownOption{{label: "custom model ID", description: customDescription, custom: true}}
	if defaultModel := modelprovider.DefaultModel(provider); defaultModel != "" {
		options = append([]dropdownOption{{value: defaultModel, label: defaultModel, description: "Gator recommended default"}}, options...)
	}
	return options
}

func matchingDropdownOptions(options []dropdownOption, query string) []dropdownOption {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return options
	}
	for _, option := range options {
		if option.custom {
			continue
		}
		if strings.EqualFold(option.value, query) {
			return options
		}
	}
	var matches []dropdownOption
	for _, option := range options {
		if option.custom {
			continue
		}
		if strings.HasPrefix(strings.ToLower(option.value), query) {
			matches = append(matches, option)
		}
	}
	for _, option := range options {
		if option.custom {
			matches = append(matches, option)
		}
	}
	return matches
}

func (m Model) dropdownOptions() []dropdownOption {
	switch m.focus {
	case providerField:
		return matchingDropdownOptions(providerDropdownOptions(), m.provider.Value())
	case modelField:
		return matchingDropdownOptions(modelDropdownOptions(m.provider.Value()), m.model.Value())
	default:
		return nil
	}
}

func (m Model) dropdownVisible() bool {
	return m.resumeStatePath == "" && (m.focus == providerField || m.focus == modelField) && len(m.dropdownOptions()) > 0
}

func (m *Model) normalizeDropdownSelection() {
	options := m.dropdownOptions()
	if len(options) == 0 {
		m.dropdownIndex = 0
		return
	}
	value := ""
	switch m.focus {
	case providerField:
		value = m.provider.Value()
	case modelField:
		value = m.model.Value()
	}
	for index, option := range options {
		if strings.EqualFold(option.value, strings.TrimSpace(value)) {
			m.dropdownIndex = index
			return
		}
	}
	if m.dropdownIndex >= len(options) {
		m.dropdownIndex = len(options) - 1
	}
}

func (m *Model) moveDropdownSelection(delta int) {
	options := m.dropdownOptions()
	if len(options) == 0 {
		m.dropdownIndex = 0
		return
	}
	m.dropdownIndex = (m.dropdownIndex + delta + len(options)) % len(options)
}

func (m *Model) applySelectedDropdown() {
	options := m.dropdownOptions()
	if len(options) == 0 {
		return
	}
	selected := options[m.dropdownIndex]
	switch m.focus {
	case providerField:
		m.provider.SetValue(selected.value)
		provider, err := modelprovider.ParseProvider(selected.value)
		if err == nil {
			m.model.SetValue(modelprovider.DefaultModel(provider))
		}
		m.notice = notice{text: "Provider selected: " + selected.value, kind: noticeInfo}
	case modelField:
		if selected.custom {
			if strings.TrimSpace(m.model.Value()) == "" {
				m.notice = notice{text: "Enter a model ID supported by the selected provider.", kind: noticeInfo}
			} else {
				m.notice = notice{text: "Custom model retained: " + strings.TrimSpace(m.model.Value()), kind: noticeInfo}
			}
			m.normalizeDropdownSelection()
			m.persistDraft()
			m.refreshPreflight()
			return
		}
		m.model.SetValue(selected.value)
		if selected.value == "" {
			m.notice = notice{text: "The provider CLI will choose its configured model.", kind: noticeInfo}
		} else {
			m.notice = notice{text: "Model selected: " + selected.value, kind: noticeInfo}
		}
	}
	m.normalizeDropdownSelection()
	m.persistDraft()
	m.refreshPreflight()
}

func (m Model) dropdownView() string {
	if !m.dropdownVisible() {
		return ""
	}
	options := m.dropdownOptions()
	start, end := m.visibleRange(len(options), m.dropdownIndex, m.popupLimit())
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		option := options[index]
		prefix := "  "
		if index == m.dropdownIndex {
			prefix = "> "
		}
		label := option.label
		if label == "" {
			label = option.value
		}
		if label == "" {
			label = "provider default"
		}
		line := prefix + keyStyle.Render(label)
		if !m.compactLayout() {
			line += "  " + dimStyle.Render(option.description)
		}
		lines = append(lines, line)
	}
	title := "Provider choices"
	if m.focus == modelField {
		title = "Recommended models"
	}
	return labelStyle.Render(title) + "\n" + m.panel(strings.Join(lines, "\n")) + "\n" + m.inline(dimStyle.Render("type to filter · up/down choose · enter apply · tab apply and continue"))
}

func (m *Model) normalizeContextSelection() {
	if _, active := activeContextCompletion(m.task.Value()); !active {
		m.contextIndex = 0
		return
	}
	if !m.contextLoaded {
		m.contextPaths, m.contextErr = contextCompletionCandidates(m.config.RepositoryPath)
		m.contextLoaded = true
	}
	matches := m.contextCompletions()
	if len(matches) == 0 {
		m.contextIndex = 0
		return
	}
	if m.contextIndex >= len(matches) {
		m.contextIndex = len(matches) - 1
	}
}

func (m Model) contextCompletions() []string {
	active, ok := activeContextCompletion(m.task.Value())
	if !ok || m.contextClosed || m.contextErr != nil {
		return nil
	}
	return matchingContextCompletions(m.contextPaths, active.query)
}

func (m Model) contextCompletionVisible() bool {
	return m.focus == taskField && len(m.contextCompletions()) > 0
}

func (m *Model) moveContextSelection(delta int) {
	matches := m.contextCompletions()
	if len(matches) == 0 {
		m.contextIndex = 0
		return
	}
	m.contextIndex = (m.contextIndex + delta + len(matches)) % len(matches)
}

func (m *Model) applySelectedContextCompletion() {
	active, ok := activeContextCompletion(m.task.Value())
	matches := m.contextCompletions()
	if !ok || len(matches) == 0 {
		return
	}
	selected := matches[m.contextIndex]
	m.task.SetValue(m.task.Value()[:active.start] + contextToken(selected) + " ")
	m.contextIndex = 0
	m.contextClosed = false
	m.persistDraft()
	m.refreshPreflight()
}

func (m Model) contextCompletionView() string {
	matches := m.contextCompletions()
	if len(matches) == 0 {
		return ""
	}
	start, end := m.visibleRange(len(matches), m.contextIndex, m.popupLimit())
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		candidate := matches[index]
		prefix := "  "
		if index == m.contextIndex {
			prefix = "> "
		}
		lines = append(lines, prefix+keyStyle.Render("["+contextBadge(candidate)+"]")+"  "+"@"+compact(candidate, max(8, m.panelTextWidth()-8)))
	}
	return labelStyle.Render("Path suggestions") + "\n" + m.panel(strings.Join(lines, "\n")) + "\n" + m.inline(dimStyle.Render("type to filter · up/down choose · enter or tab insert · esc dismiss"))
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
	return labelStyle.Render("Context references") + "\n" + m.panel(strings.Join(values, "  ")) + "\n" + m.inline(dimStyle.Render("Gator validates these paths before a run; the agent inspects them first."))
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
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	if limit == 1 {
		return "…"
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
