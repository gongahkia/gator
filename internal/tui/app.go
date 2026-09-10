// Package tui provides Gator's interactive terminal application.
package tui

import (
	"context"
	"crypto/sha256"
	"os/exec"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gongahkia/gator/internal/agent"
	gatorbrowser "github.com/gongahkia/gator/internal/browser"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/diffview"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/lsp"
	"github.com/gongahkia/gator/internal/rattles"
	"github.com/gongahkia/gator/internal/review"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/terminal"
	"github.com/gongahkia/gator/internal/tools"
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
	CustomProviders []config.CustomProvider
	// ProviderEndpoints contains non-secret endpoint overrides keyed by
	// provider. It is persisted by SaveCloudModel rather than in drafts.
	ProviderEndpoints map[string]string
	// ProviderOptions contains provider-specific non-secret configuration,
	// keyed first by provider and then by option name.
	ProviderOptions   map[string]map[string]string
	ModelAliases      map[string]string
	ExtensionCommands []ExtensionCommand
	ExtensionUI       []ExtensionUIContribution
	Theme             string
	Execution         sandbox.Policy
	// Effort is an optional intent-level default. It changes Gator's bounded
	// agent-turn budget; it does not silently pick a provider or model.
	Effort            string
	NewExecutor       func(provider, model, baseURL string) (gatorrun.Executor, error)
	BeginOAuthLogin   func(provider string) (OAuthLogin, error)
	NewConnectCommand func(provider string) (*exec.Cmd, error)
	// TerminalRegistry retains explicitly approved background terminal tasks for
	// this TUI process. A nil value creates a private registry.
	TerminalRegistry *terminal.Registry
	// LSPRegistry retains trusted language-server processes for compatible
	// resumed worktree runs in this TUI process. A nil value creates a private
	// registry.
	LSPRegistry *lsp.Registry
	// NewDelegateCommand starts a vendor-owned harness in a fresh isolated
	// worktree. It deliberately remains separate from NewExecutor: the harness
	// owns its credential, tools, approvals, and session state.
	NewDelegateCommand func(runtime, task, model string, verification [][]string, repository string) (DelegateCommand, error)
	// NewOpenCodeCommand runs a narrowly scoped OpenCode management command in
	// the developer's terminal. It is separate from a delegated run because
	// login and status remain owned by the installed OpenCode CLI.
	NewOpenCodeCommand func(arguments []string) (*exec.Cmd, error)
	// Browser owns explicit local browser-session lifecycle and is injected by
	// the command layer so the TUI never starts a hidden remote service.
	Browser BrowserBackend
	// Build identifies this running binary without performing network I/O.
	Build BuildInfo
	// CheckForUpdate performs a check-only update lookup. The TUI never
	// self-replaces while it owns the terminal process.
	CheckForUpdate func() (UpdateStatus, error)
	// CopyToClipboard writes text to the operating system clipboard. A nil
	// value uses Gator's platform clipboard integration.
	CopyToClipboard func(string) error
	// OpenBrowser opens a loopback review URL. A nil value uses the platform opener.
	OpenBrowser func(string) error
	// SaveCloudModel persists a cloud model's non-secret configuration and, when
	// supplied, its masked credential in Gator's private auth store.
	SaveCloudModel     func(CloudModelSetup) error
	SaveModelSelection func(provider, model string) error
	SetTheme           func(name string) error
	// LocalModels manages Gator's reviewed, loopback-only local model catalog.
	// It is injected from the command layer so the UI does not own runtime
	// configuration or make network requests on its event loop.
	LocalModels LocalModelManager
	// Management exposes inspect-first project trust and retained-artifact
	// operations. The command layer owns filesystem/config mutation; the TUI
	// only renders typed state and asks for explicit confirmation.
	Management ManagementBackend
	// Doctor is a get-only local diagnostic snapshot. It must never start
	// services, open OAuth, or mutate configuration.
	Doctor DoctorBackend
	// ModelManagement persists cloud credentials and custom-provider metadata.
	// Secrets never enter TUI state; the backend returns display-safe results.
	ModelManagement ModelManagementBackend
}

// BuildInfo is immutable, non-secret build provenance suitable for the TUI.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// UpdateStatus is the result of a check-only release lookup.
type UpdateStatus struct {
	Current   string
	Latest    string
	Available bool
}

