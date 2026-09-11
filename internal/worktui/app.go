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
	Error          string
}

// RunOptions contains the user-selected orchestration controls that accompany
// one prompt. They configure Gator and bound its internal Code specialist;
// they are never editable by the manager model itself.
type RunOptions struct {
	Context     context.Context
	OnEvent     agent.EventSink
	Steering    <-chan string
	OnOperation func(*workrun.Operation)
	MaxSteps    int
	Attachments []string
	Code        CodeOptions
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
	Models              func() (ModelPanel, error)
	SelectedModel       func() (provider, model string, err error)
	ModelStatus         func() (ModelStatus, error)
	Live                bool
	CurrentFolder       string
	Conversations       []worksession.Conversation
	StartConversationID string
	Jobs                []jobs.Definition
	Inbox               []inbox.Entry
	Run                 func(source, conversationID, prompt string, options RunOptions) RunResult
	MoveBack            func(conversationID string) (string, error)
	MoveForward         func(conversationID string) (string, error)
	MoveToRevision      func(conversationID, revisionID string) (string, error)
	History             func(conversationID string) (string, error)
	FirstRun            bool
	ProviderCommand     func(action, provider string) *exec.Cmd
	ProviderChoices     func(action string) []string
	CompleteSetup       func(provider string) error
	Inspect             func(topic string) (string, error)
	Copy                func(text string) error
	Theme               string
	SetTheme            func(name string) error
}

type ModelPanel interface {
	tea.Model
	Closed() bool
	Close()
}

type entry struct {
	title, subtitle, kind, id, source, command string
}
type message struct{ role, text string }
type runDone RunResult
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

type Model struct {
	models              ModelPanel
	tasks               map[string]string
	quitting            bool
	live                <-chan tea.Msg
	cancel              context.CancelFunc
	operation           *workrun.Operation
	interaction         *workrun.Interaction
	pendingInteractions []workrun.Interaction
	config              Config
	width               int
	height              int
	home                bool
	launcher            bool
	launcherMode        string
	paletteQuery        string
	section             string
	selected            int
	entries             []entry
	source              string
	conversation        string
	title               string
	input               string
	messages            []message
	running             bool
	status              string
	onboarding          bool
	firstRun            bool
	pendingPrompt       string
	scroll              int
	options             RunOptions
	queue               []queuedRun
	lastOutput          string
	theme               string
	loadingFrame        int
	loadingRun          uint64
}

func New(config Config) Model {
	model := Model{
		config:   config,
		home:     true,
		firstRun: config.FirstRun,
		source:   config.CurrentFolder,
		title:    "Work in " + filepath.Base(config.CurrentFolder),
		theme:    normalizeTheme(config.Theme),
		options: RunOptions{MaxSteps: 24, Code: CodeOptions{
			MaxSteps: 16, Sandbox: "strict", Network: "deny",
		}},
	}
	model.entries = commandPaletteEntries()
	if config.StartConversationID != "" {
		for _, conversation := range config.Conversations {
			if conversation.ID == config.StartConversationID {
				model.home = false
				model.source = conversation.SourcePath
				model.conversation = conversation.ID
				model.title = conversation.Title
				break
			}
		}
	}
	return model
}

func (m Model) Init() tea.Cmd { return nil }
