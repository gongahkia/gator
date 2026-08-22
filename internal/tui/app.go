// Package tui provides Gator's interactive terminal application.
package tui

import (
	"context"
	"crypto/sha256"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/journal"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/tools"
)

const defaultMaxSteps = 24
const maxQueuedInputs = 16

// Config supplies the local configuration and provider factory for an
// interactive session. NewExecutor is injected so the UI stays independent of
// any particular model provider and can be tested without a network request.
type Config struct {
	RepositoryPath    string
	Provider          string
	Model             string
	BaseURL           string
	Verification      [][]string
	MaxSteps          int
	StateDir          string
	ResumeStatePath   string
	ForkStatePath     string
	StartInRecent     bool
	RecentAll         bool
	CustomProviders   []config.CustomProvider
	ExtensionCommands []ExtensionCommand
	Theme             string
	NewExecutor       func(provider, model, baseURL string) (gatorrun.Executor, error)
	BeginOAuthLogin   func(provider string) (OAuthLogin, error)
	NewConnectCommand func(provider string) (*exec.Cmd, error)
	// NewDelegateCommand starts a vendor-owned harness in a fresh isolated
	// worktree. It deliberately remains separate from NewExecutor: the harness
	// owns its credential, tools, approvals, and session state.
	NewDelegateCommand func(runtime, task, model string, verification [][]string, repository string) (DelegateCommand, error)
	SetTheme           func(name string) error
}

// ExtensionCommand is a visible prompt template contributed by a trusted or
// explicitly installed extension. Selecting it only fills the composer.
type ExtensionCommand struct {
	Name        string
	Description string
	Prompt      string
}

// DelegateCommand keeps the process and a bounded caller-owned view of its
// terminal output together. The latter lets the TUI explain a non-zero exit
// without attempting to interpret a vendor CLI's private session state.
type DelegateCommand struct {
	Process *exec.Cmd
	Output  func() string
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

type drawerSection uint8

const (
	drawerThreads drawerSection = iota
	drawerRuntime
	drawerActivity
	drawerReview
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
	isError   bool
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

type commandApprovalRequest struct {
	argv  []string
	reply chan tools.CommandDecision
}

type pendingCommandApproval struct {
	argv  []string
	reply chan tools.CommandDecision
}

type executionStream struct {
	events    chan agent.Event
	done      chan executionDone
	cancel    context.CancelFunc
	steering  chan string
	approvals chan commandApprovalRequest
}

type executionDone struct {
	outcome gatorrun.Outcome
	err     error
}

type agentEventMsg struct {
	event agent.Event
}

type commandApprovalMsg struct {
	argv  []string
	reply chan tools.CommandDecision
}

type executionDoneMsg struct {
	done executionDone
}

type oauthLoginDoneMsg struct {
	provider string
	err      error
}

type connectDoneMsg struct {
	provider string
	err      error
}

type delegatedRunDoneMsg struct {
	runtime string
	output  string
	err     error
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
	vimCount            int
	vimPending          string
	vimPendingCount     int
	vimRegister         vimRegister
	vimUndo             []vimSnapshot
	vimRedo             []vimSnapshot
	vimAbsoluteNumbers  bool
	vimRelativeNumbers  bool
	vimNumberState      *vimLineNumberState
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
	transcript          viewport.Model
	followTranscript    bool
	transcriptUnread    bool
	drawerOpen          bool
	drawerSection       drawerSection
	drawerIndex         int
	queue               []queuedInput
	pendingApproval     *pendingCommandApproval
	execution           *executionStream
	oauthLogin          OAuthLogin
	oauthCancel         context.CancelFunc
	oauthProvider       string
	delegateRuntime     string
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
	panelStyle  = lipgloss.NewStyle().Padding(0, 1)
)

// applyTheme keeps terminal customization intentionally recognizable: every
// saved choice has a stable name and no hidden style file is evaluated.
func applyTheme(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "contrast":
		headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
		labelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
		dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
		keyStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("226"))
		errorStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
		okStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("46"))
		panelStyle = lipgloss.NewStyle().Padding(0, 1)
		return "contrast"
	case "mono":
		headerStyle = lipgloss.NewStyle().Bold(true)
		labelStyle = lipgloss.NewStyle().Bold(true)
		dimStyle = lipgloss.NewStyle().Faint(true)
		keyStyle = lipgloss.NewStyle().Bold(true)
		errorStyle = lipgloss.NewStyle().Bold(true)
		okStyle = lipgloss.NewStyle().Bold(true)
		panelStyle = lipgloss.NewStyle().Padding(0, 1)
		return "mono"
	default:
		headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
		labelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
		dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
		keyStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
		errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
		okStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
		panelStyle = lipgloss.NewStyle().Padding(0, 1)
		return "gator"
	}
}