// ManagementBackend is the narrow command-layer boundary used by /manage.
// Destructive actions receive stable IDs and are always confirmed in the TUI.
type ManagementBackend interface {
	Snapshot(runRecord string) (ManagementSnapshot, error)
	SetExecutionPolicy(mode, network string) error
	SetDefaults(provider, model string) error
	SetTrust(kind string, trusted bool) error
	SetExtensionEnabled(id string, enabled bool) error
	RemoveExtension(id string) error
	// PrepareExtension stages one local directory or hosted HTTPS Git source
	// and returns display-safe metadata for review. It must not publish the
	// bundle or change any configuration.
	PrepareExtension(source string) (ExtensionInstallPreview, error)
	// CommitExtensionInstall publishes exactly the staged bytes identified by
	// token whose hash equals the reviewed hash, then enables the extension.
	CommitExtensionInstall(token, hash string, replace bool) error
	// DiscardExtensionPrepare removes a staged bundle that was not installed.
	DiscardExtensionPrepare(token string) error
	// BeginMCPOAuthLogin starts a loopback browser authorization for exactly
	// one configured Streamable HTTP MCP server. Implementations must refuse
	// before emitting an authorization URL unless the current project manifest
	// hash is already trusted.
	BeginMCPOAuthLogin(server string) (OAuthLogin, error)
	// RemoveMCPCredential deletes only Gator's own stored token for one
	// configured MCP server. It never contacts the remote service.
	RemoveMCPCredential(server string) error
	PruneWorktrees() error
	RemoveWorktree(id string) error
	ExportArtifact(kind, runRecord string) (string, error)
	CheckPatch(runRecord string) (int, error)
	ApplyPatch(runRecord string) (int, error)
}

type DoctorBackend interface {
	Snapshot(provider string) (DoctorSnapshot, error)
}

// BrowserBackend is the user-facing local browser control surface. Every
// operation is local and explicit; the controller returned for a run exposes
// only tabs selected through this same backend.
type BrowserBackend interface {
	Sessions() ([]gatorbrowser.Session, error)
	Start(headed, visualCapture bool) (gatorbrowser.Session, error)
	Attach(cdpEndpoint string, visualCapture bool) (gatorbrowser.Session, []gatorbrowser.Tab, error)
	CandidateTabs(sessionID string) ([]gatorbrowser.Tab, error)
	SelectTabs(sessionID string, tabIDs []string) (gatorbrowser.Session, error)
	AddOrigin(sessionID, value string) (gatorbrowser.Session, error)
	RemoveOrigin(sessionID, value string) (gatorbrowser.Session, error)
	SetVisualCapture(sessionID string, allowed bool) (gatorbrowser.Session, error)
	AllowUpload(sessionID, path string) (gatorbrowser.Upload, error)
	Artifacts(sessionID string) ([]gatorbrowser.Artifact, error)
	ExportArtifact(sessionID, artifactID, destination string) error
	Stop(sessionID string) (gatorbrowser.Session, error)
	Controller(sessionID string) (gatorbrowser.Controller, error)
}

type DoctorSnapshot struct {
	RepositoryDetected  bool
	RepositoryPath      string
	Provider            string
	AuthKind            string
	AuthStatus          string
	Sandbox             string
	WebSearchConfigured bool
	WebSearchStatus     string
	Dependencies        []DoctorDependency
	LocalHost           string
	LocalModels         []DoctorLocalModel
	SuggestedVerify     []string
	EffectiveSandbox    string
	EffectiveNetwork    string
}

type DoctorDependency struct {
	Name      string
	Installed bool
	Required  bool
	Purpose   string
	HelpURL   string
	Advice    []string
}

type DoctorLocalModel struct {
	ID      string
	Allowed bool
	Reason  string
	Needs   string
}

type ManagementSnapshot struct {
	Config     ManagedConfig
	Settings   ManagedSettings
	Trusts     []ManagedTrust
	MCPAuth    []ManagedMCPAuth
	Runs       []ManagedRun
	Worktrees  []ManagedWorktree
	Children   []ManagedChild
	Batches    []ManagedBatch
	Extensions []ManagedExtension
	LSPRuntime []ManagedLSPRuntime
}

