package modelcatalog

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/rattles"
)

// OAuthLogin is an application-owned browser login. The panel displays its URL
// and retains cancellation while the command layer owns credential storage.
type OAuthLogin interface {
	URL() string
	Complete(context.Context) error
	Cancel()
}

// Config contains only the state and callbacks required by model management.
// It intentionally excludes composer, execution, review, and worktree state.
type Config struct {
	Provider           string
	Model              string
	BaseURL            string
	StateDir           string
	Theme              string
	CustomProviders    []config.CustomProvider
	ModelAliases       map[string]string
	ProviderEndpoints  map[string]string
	ProviderOptions    map[string]map[string]string
	LocalModels        LocalManager
	ModelManagement    ManagementBackend
	SaveModelSelection func(provider, model string) error
	SaveCloudModel     func(CloudModelSetup) error
	BeginOAuthLogin    func(provider string) (OAuthLogin, error)
}

type screen uint8

const (
	localModelsScreen screen = iota
	closedScreen
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

// Model is the private Bubble Tea state behind Panel.
type Model struct {
	config          Config
	catalogOnly     bool
	screen          screen
	provider        textinput.Model
	model           textinput.Model
	localModels     localModelsState
	notice          notice
	oauthLogin      OAuthLogin
	oauthCancel     context.CancelFunc
	oauthProvider   string
	delegateRuntime string
	commandOutput   string
	width           int
	height          int
}

func newModel(config Config) Model {
	config.Theme = applyWorkSurfaceTheme(config.Theme)
	if strings.TrimSpace(config.Provider) == "" {
		config.Provider = "openai"
	}
	provider := textinput.New()
	provider.SetValue(config.Provider)
	model := textinput.New()
	model.SetValue(config.Model)
	localSpinner := spinner.New(spinner.WithSpinner(spinner.Spinner{
		Frames: rattles.BrailleDots.Frames,
		FPS:    rattles.BrailleDots.Interval,
	}), spinner.WithStyle(keyStyle))
	return Model{
		config:      config,
		catalogOnly: true,
		screen:      localModelsScreen,
		provider:    provider,
		model:       model,
		localModels: newLocalModelsState(config.LocalModels, localSpinner),
	}
}

func (m Model) Init() tea.Cmd { return nil }
func (m Model) View() string  { return "" }

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case spinner.TickMsg:
		if m.localModels.action != localModelIdle {
			var command tea.Cmd
			m.localModels.spinner, command = m.localModels.spinner.Update(msg)
			return m, command
		}
	case cloudModelSetupSavedMsg:
		return m.updateCloudModelSetupSaved(msg)
	case credentialStatusesMsg:
		return m.updateCredentialStatuses(msg)
	case credentialRemovedMsg:
		return m.updateCredentialRemoved(msg)
	case customProviderSavedMsg:
		return m.updateCustomProviderSaved(msg)
	case customProviderRemovedMsg:
		return m.updateCustomProviderRemoved(msg)
	case customProviderDiscoverMsg:
		return m.updateCustomProviderDiscovered(msg)
	case customProviderAppliedMsg:
		return m.updateCustomProviderApplied(msg)
	case localModelStatusMsg:
		return m.updateLocalModelStatus(msg)
	case localModelProgressMsg:
		return m.updateLocalProgress(msg)
	case localModelDoneMsg:
		return m.updateLocalModelDone(msg)
	case oauthLoginDoneMsg:
		return m.updateOAuthLoginDone(msg)
	}
	return m, nil
}

// Selection persistence is owned by the explicit callbacks above. A focused
// catalog has no composer draft or run preflight to update.
func (m *Model) persistDraft()       {}
func (m *Model) refreshPreflight()   {}
func (m *Model) focusField() tea.Cmd { return nil }
