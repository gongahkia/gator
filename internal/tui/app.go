// Package tui provides Gator's interactive terminal application.
package tui

import (
	"context"
	"crypto/sha256"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/journal"
	gatorrun "github.com/gongahkia/gator/internal/run"
)

const defaultMaxSteps = 24
const maxQueuedInputs = 16

// Config supplies the local configuration and provider factory for an
// interactive session. NewExecutor is injected so the UI stays independent of
// any particular model provider and can be tested without a network request.
type Config struct {
	RepositoryPath  string
	Provider        string
	Model           string
	BaseURL         string
	Verification    [][]string
	MaxSteps        int
	StateDir        string
	ResumeStatePath string
	ForkStatePath   string
	StartInRecent   bool
	RecentAll       bool
	NewExecutor     func(provider, model, baseURL string) (gatorrun.Executor, error)
	BeginOAuthLogin func(provider string) (OAuthLogin, error)
}

// OAuthLogin is an application-owned browser login that has already started a
// local callback listener. The TUI displays its URL before it waits, so the
// user can complete or cancel authentication without a hidden subprocess.
type OAuthLogin interface {
	URL() string
	Complete(context.Context) error
	Cancel()
}

type screen uint8

const (
	composeScreen screen = iota
	attachmentConfirmScreen
	runningScreen
	reviewScreen
	transcriptScreen
	helpScreen
	recentScreen
	threadScreen
)

type field uint8

const (
	taskField field = iota
	verificationField
	providerField
	modelField
)

type vimMode uint8

const (
	vimOff vimMode = iota
	vimNormal
	vimInsert
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
	step   int
	text   string
	detail string
	kind   agent.EventKind
}

type chatAuthor uint8

const (
	chatSystem chatAuthor = iota
	chatUser
	chatAgent
	chatTool
)

// chatEntry is local to the active TUI. The durable session remains the
// source of truth for resume; this is the readable terminal conversation.
type chatEntry struct {
	author    chatAuthor
	text      string
	detail    string
	streaming bool
}

type queuedInputKind uint8

const (
	queuedPrompt queuedInputKind = iota
	queuedCommand
)

// queuedInput is intentionally TUI-local. A queued instruction is not part of
// a retained session until it actually begins a run.
type queuedInput struct {
	kind queuedInputKind
	text string
}

type attachmentPreview struct {
	name      string
	mediaType string
	bytes     int
	digest    [sha256.Size]byte
}