// ManagedConfig is a bounded, display-safe rendering of config.json. It is
// intentionally distinct from the private auth store: a backend must redact
// any secret-shaped values before returning this snapshot.
type ManagedConfig struct {
	Path      string
	JSON      string
	Truncated bool
}

// ManagedMCPAuth is display-safe MCP authorization state for one configured
// Streamable HTTP server. It deliberately carries no token, endpoint URL,
// client identifier, or authorization-server metadata.
type ManagedMCPAuth struct {
	Server        string
	Authenticated bool
	Expired       bool
	// BundleTrusted reports whether the current .gator/mcp.json hash is the
	// explicitly trusted hash. Login stays unavailable while it is false.
	BundleTrusted bool
}

type ManagedSettings struct {
	SandboxMode     string
	Network         string
	DefaultProvider string
	DefaultModel    string
}

type ManagedRun struct {
	ID           string
	StatePath    string
	WorktreePath string
	Provider     string
	Model        string
	Task         string
	UpdatedAt    time.Time
	Available    bool
}

type ManagedTrust struct {
	Kind       string
	Hash       string
	Configured bool
	Trusted    bool
}

type ManagedWorktree struct {
	ID   string
	Path string
}

type ManagedChild struct {
	ID                  string
	Status              string
	Role                string
	BatchID             string
	WorktreePath        string
	Provider            string
	Model               string
	Profile             string
	DeclaredPaths       []string
	ChangedPaths        []string
	EffectiveMode       string
	EffectiveSandbox    string
	EffectiveNetwork    string
	MaxSteps            int
	OmittedCapabilities []string
	PatchBytes          int
	Error               string
}

type ManagedBatch struct {
	ID               string
	Status           string
	ChildIDs         []string
	Conflicts        []ManagedConflict
	ComparisonStatus string
	ComparisonTree   string
	ComparisonDetail string
	Error            string
}

type ManagedConflict struct {
	Kind     string
	ChildIDs []string
	Paths    []string
	Detail   string
}

type ManagedExtension struct {
	ID          string
	Name        string
	Description string
	Enabled     bool
	Tools       int
}

type ManagedLSPRuntime struct {
	Worktree string
	Hash     string
	Active   int
	Retired  bool
	Servers  []ManagedLSPServer
}

type ManagedLSPServer struct {
	Name     string
	Language string
	Started  bool
}

// ExtensionInstallPreview describes staged executable code awaiting review.
// Token identifies the private staging directory; Hash is the object the
// developer confirms, and only those exact bytes may then be published.
type ExtensionInstallPreview struct {
	Token            string
	Source           string
	ID               string
	Name             string
	Description      string
	Hash             string
	Skills           int
	Prompts          int
	Commands         int
	UI               int
	Tools            int
	AlreadyInstalled bool
}

// ModelManagementBackend is the secret-safe command-layer boundary for /model
// credential removal and custom OpenAI-compatible provider editing.
type ModelManagementBackend interface {
	CredentialStatuses() ([]StoredCredentialStatus, error)
	RemoveCredential(provider string) (CredentialRemovalResult, error)
	SaveCustomProvider(CustomProviderSetup) ([]config.CustomProvider, error)
	RemoveCustomProvider(id string) ([]config.CustomProvider, error)
	DiscoverCustomProvider(id string) (CustomProviderDiscovery, error)
	ApplyCustomProviderDiscovery(id string, models []string) ([]config.CustomProvider, error)
}

// StoredCredentialStatus is display-safe metadata about one Gator auth.json
// entry. It must never include key, access, or refresh material.
type StoredCredentialStatus struct {
	Provider string
	StoreKey string
	Present  bool
	Kind     string
	Expired  bool
}

// CredentialRemovalResult reports whether a Gator-owned credential was deleted
// and which non-secret ambient sources still exist in this process.
type CredentialRemovalResult struct {
	Provider         string
	StoreKey         string
	Removed          bool
	Kind             string
	RemainingSources []string
}

// CustomProviderSetup is non-secret custom-provider metadata. APIKeyEnv is an
// environment variable name, never a key value.
type CustomProviderSetup struct {
	ID           string
	BaseURL      string
	APIKeyEnv    string
	Models       []string
	DefaultModel string
}