// New creates a terminal model in task-composition mode.
func New(config Config) Model {
	config.Theme = applyTheme(config.Theme)
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
		config:           config,
		screen:           composeScreen,
		recentAll:        config.RecentAll,
		task:             task,
		verification:     verification,
		provider:         provider,
		model:            model,
		transcript:       viewport.New(76, 8),
		followTranscript: true,
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
		m.syncTranscript(m.followTranscript)
	case commandApprovalMsg:
		m.pendingApproval = &pendingCommandApproval{argv: append([]string(nil), msg.argv...), reply: msg.reply}
		m.notice = notice{text: "Approve worktree command: " + strings.Join(msg.argv, " "), kind: noticeInfo}
		m.setActivity(activityAwaitingApproval, "awaiting command approval", time.Now())
		return m, waitForExecution(m.execution)
	case agentEventMsg:
		m.appendEvent(msg.event)
		return m, waitForExecution(m.execution)
	case activityTickMsg:
		if m.screen == runningScreen && m.execution != nil {
			return m, waitForExecution(m.execution)
		}
		return m, nil
	case executionDoneMsg:
		m.resolvePendingApproval(tools.CommandDeny)
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
	case connectDoneMsg:
		if msg.err != nil {
			m.notice = notice{text: "Provider-owned login stopped: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		runtime := delegatedRuntimeForProvider(msg.provider)
		if runtime == "" || m.config.NewDelegateCommand == nil {
			m.commandOutput = "Provider-owned login completed for " + msg.provider + ". Its credential remains in that vendor CLI's store. Use the matching gator delegate runtime for harness-owned runs."
			m.notice = notice{text: "Provider-owned login completed. See the next action below.", kind: noticeSuccess}
			return m, nil
		}
		m.returnToComposer()
		m.delegateRuntime = runtime
		m.provider.SetValue(msg.provider)
		if _, modelName, _, err := m.resolveProviderAndModel(msg.provider, ""); err == nil {
			m.model.SetValue(modelName)
		}
		m.commandOutput = delegatedRuntimeLabel(runtime) + " is ready. Send a task to run it in a fresh isolated worktree. The vendor CLI keeps its own login, tools, approvals, and session state."
		m.notice = notice{text: "Provider-owned login completed. Harness mode is ready for the next task.", kind: noticeSuccess}
		m.persistDraft()
		m.refreshPreflight()
		return m, nil
	case delegatedRunDoneMsg:
		m.task.Reset()
		m.task.Placeholder = "Send another delegated task..."
		m.focus = taskField
		_ = m.focusField()
		if msg.err != nil {
			m.commandOutput = delegatedRuntimeLabel(msg.runtime) + " stopped."
			if output := delegatedOutputTail(msg.output); output != "" {
				m.commandOutput += "\n\nTerminal output:\n" + output
			}
			m.notice = notice{text: "Delegated run stopped: " + msg.err.Error(), kind: noticeError}
		} else {
			m.commandOutput = delegatedRuntimeLabel(msg.runtime) + " completed. Review its retained worktree before applying changes."
			if output := delegatedOutputTail(msg.output); output != "" {
				m.commandOutput += "\n\nTerminal output:\n" + output
			}
			m.notice = notice{text: "Delegated run complete. Review the retained worktree before applying changes.", kind: noticeSuccess}
			if strings.TrimSpace(m.config.StateDir) != "" {
				if err := journal.DeleteDraft(m.config.StateDir, m.config.RepositoryPath); err != nil {
					m.draftErr = err
				}
			}
		}
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

func delegatedOutputTail(value string) string {
	const limit = 6 * 1024
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return "… earlier terminal output omitted …\n" + value[len(value)-limit:]
}
