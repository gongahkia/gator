// Package worktui implements Gator's conversation-first Work terminal application.
package worktui

import (
	"context"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

const gatorWordmark = "🐊 Gator"

type RunResult struct {
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
	Path       string
	Status     string
	Artifacts  []ArtifactSummary
	Candidates []CandidateSummary
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
	Replace     bool
	Execute     bool
}

// RunOptions contains the user-selected orchestration controls that accompany
// one prompt. They configure Gator and bound its internal Code specialist;
// they are never editable by the manager model itself.
type RunOptions struct {
	Context           context.Context
	OnEvent           agent.EventSink
	Steering          <-chan string
	OnOperation       func(*workrun.Operation)
	MaxSteps          int
	Attachments       []string
	Mode              string
	Artifacts         []string
	PreviousArtifacts []string
	ConnectorIDs      []string
	WebOrigins        []string
	RefreshSource     bool
	Code              CodeOptions
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

type Config struct {
	Models                  func() (ModelPanel, error)
	SelectedModel           func() (provider, model string, err error)
	ModelStatus             func() (ModelStatus, error)
	Live                    bool
	CurrentFolder           string
	ResolveSource           func(path string) (string, error)
	ConnectorChoices        func() []string
	BundleAction            func(BundleActionRequest) (string, error)
	ListConversations       func() ([]worksession.Conversation, error)
	LoadConversation        func(conversationID string) (ConversationState, error)
	LoadConversationOptions func(conversationID string) (RunOptions, error)
	Conversations           []worksession.Conversation
	StartConversationID     string
	Jobs                    []jobs.Definition
	Inbox                   []inbox.Entry
	Run                     func(source, conversationID, prompt string, options RunOptions) RunResult
	MoveBack                func(conversationID string) (string, error)
	MoveForward             func(conversationID string) (string, error)
	MoveToRevision          func(conversationID, revisionID string) (string, error)
	History                 func(conversationID string) (string, error)
	FirstRun                bool
	ProviderCommand         func(action, provider string) *exec.Cmd
	ProviderChoices         func(action string) []string
	CompleteSetup           func(provider string) error
	Inspect                 func(topic string) (string, error)
	Copy                    func(text string) error
	Theme                   string
	SetTheme                func(name string) error
	// StatusLine follows the Codex-style ordered-item convention. Nil uses
	// Gator's defaults; a non-nil empty list hides the composer footer.
	StatusLine    *[]string
	SetStatusLine func(items *[]string) error
}

type ModelPanel interface {
	tea.Model
	Closed() bool
	Close()
}

type entry struct {
	title, subtitle, kind, id, source, command string
}
type message struct {
	role   string
	text   string
	bundle *BundleSummary
}
type runDone RunResult
type bundleActionDone struct {
	action string
	text   string
	err    error
}
type loadingTickMsg struct{ run uint64 }

type providerActionDone struct {
	action   string
	provider string
	err      error
}

type queuedRun struct {
	prompt  string
	options RunOptions
}

type pendingBundleAction struct {
	request BundleActionRequest
	summary string
}

type Model struct {
	models               ModelPanel
	tasks                map[string]string
	quitting             bool
	live                 <-chan tea.Msg
	cancel               context.CancelFunc
	operation            *workrun.Operation
	interaction          *workrun.Interaction
	pendingInteractions  []workrun.Interaction
	config               Config
	width                int
	height               int
	home                 bool
	launcher             bool
	launcherMode         string
	paletteQuery         string
	section              string
	selected             int
	entries              []entry
	source               string
	conversation         string
	revision             string
	snapshot             string
	title                string
	input                string
	messages             []message
	running              bool
	status               string
	onboarding           bool
	firstRun             bool
	pendingPrompt        string
	scroll               int
	options              RunOptions
	queue                []queuedRun
	lastOutput           string
	lastBundle           BundleSummary
	pendingBundleAction  *pendingBundleAction
	theme                string
	loadingFrame         int
	loadingRun           uint64
	modelStatus          ModelStatus
	statusLine           []string
	statusLineConfigured bool
	statusLineDraft      []string
	statusLineDraftSet   bool
}

func New(config Config) Model {
	model := Model{
		config:   config,
		home:     true,
		firstRun: config.FirstRun,
		source:   config.CurrentFolder,
		title:    "Work in " + filepath.Base(config.CurrentFolder),
		theme:    normalizeTheme(config.Theme),
		options: RunOptions{MaxSteps: 24, Mode: "auto", Code: CodeOptions{
			MaxSteps: 16, Sandbox: "strict", Network: "deny",
		}},
	}
	model.entries = commandPaletteEntries()
	model.statusLine, model.statusLineConfigured = resolveStatusLine(config.StatusLine)
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
	return model
}

func (m Model) Init() tea.Cmd { return nil }