// CustomProviderDiscovery is an untrusted /models catalog preview.
type CustomProviderDiscovery struct {
	ID           string
	Models       []string
	DefaultModel string
}

// CloudModelSetup is the secret-safe boundary between the TUI and the command
// layer. APIKey is populated only while a save is in progress and must never
// be rendered, copied to a draft, or returned in a completion message.
type CloudModelSetup struct {
	Provider        string
	Model           string
	BaseURL         string
	Options         map[string]string
	APIKey          string
	CredentialType  string
	DelegateRuntime string
}

// ExtensionCommand is a visible prompt template contributed by a trusted or
// explicitly installed extension. Selecting it only fills the composer.
type ExtensionCommand struct {
	Name        string
	Description string
	Prompt      string
}

// ExtensionUIContribution is a host-rendered card from a verified extension.
// It intentionally contains static text and an optional composer template,
// never executable UI code, a style sheet, or a browser endpoint.
type ExtensionUIContribution struct {
	ID          string
	Slot        string
	Title       string
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
	terminalScreen
	reviewScreen
	transcriptScreen
	helpScreen
	recentScreen
	threadScreen
	effortScreen
	extensionUIScreen
	localModelsScreen
	managementScreen
	doctorScreen
	runOptionsScreen
	reviewWebScreen
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
	terminals chan terminal.Attachment
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

type terminalManagerMsg struct {
	attachment terminal.Attachment
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

type openCodeCommandDoneMsg struct {
	action   string
	provider string
	err      error
}

type updateStatusMsg struct {
	status UpdateStatus
	err    error
}

type diffLoadedMsg struct {
	diff      string
	truncated bool
	err       error
}

type reviewLoadedMsg struct {
	snapshot review.Snapshot
	err      error
}

type reviewMutationDoneMsg struct {
	err error
}

type activityTickMsg struct{}

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
	return newModel(config, false)
}

func newModel(config Config, catalogOnly bool) Model {
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
	if config.TerminalRegistry == nil {
		config.TerminalRegistry = terminal.NewRegistry()
	}
	if config.LSPRegistry == nil {
		config.LSPRegistry = lsp.NewRegistry()
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

	terminalInput := textinput.New()
	terminalInput.Prompt = "› "
	terminalInput.Placeholder = "type terminal input, then Enter"
	terminalInput.CharLimit = 16 * 1024
	terminalInput.Width = 60
	terminalInput.Blur()

	recentTarget := textinput.New()
	recentTarget.Prompt = ""
	recentTarget.Placeholder = "thread ID or run record path"
	recentTarget.CharLimit = 512
	recentTarget.Width = 60
	recentTarget.Blur()

	localSpinner := spinner.New(spinner.WithSpinner(spinner.Spinner{
		Frames: rattles.BrailleDots.Frames,
		FPS:    rattles.BrailleDots.Interval,
	}), spinner.WithStyle(keyStyle))
	copyToClipboard := config.CopyToClipboard
	if copyToClipboard == nil {
		copyToClipboard = clipboard.WriteAll
	}
	reviewRequest := textarea.New()
	reviewRequest.Prompt = ""
	reviewRequest.Placeholder = "Describe the requested change… (Ctrl+R sends a constrained follow-up)"
	reviewRequest.CharLimit = maximumReviewRequestBytes
	reviewRequest.SetHeight(5)
	reviewRequest.SetWidth(76)
	reviewRequest.Blur()

	application := Model{
		config:           config,
		catalogOnly:      catalogOnly,
		screen:           composeScreen,
		recentAll:        config.RecentAll,
		task:             task,
		verification:     verification,
		provider:         provider,
		terminalInput:    terminalInput,
		recentTarget:     recentTarget,
		terminalRegistry: config.TerminalRegistry,
		lspRegistry:      config.LSPRegistry,
		terminalViews:    make(map[string]attachedTerminalView),
		model:            model,
		localModels:      newLocalModelsState(config.LocalModels, localSpinner),
		management:       newManagementState(config.Management),
		runOptions:       newRunOptionsState(),
		reviewWeb:        newReviewWebState(),
		effort:           parseEffort(config.Effort),
		reviewScope:      review.All,
		reviewRangeFrom:  -1,
		reviewRawFiles:   make(map[string]bool),
		reviewRequest:    reviewRequest,
		transcript:       viewport.New(76, 8),
		followTranscript: true,
		copyToClipboard:  copyToClipboard,
		notice: notice{
			text: "Gator works in an isolated Git worktree. Review remains explicit.",
			kind: noticeInfo,
		},
	}
	application.appendChat(chatEntry{author: chatSystem, text: "New isolated thread. Gator works in a separate Git worktree; use /permissions for the active policy."})
	if !catalogOnly && strings.TrimSpace(config.StateDir) != "" {
		if draft, found, err := journal.LoadDraft(config.StateDir, config.RepositoryPath); err != nil {
			application.draftErr = err
			application.notice = notice{text: "Draft recovery unavailable: " + err.Error(), kind: noticeError}
		} else if found {
			application.task.SetValue(draft.Task)
			application.verification.SetValue(draft.Verification)
			application.provider.SetValue(draft.Provider)
			application.model.SetValue(draft.Model)
			application.applyDraftRunOptions(draft)
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

// Close releases every task retained by this interactive process. Detached
// terminals are session-bound by design; quitting Gator is an explicit stop.
func (m Model) Close() {
	if m.execution != nil && m.execution.cancel != nil {
		m.execution.cancel()
	}
	if closer, ok := m.localModels.manager.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
	if m.terminalRegistry != nil {
		m.terminalRegistry.Close()
	}
	if m.lspRegistry != nil {
		m.lspRegistry.Close()
	}
	m.discardStagedExtension()
	m.stopReviewWeb()
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
		if m.screen == terminalScreen {
			m.refreshAttachedTerminal()
		}
	case commandApprovalMsg:
		m.pendingApproval = &pendingCommandApproval{argv: append([]string(nil), msg.argv...), reply: msg.reply}
		m.notice = notice{text: "Approve worktree command: " + strings.Join(msg.argv, " "), kind: noticeInfo}
		m.setActivity(activityAwaitingApproval, "awaiting command approval", time.Now())
		return m, waitForExecution(m.execution)
	case terminalManagerMsg:
		m.setTerminalAttachment(msg.attachment)
		return m, waitForExecution(m.execution)
	case agentEventMsg:
		m.appendEvent(msg.event)
		return m, waitForExecution(m.execution)
	case activityTickMsg:
		if (m.screen == runningScreen || m.screen == terminalScreen) && m.execution != nil {
			if m.screen == terminalScreen {
				m.refreshAttachedTerminal()
			}
			return m, waitForExecution(m.execution)
		}
		return m, nil
	case spinner.TickMsg:
		if m.screen == localModelsScreen && m.localModels.action != localModelIdle {
			var command tea.Cmd
			m.localModels.spinner, command = m.localModels.spinner.Update(msg)
			return m, command
		}
	case clipboardWriteMsg:
		return m.applyClipboardWrite(msg), nil
	case cloudModelSetupSavedMsg:
		if msg.err != nil {
			if m.localModels.cloudSetup != nil {
				m.localModels.cloudSetup.saving = false
			}
			m.notice = notice{text: "Save cloud model configuration: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		m.localModels.cloudSetup = nil
		m.provider.SetValue(msg.provider)
		m.model.SetValue(msg.model)
		m.config.BaseURL = msg.baseURL
		if m.config.ProviderEndpoints == nil {
			m.config.ProviderEndpoints = make(map[string]string)
		}
		if msg.baseURL == "" {
			delete(m.config.ProviderEndpoints, msg.provider)
		} else {
			m.config.ProviderEndpoints[msg.provider] = msg.baseURL
		}
		if m.config.ProviderOptions == nil {
			m.config.ProviderOptions = make(map[string]map[string]string)
		}
		if len(msg.options) == 0 {
			delete(m.config.ProviderOptions, msg.provider)
		} else {
			m.config.ProviderOptions[msg.provider] = cloneProviderOptions(msg.options)
		}
		m.delegateRuntime = msg.delegateRuntime
		m.persistDraft()
		m.refreshPreflight()
		m.selectActiveModelCatalogEntry()
		m.notice = notice{text: "Cloud model configuration saved. The selected model is ready when its provider reports configured.", kind: noticeSuccess}
		return m, nil
	case credentialStatusesMsg:
		if msg.err != nil {
			m.notice = notice{text: "Read stored credentials: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		m.applyCredentialStatuses(msg.statuses)
		return m, nil
	case credentialRemovedMsg:
		if msg.err != nil {
			m.notice = notice{text: "Remove Gator credential: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		delete(m.localModels.credentials, msg.result.StoreKey)
		m.notice = notice{text: credentialRemovalNotice(msg.result), kind: noticeSuccess}
		if m.config.ModelManagement != nil {
			return m, loadCredentialStatuses(m.config.ModelManagement)
		}
		return m, nil
	case customProviderSavedMsg:
		if m.localModels.customSetup != nil {
			m.localModels.customSetup.saving = false
		}
		if msg.err != nil {
			m.notice = notice{text: "Save custom provider: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		m.localModels.customSetup = nil
		m.localModels.pendingCustom = nil
		m.applyCustomProviders(msg.providers, msg.id)
		m.notice = notice{text: "Saved custom provider " + msg.id + ". The API key environment variable was not written to config.json.", kind: noticeSuccess}
		return m, nil
	case customProviderRemovedMsg:
		if msg.err != nil {
			m.notice = notice{text: "Remove custom provider: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		m.applyCustomProviders(msg.providers, msg.id)
		m.notice = notice{text: "Removed custom provider " + msg.id + ". Its API key environment variable was not changed.", kind: noticeSuccess}
		return m, nil
	case customProviderDiscoverMsg:
		if msg.generation != m.localModels.generation || m.localModels.action != localModelDiscovering {
			return m, nil
		}
		m.localModels.action = localModelIdle
		if msg.err != nil {
			m.notice = notice{text: "Discover custom provider models: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		preview := msg.preview
		m.localModels.discovery = &preview
		m.localModels.confirmation = localModelConfirmApplyDiscovery
		m.notice = notice{text: "Discovered model IDs are untrusted server data. Confirm to replace the configured catalog.", kind: noticeInfo}
		return m, nil
	case customProviderAppliedMsg:
		if msg.err != nil {
			m.notice = notice{text: "Apply discovered models: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		m.localModels.discovery = nil
		m.applyCustomProviders(msg.providers, msg.id)
		m.notice = notice{text: "Replaced the model catalog for " + msg.id + " with discovered IDs.", kind: noticeSuccess}
		return m, nil
	case localModelStatusMsg:
		if msg.generation != m.localModels.generation || m.localModels.action != localModelRefreshing {
			return m, nil
		}
		m.localModels.action = localModelIdle
		m.localModels.operation = nil
		m.localModels.err = msg.err
		if msg.err != nil {
			m.notice = notice{text: "Check local models: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		m.applyLocalModelCatalog(msg.catalog)
		m.selectActiveModelCatalogEntry()
		if msg.catalog.RuntimeError != "" {
			if m.localModels.section == localModelSection && !m.localModels.startDismissed {
				m.requestLocalRuntimeRecovery()
			} else {
				if m.ollamaMissing() {
					m.notice = notice{text: "Ollama is not installed. Open the Local section for installation help.", kind: noticeInfo}
				} else {
					m.notice = notice{text: "Local runtime is unavailable. Open the Local section to choose whether Gator should start Ollama.", kind: noticeInfo}
				}
			}
		} else {
			m.localModels.startDismissed = false
			m.notice = notice{text: "Local model catalog refreshed.", kind: noticeSuccess}
		}
		return m, nil
	case localModelProgressMsg:
		if msg.operation == nil || msg.operation != m.localModels.operation || m.localModels.action == localModelIdle {
			return m, nil
		}
		m.localModels.progress = msg.progress
		return m, waitForLocalModelOperation(m.localModels.operation)
	case localModelDoneMsg:
		if msg.operation == nil || msg.operation != m.localModels.operation {
			return m, nil
		}
		m.localModels.operation = nil
		action := m.localModels.action
		m.localModels.action = localModelIdle
		m.localModels.err = msg.done.err
		if msg.done.err != nil {
			m.notice = notice{text: "Local model operation stopped: " + msg.done.err.Error(), kind: noticeError}
			return m, nil
		}
		if msg.done.aliases != nil {
			m.applyModelAliases(msg.done.aliases)
			m.notice = notice{text: "Model display name saved. Provider model IDs are unchanged.", kind: noticeSuccess}
			return m, nil
		}
		if msg.done.update != nil {
			m.applyLocalModelUpdate(*msg.done.update)
			if action == localModelRemoving {
				m.notice = notice{text: "Local model removed and current configuration updated.", kind: noticeSuccess}
			} else {
				m.notice = notice{text: "Local model selected. Return to the composer and send a task to use it.", kind: noticeSuccess}
			}
			return m, nil
		}
		if msg.done.catalog != nil {
			m.applyLocalModelCatalog(*msg.done.catalog)
		}
		if action == localModelStarting {
			m.notice = notice{text: "Ollama is running under this Gator session. Choose a local model to download or use.", kind: noticeSuccess}
		} else {
			m.notice = notice{text: "Local model download finished. Press u to select it for Gator.", kind: noticeSuccess}
		}
		return m, nil
	case doctorSnapshotMsg:
		m.doctor.loading = false
		m.doctor.err = msg.err
		if msg.err != nil {
			m.notice = notice{text: "Refresh diagnostics: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		m.doctor.data = msg.snapshot
		return m, nil
	case reviewWebStartedMsg:
		m.applyReviewWebStarted(msg)
		return m, nil
	case managementSnapshotMsg:
		m.management.loading = false
		m.management.err = msg.err
		if msg.err != nil {
			m.notice = notice{text: "Refresh management state: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		m.management.data = msg.snapshot
		m.config.Execution.Mode = sandbox.Mode(msg.snapshot.Settings.SandboxMode)
		m.config.Execution.Network = sandbox.Network(msg.snapshot.Settings.Network)
		if m.management.section == managementRuns && m.management.selectedRunRecord != "" {
			for index, run := range msg.snapshot.Runs {
				if run.StatePath == m.management.selectedRunRecord {
					m.management.index = index
					break
				}
			}
		}
		if count := m.managementItemCount(); count == 0 {
			m.management.index = 0
		} else if m.management.index >= count {
			m.management.index = count - 1
		}
		return m, nil
	case extensionPreparedMsg:
		m.management.loading = false
		if msg.err != nil {
			m.notice = notice{text: "Stage extension source: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		preview := msg.preview
		m.management.installPreview = &preview
		label := "Install extension " + preview.ID + " from the staged bundle sha256:" + preview.Hash
		replace := ""
		if preview.AlreadyInstalled {
			replace = "replace"
			label = "Replace installed extension " + preview.ID + " with the staged bundle sha256:" + preview.Hash
		}
		m.management.confirm = &managementConfirmation{
			action: "install-extension", id: preview.Token, value: preview.Hash, value2: replace, label: label,
		}
		m.notice = notice{text: "Review the staged bundle. Only these exact bytes are installed if you confirm.", kind: noticeInfo}
		return m, nil
	case mcpLoginDoneMsg:
		if msg.server != m.management.mcpLoginServer {
			return m, nil
		}
		m.oauthLogin = nil
		m.oauthCancel = nil
		m.oauthProvider = ""
		m.management.mcpLoginServer = ""
		m.management.mcpLoginURL = ""
		if msg.err != nil {
			m.notice = notice{text: "MCP authorization stopped: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		m.notice = notice{text: "Stored a Gator OAuth credential bound to MCP server " + msg.server + ". Its tools still require approval.", kind: noticeSuccess}
		m.management.loading = true
		return m, loadManagement(m.management.backend, m.management.selectedRunRecord)
	case managementActionMsg:
		m.management.loading = false
		if msg.err != nil {
			m.management.err = msg.err
			m.management.actionDetail = ""
			m.notice = notice{text: msg.message + ": " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		m.management.err = nil
		m.management.actionDetail = msg.detail
		if msg.checkedRun != "" {
			m.management.checkedRunRecord = msg.checkedRun
		} else if strings.HasPrefix(msg.message, "Applied") || strings.Contains(strings.ToLower(msg.message), "apply retained") {
			m.management.checkedRunRecord = ""
		}
		m.notice = notice{text: msg.message + ".", kind: noticeSuccess}
		m.management.loading = true
		return m, loadManagement(m.management.backend, m.management.selectedRunRecord)
	case executionDoneMsg:
		m.resolvePendingApproval(tools.CommandDeny)
		wasNewThread := m.resumeStatePath == ""
		wasCancelling := m.cancelling
		m.execution = nil
		if !m.hasBackgroundTerminalTask() {
			m.setTerminalAttachment(nil)
		}
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
			} else if m.hasBackgroundTerminalTask() {
				m.notice = notice{text: "Run complete. A detached terminal task is still running in this Gator session; press Ctrl+T to attach.", kind: noticeSuccess}
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
		return m, loadReview(msg.done.outcome)
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
	case openCodeCommandDoneMsg:
		if msg.err != nil {
			m.notice = notice{text: "OpenCode " + msg.action + " stopped: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		if msg.action == "login" {
			m.returnToComposer()
			m.delegateRuntime = "opencode"
			m.provider.SetValue("opencode")
			m.commandOutput = "OpenCode login completed for " + msg.provider + ". Select a harness model with /opencode use PROVIDER/MODEL, then send an Execute-mode task."
			m.notice = notice{text: "OpenCode provider login completed. The installed CLI retains its credential.", kind: noticeSuccess}
			m.persistDraft()
			m.refreshPreflight()
			return m, m.focusField()
		}
		m.commandOutput = "OpenCode authentication status completed in the terminal. Use /opencode login PROVIDER [METHOD] to change a provider connection."
		m.notice = notice{text: "OpenCode status completed.", kind: noticeSuccess}
		return m, nil
	case updateStatusMsg:
		m.updateChecking = false
		if msg.err != nil {
			m.commandOutput = m.versionStatus() + "\n\nUpdate check failed: " + msg.err.Error()
			m.notice = notice{text: "Update check failed: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		current := strings.TrimSpace(msg.status.Current)
		if current == "" {
			current = strings.TrimSpace(m.config.Build.Version)
		}
		if current == "" {
			current = "dev"
		}
		latest := strings.TrimSpace(msg.status.Latest)
		if msg.status.Available {
			m.commandOutput = "Gator " + latest + " is available; this TUI is running " + current + ".\n\nExit the TUI and run 'gator update' to download, verify, and replace the executable."
			m.notice = notice{text: "A newer Gator release is available. The running TUI was not changed.", kind: noticeInfo}
			return m, nil
		}
		m.commandOutput = "Gator " + current + " is up to date."
		if latest != "" && latest != current {
			m.commandOutput += " Latest published release: " + latest + "."
		}
		m.notice = notice{text: "No update was installed; the running build is current.", kind: noticeSuccess}
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
		m.focusedDiffInfo = diffview.Focus(msg.diff)
		m.focusedDiff = m.focusedDiffInfo.Text
		m.diffOffset = 0
		if msg.err == nil {
			m.diffStats = summarizeDiff(msg.diff)
		}
		return m, nil
	case reviewLoadedMsg:
		m.reviewLoaded = msg.err == nil
		m.diffErr = msg.err
		if msg.err != nil {
			return m, nil
		}
		m.reviewSnapshot = msg.snapshot
		m.reviewScope = review.All
		m.reviewPane = reviewFilesPane
		m.reviewFileIndex = 0
		m.reviewHunkIndex = 0
		m.reviewLineIndex = 0
		m.reviewRangeFrom = -1
		m.reviewMutation = nil
		m.reviewRequestOn = false
		m.normalizeReviewSelection()
		return m, nil
	case reviewMutationDoneMsg:
		m.reviewMutation = nil
		if msg.err != nil {
			m.notice = notice{text: "Update review selection: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		m.notice = notice{text: "Review selection updated in the retained worktree.", kind: noticeSuccess}
		if m.outcome == nil {
			return m, nil
		}
		return m, loadReview(*m.outcome)
	case tea.MouseMsg:
		if m.screen == reviewScreen {
			return m.updateReviewMouse(msg)
		}
		if m.screen == composeScreen || m.screen == runningScreen {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.pageTranscript(false)
				return m, nil
			case tea.MouseButtonWheelDown:
				m.pageTranscript(true)
				return m, nil
			}
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
