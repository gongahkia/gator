package tui

import (
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/lsp"
	"github.com/gongahkia/gator/internal/rattles"
	"github.com/gongahkia/gator/internal/review"
	"github.com/gongahkia/gator/internal/terminal"
)

func New(config Config) Model {
	return newModel(config, false)
}

func newModel(config Config, catalogOnly bool) Model {
	if catalogOnly {
		config.Theme = applyWorkSurfaceTheme(config.Theme)
	} else {
		config.Theme = applyTheme(config.Theme)
	}
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