type executionStream struct {
	events   chan agent.Event
	done     chan executionDone
	cancel   context.CancelFunc
	steering chan string
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

type oauthLoginDoneMsg struct {
	provider string
	err      error
}

type diffLoadedMsg struct {
	diff      string
	truncated bool
	err       error
}

type activityTickMsg struct{}

// Model is the Bubble Tea state model for Gator's terminal experience.
type Model struct {
	config Config

	screen              screen
	focus               field
	vim                 vimMode
	vimCommand          string
	quitAfterRun        bool
	width               int
	height              int
	task                textarea.Model
	verification        textarea.Model
	provider            textinput.Model
	model               textinput.Model
	notice              notice
	commandOutput       string
	commandIndex        int
	dropdownIndex       int
	contextIndex        int
	contextPaths        []string
	contextLoaded       bool
	contextErr          error
	contextClosed       bool
	attachmentPreview   []attachmentPreview
	attachmentConfirmed bool
	preflight           []string
	draftErr            error
	helpReturn          screen
	transcriptReturn    screen
	threadReturn        screen
	transcriptIndex     int
	recentThreads       []journal.RecentThread
	recentIndex         int
	recentAll           bool
	threadTurns         []journal.ThreadTurn
	threadForks         map[string][]journal.RecentThread
	threadIndex         int
	events              []timelineEntry
	chat                []chatEntry
	chatIndex           int
	queue               []queuedInput
	execution           *executionStream
	oauthLogin          OAuthLogin
	oauthCancel         context.CancelFunc
	oauthProvider       string
	cancelling          bool
	lastRunCancelled    bool
	activity            runActivity
	verificationStatus  []verificationStatus

	outcome         *gatorrun.Outcome
	runErr          error
	diff            string
	diffTruncated   bool
	diffErr         error
	diffStats       diffStats
	resumeStatePath string
	forkStatePath   string
	forceCompaction bool
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
	task.Placeholder = "Message Gator..."
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
		recentAll:    config.RecentAll,
		task:         task,
		verification: verification,
		provider:     provider,
		model:        model,
		notice: notice{
			text: "Gator works in an isolated Git worktree. Review remains explicit.",
			kind: noticeInfo,
		},
	}
	application.appendChat(chatEntry{author: chatSystem, text: "New isolated thread. Gator works in a separate Git worktree; use /permissions for the active policy."})
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
	if strings.TrimSpace(config.ResumeStatePath) != "" {
		next, _ := application.beginContinuation(config.ResumeStatePath)
		return next.(Model)
	}
	if strings.TrimSpace(config.ForkStatePath) != "" {
		next, _ := application.beginFork(config.ForkStatePath)
		return next.(Model)
	}
	if config.StartInRecent {
		next, _ := application.openRecentRuns()
		return next.(Model)
	}
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
		m.appendEvent(msg.event)
		return m, waitForExecution(m.execution)
	case activityTickMsg:
		if m.screen == runningScreen && m.execution != nil {
			return m, waitForExecution(m.execution)
		}
		return m, nil
	case executionDoneMsg:
		wasNewThread := m.resumeStatePath == ""
		wasCancelling := m.cancelling
		m.execution = nil
		m.cancelling = false
		m.lastRunCancelled = wasCancelling
		m.outcome = &msg.done.outcome
		m.threadID = msg.done.outcome.ThreadID
		if msg.done.outcome.StatePath != "" {
			m.resumeStatePath = msg.done.outcome.StatePath
			m.forkStatePath = ""
		}
		m.runErr = msg.done.err
		m.finishRunActivity(msg.done.err, wasCancelling)
		m.appendCompletion(msg.done.outcome, msg.done.err)
		m.screen = composeScreen
		m.task.Reset()
		m.task.Placeholder = "Send a follow-up..."
		m.focus = taskField
		_ = m.focusField()
		m.refreshPreflight()
		if msg.done.err != nil {
			m.notice = notice{text: m.runMode.String() + " stopped: " + msg.done.err.Error(), kind: noticeError}
		} else {
			if m.runMode == gatorrun.PlanMode {
				m.notice = notice{text: "Plan ready. Continue this thread in Execute mode when you are ready to make changes.", kind: noticeSuccess}
			} else {
				m.notice = notice{text: "Run complete. Inspect the diff and evidence before applying anything.", kind: noticeSuccess}
			}
		}
		if msg.done.outcome.StatePath != "" && wasNewThread && strings.TrimSpace(m.config.StateDir) != "" {
			if err := journal.DeleteDraft(m.config.StateDir, m.config.RepositoryPath); err != nil {
				m.draftErr = err
			}
		}
		if msg.done.err == nil && !wasCancelling && len(m.queue) > 0 {
			m.notice = notice{text: "Previous run completed. Starting the next queued instruction...", kind: noticeInfo}
			queued, command, dispatched := m.dispatchNextQueued()
			if dispatched {
				return queued, command
			}
			m = queued.(Model)
		} else if len(m.queue) > 0 {
			if wasCancelling {
				m.notice = notice{text: "Run cancelled. " + m.queueSummary() + " retained; use /queue to inspect it.", kind: noticeInfo}
			} else {
				m.notice = notice{text: "Run stopped. " + m.queueSummary() + " retained; use /queue to inspect it.", kind: noticeError}
			}
		}
		if m.quitAfterRun && msg.done.err == nil && !wasCancelling && len(m.queue) == 0 {
			m.quitAfterRun = false
			return m, tea.Quit
		}
		if msg.done.err != nil || wasCancelling {
			m.quitAfterRun = false
		}
		if msg.done.outcome.Worktree.Path == "" {
			return m, nil
		}
		return m, loadDiff(msg.done.outcome.Worktree.Root)
	case oauthLoginDoneMsg:
		if msg.provider != m.oauthProvider {
			return m, nil
		}
		m.oauthLogin = nil
		m.oauthCancel = nil
		m.oauthProvider = ""
		if msg.err != nil {
			m.notice = notice{text: "OAuth login stopped: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		m.commandOutput = "Signed in to " + msg.provider + "."
		m.notice = notice{text: "OAuth credential stored. The provider is ready for a Gator-owned run.", kind: noticeSuccess}
		m.refreshPreflight()
		return m, nil
	case diffLoadedMsg:
		m.diff, m.diffTruncated, m.diffErr = msg.diff, msg.truncated, msg.err
		if msg.err == nil {
			m.diffStats = summarizeDiff(msg.diff)
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}
