// Package worktui implements Gator's conversation-first Work terminal application.
package worktui

import (
	"context"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/delivery"
	"github.com/gongahkia/gator/internal/followup"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workhome"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

type RunResult struct {
	WorkID         string
	ConversationID string
	RevisionID     string
	SnapshotID     string
	FinalText      string
	OutputPath     string
	Bundle         BundleSummary
	Error          string
}

// BundleSummary is the display-safe, verified projection of a retained Work
// bundle. The TUI never trusts model prose to describe produced files.
type BundleSummary struct {
	Path        string
	Status      string
	Artifacts   []ArtifactSummary
	Candidates  []CandidateSummary
	RunID       string
	FinishedAt  string
	Sources     []string
	Connections []ConnectionUse
}

// ConnectionUse describes retained provenance, never current authorization.
type ConnectionUse struct {
	ID         string
	Operations []string
}

type ArtifactSummary struct {
	Path        string
	MediaType   string
	Bytes       int64
	Valid       bool
	Preview     string
	PreviewKind string
	Truncated   bool
}

type CandidateSummary struct {
	ID           string
	PatchPath    string
	Status       string
	ChangedPaths []string
}

// TranscriptMessage is a display-safe item in a restored Work conversation.
// A bundle is supplied as structured, verified metadata rather than as model
// prose, so historical artifact cards retain the same trust boundary as new
// cards produced in this TUI session.
type TranscriptMessage struct {
	Role   string
	Text   string
	Bundle *BundleSummary
}

// ConversationState is the projection the TUI needs to reopen one selected
// conversation revision. Messages must contain only the selected head's
// ancestry; sibling revisions belong to other retained branches.
type ConversationState struct {
	Title      string
	SourcePath string
	RevisionID string
	SnapshotID string
	OutputPath string
	LastBundle BundleSummary
	Messages   []TranscriptMessage
}

type BundleActionRequest struct {
	Action      string
	BundlePath  string
	Target      string
	CandidateID string
	DeliveryID  string
	Replace     bool
	Execute     bool
}

// RunOptions contains the user-selected orchestration controls that accompany
// one prompt. They configure Gator and bound its internal Code specialist;
// they are never editable by the manager model itself.
type RunOptions struct {
	Context                 context.Context
	OnEvent                 agent.EventSink
	Steering                <-chan string
	OnOperation             func(*workrun.Operation)
	MaxSteps                int
	Attachments             []string
	Mode                    string
	Artifacts               []string
	PreviousArtifacts       []string
	ConnectorIDs            []string
	WebOrigins              []string
	IgnoredInstructionPaths []string
	ExcludedKnowledgeIDs    []string
	RefreshSource           bool
	Code                    CodeOptions
}

type CodeOptions struct {
	MaxSteps               int
	Verification           []string
	Scopes                 []string
	Profile                string
	Setup                  []string
	AllowedCommands        []string
	AllowedCommandPrefixes []string
	Sandbox                string
	Network                string
	Capabilities           []string
	BrowserSession         string
}

// ModelStatus is display-safe configuration and authentication state for the
// currently selected model. Access must describe only the credential source,
// never credential material.
type ModelStatus struct {
	Provider string
	Model    string
	Access   string
}

type GoalState struct{ Title, SourcePath, ConversationID string }

type Config struct {
	Models                  func() (ModelPanel, error)
	SelectedModel           func() (provider, model string, err error)
	ModelStatus             func() (ModelStatus, error)
	Live                    bool
	BridgeMode              bool
	CurrentFolder           string
	GoalID                  string
	InitialTitle            string
	ResolveSource           func(path string) (string, error)
	ConnectorChoices        func() []string
	ConnectorAction         func(arguments []string) (string, error)
	ConnectorCommand        func(arguments []string) *exec.Cmd
	BundleAction            func(BundleActionRequest) (string, error)
	ListConversations       func() ([]worksession.Conversation, error)
	LoadConversation        func(conversationID string) (ConversationState, error)
	LoadConversationOptions func(conversationID string) (RunOptions, error)
	Conversations           []worksession.Conversation
	StartConversationID     string
	Jobs                    []jobs.Definition
	SupervisorRunning       bool
	Inbox                   []inbox.Entry
	Goals                   []goal.Record
	Attention               workhome.Attention
	RefreshAttention        func() (workhome.Attention, error)
	ReadOverview            func(context.Context) (workhome.Overview, error)
	ControlCommand          func([]string) *exec.Cmd
	ListGoals               func() ([]goal.Record, error)
	SelectGoal              func(id string) (GoalState, error)
	ClearGoal               func()
	MemoryAction            func(arguments []string) (string, error)
	MemoryResult            func(arguments []string) (*uisnapshot.Result, error)
	RecallStatus            func(context.Context) (RecallStatus, error)
	ConfigureRecall         func(provider string, allowUpload bool) error
	InstallRecall           func(context.Context, func(string)) error
	ReadContext             func(workID, goalID string) (ContextState, error)
	ChangeKnowledge         func(id, action, text, goalID string) error
	SaveFollowup            func(followup.Draft) (goal.Record, jobs.Definition, error)
	ReadChecks              func() ([]workhome.Check, bool, error)
	StartSupervisor         func() error
	Controls                func(context.Context, string, string, []string) (ControlScreen, error)
	BeforeLocalCommand      func() (func(), error)
	ResetPrivateState       func() (GoalState, error)
	PilotAction             func(workID string, arguments []string) (*uisnapshot.Result, error)
	PlanAction              func(command string) (string, error)
	PlanResult              func(command string) (*uisnapshot.Result, error)
	ActivityAction          func() (string, error)
	ActivityResult          func() (*uisnapshot.Result, error)
	NotificationAction      func(arguments []string) (string, error)
	SuggestionAction        func(arguments []string) (string, error)
	NoteAction              func(command string) (string, error)
	NoteResult              func(command string) (*uisnapshot.Result, error)
	SkillAction             func(command string) (string, error)
	SkillResult             func(command string) (*uisnapshot.Result, error)
	ReviewBundle            func(bundlePath string) (*uisnapshot.Result, error)
	// JobAction uses the same local Job service as the CLI. It keeps scheduling
	// as repeatable Work rather than creating a second TUI-only runtime.
	JobAction      func(arguments []string) (string, error)
	RefreshJobs    func() ([]jobs.Definition, error)
	InboxAction    func(arguments []string) (string, error)
	RefreshInbox   func() ([]inbox.Entry, error)
	Run            func(source, conversationID, prompt string, options RunOptions) RunResult
	MoveBack       func(conversationID string) (string, error)
	MoveForward    func(conversationID string) (string, error)
	MoveToRevision func(conversationID, revisionID string) (string, error)
	// The History, Delivery, Learning, and Session stores are the canonical
	// local product services. The TUI reads and mutates them directly; it never
	// owns duplicate display state or shells out to the CLI.
	HistoryStore  *workhistory.Store
	DeliveryStore *delivery.Store
	LearningStore *learning.Store
	SessionStore  *worksession.Store
	// FeedbackAction records explicit feedback against the current Work revision
	// using the same local service as the Work CLI.
	FeedbackAction func(workID string, arguments []string) (string, error)
	FirstRun       bool
	Inspect        func(topic string) (string, error)
	DoctorResult   func() (*uisnapshot.Result, error)
	Copy           func(text string) error
	Theme          string
	SetTheme       func(name string) error
	// StatusLine follows an ordered-item convention. The visible
	// status surface is separately opt-in so normal Work stays uncluttered.
	StatusLine        *[]string
	StatusLineEnabled bool
	SetStatusLine     func(enabled bool, items *[]string) error
}

type ModelPanel interface {
	tea.Model
	UISnapshot() uisnapshot.Panel
	Closed() bool
	Close()
}

type entry struct {
	title, subtitle, kind, id, source, command string
	advanced                                   bool
}
type message struct {
	role   string
	text   string
	bundle *BundleSummary
	result *uisnapshot.Result
}
type runDone RunResult
type bundleActionDone struct {
	action string
	text   string
	err    error
}
type loadingTickMsg struct{ run uint64 }

type connectorActionDone struct {
	action string
	id     string
	text   string
	err    error
}

type composerEditorDone struct {
	path   string
	editor string
	err    error
}

type queuedRun struct {
	id      uint64
	prompt  string
	options RunOptions
}

type pendingBundleAction struct {
	request BundleActionRequest
	summary string
}

type Model struct {
	models                 ModelPanel
	tasks                  map[string]string
	quitting               bool
	live                   <-chan tea.Msg
	cancel                 context.CancelFunc
	operation              *workrun.Operation
	interaction            *workrun.Interaction
	pendingInteractions    []workrun.Interaction
	config                 Config
	width                  int
	height                 int
	home                   bool
	overview               workhome.Overview
	overviewGeneration     uint64
	overviewBusy           bool
	overviewIssue          string
	controlReturnHome      bool
	launcher               bool
	launcherMode           string
	paletteQuery           string
	section                string
	selected               int
	entries                []entry
	source                 string
	goalID                 string
	conversation           string
	revision               string
	snapshot               string
	title                  string
	input                  string
	promptHistory          []string
	historyIndex           int
	historyDraft           string
	composerEditorPath     string
	bridgeExternal         *BridgeExternal
	messages               []message
	running                bool
	cancelling             bool
	status                 string
	firstRun               bool
	pendingPrompt          string
	scroll                 int
	options                RunOptions
	queue                  []queuedRun
	queueSequence          uint64
	bridgeQueueResult      *BridgeQueueResult
	lastOutput             string
	lastBundle             BundleSummary
	pendingBundleAction    *pendingBundleAction
	pendingConnectorCmd    tea.Cmd
	theme                  string
	loadingFrame           int
	loadingRun             uint64
	runStarted             time.Time
	modelStatus            ModelStatus
	bridgeNotice           string
	bridgeRecentIssue      string
	statusLine             []string
	statusLineConfigured   bool
	statusLineEnabled      bool
	statusLineDraft        []string
	statusLineDraftSet     bool
	statusLineDraftEnabled bool
	historyItems           []historyItem
	historyDetail          *historyDetail
	historyFilter          workhistory.Status
	learningItems          []learning.Record
	learningDetail         *learning.Record
	learningFilter         learning.Status
	learningForm           *learningForm
	sectionNotice          string
	productHelp            bool
	controlFamily          string
	controlRootArgs        []string
	controlScreen          ControlScreen
	controlForm            *ControlAction
	controlField           int
	controlReview          bool
	controlBusy            bool
	controlBackground      bool
	controlResultNotice    string
	controlCancel          context.CancelFunc
	controlGeneration      uint64
	followupForm           *followupForm
	followupSaving         bool
	checks                 []workhome.Check
	checkDetail            *workhome.Check
	recall                 RecallStatus
	recallBusy             bool
	recallCancel           context.CancelFunc
	recallEvents           <-chan tea.Msg
	recallConfirm          string
	contextState           ContextState
	contextDetail          *KnowledgeItem
	contextEdit            string
	contextEditing         bool
	contextConfirm         bool
	contextWorkID          string
	lastWorkID             string
}

func New(config Config) Model {
	// Keep the home useful for embedders that supply records directly without
	// the CLI's read-only attention projection.
	if config.Attention.ActiveGoals == 0 {
		for _, record := range config.Goals {
			if record.Status == goal.Active {
				config.Attention.ActiveGoals++
			}
		}
	}
	if config.Attention.UnreadResults == 0 {
		for _, item := range config.Inbox {
			if item.ReadAt.IsZero() {
				config.Attention.UnreadResults++
			}
		}
	}
	model := Model{
		config:   config,
		home:     true,
		firstRun: config.FirstRun,
		source:   config.CurrentFolder,
		goalID:   config.GoalID,
		title:    "Work in " + filepath.Base(config.CurrentFolder),
		theme:    normalizeTheme(config.Theme),
		options: RunOptions{MaxSteps: 24, Mode: "auto", Code: CodeOptions{
			MaxSteps: 16, Sandbox: "strict", Network: "deny",
		}},
	}
	model.entries = model.availableCommandEntries()
	model.statusLine, model.statusLineConfigured = resolveStatusLine(config.StatusLine)
	model.statusLineEnabled = config.StatusLineEnabled
	model.refreshModelStatus()
	if config.StartConversationID != "" {
		found := false
		for _, conversation := range config.Conversations {
			if conversation.ID == config.StartConversationID {
				found = true
				model.home = false
				model.source = conversation.SourcePath
				model.conversation = conversation.ID
				model.title = conversation.Title
				model.restoreConversation(conversation.ID, "")
				break
			}
		}
		if !found && config.LoadConversation != nil {
			// The interactive entrypoint normally preloads a requested ID into the
			// picker, but the TUI remains correct for other callers as well.
			model.home = false
			model.conversation = config.StartConversationID
			model.title = "Restoring conversation"
			model.restoreConversation(config.StartConversationID, "")
		}
	}
	if config.InitialTitle != "" {
		model.home = false
		model.title = config.InitialTitle
	}
	return model
}

// Init forces one redraw after the alternate screen has been entered. Some
// terminal emulators otherwise retain cells from the previous buffer until
// their first viewport-sized repaint.
func (m Model) Init() tea.Cmd {
	if m.config.ReadChecks != nil {
		return tea.Batch(tea.ClearScreen, m.refreshChecksCommand(), m.readOverviewCommand(), nextChecksTick())
	}
	return tea.Batch(tea.ClearScreen, m.readOverviewCommand())
}
