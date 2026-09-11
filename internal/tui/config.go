// Package tui provides Gator's interactive terminal application.
package tui

import (
	"context"
	"os/exec"
	"time"

	gatorbrowser "github.com/gongahkia/gator/internal/browser"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/lsp"
	"github.com/gongahkia/gator/internal/modelcatalog"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/terminal"
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
type ModelManagementBackend = modelcatalog.ManagementBackend

// StoredCredentialStatus is display-safe metadata about one Gator auth.json
// entry. It must never include key, access, or refresh material.
type StoredCredentialStatus = modelcatalog.StoredCredentialStatus

// CredentialRemovalResult reports whether a Gator-owned credential was deleted
// and which non-secret ambient sources still exist in this process.
type CredentialRemovalResult = modelcatalog.CredentialRemovalResult

// CustomProviderSetup is non-secret custom-provider metadata. APIKeyEnv is an
// environment variable name, never a key value.
type CustomProviderSetup = modelcatalog.CustomProviderSetup

// CustomProviderDiscovery is an untrusted /models catalog preview.
type CustomProviderDiscovery = modelcatalog.CustomProviderDiscovery

// CloudModelSetup is the secret-safe boundary between the TUI and the command
// layer. APIKey is populated only while a save is in progress and must never
// be rendered, copied to a draft, or returned in a completion message.
type CloudModelSetup = modelcatalog.CloudModelSetup

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
