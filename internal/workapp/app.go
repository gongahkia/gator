// Package workapp owns Gator's conversation application state and display-safe
// projections. The desktop host runs its message loop without a terminal renderer.
package workapp

import (
	"context"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
	appconfig "github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/delivery"
	"github.com/gongahkia/gator/internal/followup"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workactivity"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workhome"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

type RunResult struct {
	AppScreen       *ControlScreen
	AppScreenFamily string
	AppScreenAction string
	AppChanges      []workrun.AppChange
	Activity        *workactivity.Trace
	WorkID          string
	ConversationID  string
	RevisionID      string
	SnapshotID      string
	FinalText       string
	OutputPath      string
	Bundle          BundleSummary
	Error           string
}

// BundleSummary is the display-safe, verified projection of a retained Work
// bundle. The application never trusts model prose to describe produced files.
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
	SHA256      string
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
// cards produced in this application session.
type TranscriptMessage struct {
	CreatedAt       time.Time
	ReplyToUpdateID string
	InputIDs        []string
	Inputs          []string
	Activity        *workactivity.Trace
	Role            string
	Text            string
	Bundle          *BundleSummary
}

// ConversationState is the projection the application needs to reopen one selected
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
	ReplyToUpdateID         string
	ModelProvider           string
	ModelName               string
	Context                 context.Context          `json:"-"`
	OnEvent                 agent.EventSink          `json:"-"`
	Steering                <-chan string            `json:"-"`
	OnOperation             func(*workrun.Operation) `json:"-"`
	MaxSteps                int
	Attachments             []string
	Uploads                 []string
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
	SaveConnections          func(string, []string) error
	Desktop                  appconfig.DesktopSettings
	ReadDesktop              func() (appconfig.DesktopSettings, error)
	ValidateUploads          func(source string, paths, uploads []string) error
	Models                   func() (ModelPanel, error)
	ComposerModels           func() ([]ComposerModel, error)
	SelectComposerModel      func(string, string) error
	SelectedModel            func() (provider, model string, err error)
	ModelStatus              func() (ModelStatus, error)
	Live                     bool
	BridgeMode               bool
	ManagedSessions          bool
	StartNew                 bool
	StartView                bool
	ViewOriginConversationID string
	CurrentFolder            string
	GoalID                   string
	InitialTitle             string
	ResolveSource            func(path string) (string, error)
	ConnectorChoices         func() []string
	ConnectorAction          func(arguments []string) (string, error)
	ConnectorCommand         func(arguments []string) *exec.Cmd
	BundleAction             func(BundleActionRequest) (string, error)
	ListConversations        func() ([]worksession.Conversation, error)
	LoadConversation         func(conversationID string) (ConversationState, error)
	LoadConversationOptions  func(conversationID string) (RunOptions, error)
	Conversations            []worksession.Conversation
	StartConversationID      string
	LastConversation         func() (string, error)
	RememberConversation     func(string) error
	CreateConversation       func(string) (worksession.Conversation, error)
	PrimaryConversation      func() (string, error)
	BindPrimaryConversation  func(string) error
	Jobs                     []jobs.Definition
	SupervisorRunning        bool
	Inbox                    []inbox.Entry
	InboxStore               *inbox.Store
	Goals                    []goal.Record
	Attention                workhome.Attention
	RefreshAttention         func() (workhome.Attention, error)
	ReadOverview             func(context.Context) (workhome.Overview, error)
	ControlCommand           func([]string) *exec.Cmd
	ListGoals                func() ([]goal.Record, error)
	SelectGoal               func(id string) (GoalState, error)
	ClearGoal                func()
	MemoryAction             func(arguments []string) (string, error)
	MemoryResult             func(arguments []string) (*uisnapshot.Result, error)
	RecallStatus             func(context.Context) (RecallStatus, error)
	ConfigureChatLearning    func(bool) error
	ConfigureChatHistory     func(bool) error
	ConfigureRecall          func(provider string, allowUpload bool) error
	InstallRecall            func(context.Context, func(string)) error
	ReadContext              func(workID, goalID string) (ContextState, error)
	ChangeKnowledge          func(id, action, text, goalID string) error
	SaveFollowup             func(followup.Draft) (goal.Record, jobs.Definition, error)
	ReadChecks               func() ([]workhome.Check, bool, error)
	StartSupervisor          func() error
	Controls                 func(context.Context, string, string, []string) (ControlScreen, error)
	PendingConnection        func() string
	BeforeLocalCommand       func() (func(), error)
	ResetPrivateState        func() (GoalState, error)
	PilotAction              func(workID string, arguments []string) (*uisnapshot.Result, error)
	PlanAction               func(command string) (string, error)
	PlanResult               func(command string) (*uisnapshot.Result, error)
	ActivityAction           func() (string, error)
	ActivityResult           func() (*uisnapshot.Result, error)
	NotificationAction       func(arguments []string) (string, error)
	SuggestionAction         func(arguments []string) (string, error)
	NoteAction               func(command string) (string, error)
	NoteResult               func(command string) (*uisnapshot.Result, error)
	SkillAction              func(command string) (string, error)
	SkillResult              func(command string) (*uisnapshot.Result, error)
	ReviewBundle             func(bundlePath string) (*uisnapshot.Result, error)
	// JobAction uses the canonical local Job service. It keeps scheduling
	// as repeatable Work rather than creating a second interface-specific runtime.
	JobAction      func(arguments []string) (string, error)
	RefreshJobs    func() ([]jobs.Definition, error)
	InboxAction    func(arguments []string) (string, error)
	RefreshInbox   func() ([]inbox.Entry, error)
	Run            func(source, conversationID, prompt string, options RunOptions) RunResult
	MoveBack       func(conversationID string) (string, error)
	MoveForward    func(conversationID string) (string, error)
	MoveToRevision func(conversationID, revisionID string) (string, error)
	// The History, Delivery, Learning, and Session stores are the canonical
	// local product services. The application reads and mutates them directly; it never
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
	createdAt       time.Time
	replyToUpdateID string
	inputIDs        []string
	inputs          []string
	activity        *workactivity.Trace
	role            string
	text            string
	bundle          *BundleSummary
	result          *uisnapshot.Result
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
	id          uint64
	prompt      string
	options     RunOptions
	requestID   string
	interrupted bool
}

