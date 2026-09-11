package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/gongahkia/gator/internal/diffview"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/lsp"
	"github.com/gongahkia/gator/internal/review"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/terminal"
)

// Model is the Bubble Tea state model for Gator's terminal experience.
type Model struct {
	config      Config
	catalogOnly bool

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
	terminalInput       textinput.Model
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
	copyToClipboard     func(string) error
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
	browserSession      string
	updateChecking      bool
	cancelling          bool
	lastRunCancelled    bool
	activity            runActivity
	verificationStatus  []verificationStatus
	terminalAttachment  terminal.Attachment
	terminalRegistry    *terminal.Registry
	lspRegistry         *lsp.Registry
	terminalTasks       []terminal.Task
	terminalViews       map[string]attachedTerminalView
	terminalIndex       int
	terminalRawInput    bool
	terminalErr         error
	terminalReturn      screen

	outcome           *gatorrun.Outcome
	runErr            error
	diff              string
	focusedDiff       string
	focusedDiffInfo   diffview.Focused
	diffMode          diffDisplayMode
	diffOffset        int
	diffTruncated     bool
	diffErr           error
	diffStats         diffStats
	reviewSnapshot    review.Snapshot
	reviewLoaded      bool
	reviewScope       review.Scope
	reviewPane        reviewPane
	reviewFileIndex   int
	reviewHunkIndex   int
	reviewLineIndex   int
	reviewRangeFrom   int
	reviewRawFiles    map[string]bool
	reviewMutation    *reviewMutation
	reviewRequest     textarea.Model
	reviewRequestOn   bool
	resumeStatePath   string
	forkStatePath     string
	forceCompaction   bool
	threadID          string
	runMode           gatorrun.Mode
	effort            effortLevel
	effortIndex       int
	extensionUIIndex  int
	extensionUIReturn screen
	localModels       localModelsState
	management        managementState
	doctor            doctorState
	runOptions        runOptionsState
	reviewWeb         reviewWebState
	recentTarget      textinput.Model
}