type pendingBundleAction struct {
	request BundleActionRequest
	summary string
}

type Model struct {
	chatUpdates            []inbox.Entry
	chatUpdatesIssue       string
	chatUpdatesTotal       int
	updateConversations    []BridgeUpdateConversation
	updatePageChat         string
	updatePageAnchors      []string
	updatesHaveOlder       bool
	submissionReply        string
	background             bool
	sessionRequest         *BridgeSessionRequest
	sessionSequence        uint64
	desktop                appconfig.DesktopSettings
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
	requests               worksession.RequestJournal
	activeRequest          string
	nextRequest            string
	submissionID           string
	requestsRecovered      bool
	requestsIssue          string
	bridgeQueueResult      *BridgeQueueResult
	lastOutput             string
	lastBundle             BundleSummary
	pendingBundleAction    *pendingBundleAction
	pendingConnectorCmd    tea.Cmd
	theme                  string
	loadingFrame           int
	loadingRun             uint64
	runStarted             time.Time
	activity               *workactivity.Trace
	modelStatus            ModelStatus
	composerModels         []ComposerModel
	composerModelsIssue    string
	composerResult         *ComposerResult
	bridgeNotice           string
	bridgeUploadResult     *BridgeUploadResult
	bridgeRecentIssue      string
	bridgeFollowingIssue   string
	checksIssue            string
	statusLine             []string
	statusLineConfigured   bool
	statusLineEnabled      bool
	statusLineDraft        []string
	statusLineDraftSet     bool
	statusLineDraftEnabled bool
	historyItems           []historyItem
	historyDetail          *historyDetail
	historyFilter          workhistory.Status
	inboxDetail            string
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
	connectionPolling      bool
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
	primary                bool
	primaryID              string
	primarySource          string
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
		background: config.ManagedSessions,
		desktop:    config.Desktop,
		config:     config,
		home:       true,
		firstRun:   config.FirstRun,
		source:     config.CurrentFolder,
		goalID:     config.GoalID,
		title:      "Work in " + filepath.Base(config.CurrentFolder),
		theme:      normalizeTheme(config.Theme),
		options: RunOptions{MaxSteps: 24, Mode: "auto", Code: CodeOptions{
			MaxSteps: 16, Sandbox: "strict", Network: "deny",
		}},
	}
	model.entries = model.availableCommandEntries()
	model.statusLine, model.statusLineConfigured = resolveStatusLine(config.StatusLine)
	model.statusLineEnabled = config.StatusLineEnabled
	model.refreshModelStatus()
	model.primarySource = config.CurrentFolder
	if !config.StartNew && !config.StartView && config.LastConversation != nil && config.StartConversationID == "" && config.GoalID == "" {
		id, err := config.LastConversation()
		if err != nil {
			model.bridgeNotice = "Couldn't reopen your last chat: " + err.Error()
		} else {
			config.StartConversationID = id
		}
	}
	if config.PrimaryConversation != nil {
		id, err := config.PrimaryConversation()
		model.primaryID = id
		if err != nil {
			model.bridgeNotice = "Couldn't open your main conversation: " + err.Error()
		} else if config.StartConversationID == "" && config.GoalID == "" {
			model.primary = true
			model.title = "Gator"
			config.StartConversationID = id
		} else {
			model.primary = id != "" && config.StartConversationID == id && config.GoalID == ""
		}
	}
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
			// picker, but the application remains correct for other callers as well.
			model.home = false
			model.conversation = config.StartConversationID
			model.title = "Restoring conversation"
			model.restoreConversation(config.StartConversationID, "")
		}
	}
	if !config.StartView && model.conversation == "" && config.GoalID == "" && config.CreateConversation != nil && model.bridgeNotice == "" {
		model.createChat()
	}
	if config.InitialTitle != "" {
		model.home = false
		model.title = config.InitialTitle
	}
	model.refreshChatUpdates()
	return model
}

// Init loads the initial application projections. The desktop owns rendering.
func (m Model) Init() tea.Cmd {
	if m.config.ReadChecks != nil {
		return tea.Batch(m.refreshChecksCommand(), m.readOverviewCommand(), nextChecksTick())
	}
	return m.readOverviewCommand()
}
