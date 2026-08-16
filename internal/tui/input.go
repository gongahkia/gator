package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/journal"
)

func (m Model) handleKey(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if message.String() == "f1" {
		if m.screen == helpScreen {
			m.screen = m.helpReturn
		} else {
			m.helpReturn = m.screen
			m.screen = helpScreen
		}
		return m, nil
	}
	switch m.screen {
	case composeScreen:
		return m.updateComposer(message)
	case attachmentConfirmScreen:
		return m.updateAttachmentConfirmation(message)
	case runningScreen:
		return m.updateRunning(message)
	case reviewScreen:
		return m.updateReview(message)
	case transcriptScreen:
		return m.updateTranscript(message)
	case helpScreen:
		return m.updateHelp(message)
	case recentScreen:
		return m.updateRecentRuns(message)
	default:
		return m, nil
	}
}

func (m Model) updateAttachmentConfirmation(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "enter", "y":
		m.attachmentConfirmed = true
		m.screen = composeScreen
		return m.startRun()
	case "esc", "n", "ctrl+c":
		m.attachmentConfirmed = false
		m.attachmentPreview = nil
		m.screen = composeScreen
		m.notice = notice{text: "Attachment send cancelled. No file bytes were sent.", kind: noticeInfo}
		return m, m.focusField()
	default:
		return m, nil
	}
}

func (m Model) updateComposer(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.focus == taskField && message.Type == tea.KeyCtrlAt {
		m.contextClosed = false
		m.normalizeContextSelection()
		if !m.contextCompletionVisible() {
			m.notice = notice{text: "Type @ followed by a repository path to open path suggestions.", kind: noticeInfo}
		}
		return m, nil
	}
	if m.focus == taskField && m.commandPaletteVisible() {
		switch message.String() {
		case "up", "ctrl+p":
			m.moveCommandSelection(-1)
			return m, nil
		case "down", "ctrl+n":
			m.moveCommandSelection(1)
			return m, nil
		case "enter", "tab":
			return m.executeSelectedCommand()
		case "esc":
			m.task.Reset()
			m.commandIndex = 0
			m.notice = notice{text: "Command palette dismissed.", kind: noticeInfo}
			return m, nil
		}
	}
	if m.focus == taskField && m.contextCompletionVisible() {
		switch message.String() {
		case "up", "ctrl+p":
			m.moveContextSelection(-1)
			return m, nil
		case "down", "ctrl+n":
			m.moveContextSelection(1)
			return m, nil
		case "enter", "tab":
			m.applySelectedContextCompletion()
			return m, nil
		case "esc":
			m.contextClosed = true
			m.notice = notice{text: "Path suggestions dismissed.", kind: noticeInfo}
			return m, nil
		}
	}
	if m.dropdownVisible() {
		switch message.String() {
		case "up", "ctrl+p":
			m.moveDropdownSelection(-1)
			return m, nil
		case "down", "ctrl+n":
			m.moveDropdownSelection(1)
			return m, nil
		case "enter":
			m.applySelectedDropdown()
			return m, nil
		case "tab":
			m.applySelectedDropdown()
			m.focus = nextField(m.focus, false)
			return m, m.focusField()
		}
	}
	switch message.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "ctrl+o":
		return m.openRecentRuns()
	case "pgup":
		m.moveChatSelection(-m.chatEntryLimit())
		return m, nil
	case "pgdown":
		m.moveChatSelection(m.chatEntryLimit())
		return m, nil
	case "?":
		if m.focus == taskField && strings.TrimSpace(m.task.Value()) == "" {
			m.task.SetValue("/")
			m.commandIndex = 0
			return m, nil
		}
	case "ctrl+r":
		return m.startRun()
	case "tab", "shift+tab":
		if m.resumeStatePath != "" {
			m.notice = notice{text: "A continuation inherits the model and verification policy from its original run.", kind: noticeInfo}
			return m, nil
		}
		m.focus = nextField(m.focus, message.String() == "shift+tab")
		return m, m.focusField()
	}

	var command tea.Cmd
	switch m.focus {
	case taskField:
		m.task, command = m.task.Update(message)
		m.normalizeCommandSelection()
		m.contextClosed = false
		m.normalizeContextSelection()
		m.persistDraft()
		m.refreshPreflight()
	case verificationField:
		m.verification, command = m.verification.Update(message)
		m.persistDraft()
		m.refreshPreflight()
	case modelField:
		m.model, command = m.model.Update(message)
		m.normalizeDropdownSelection()
		m.persistDraft()
		m.refreshPreflight()
	case providerField:
		m.provider, command = m.provider.Update(message)
		m.normalizeDropdownSelection()
		m.persistDraft()
		m.refreshPreflight()
	}
	return m, command
}

func (m Model) updateRunning(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "ctrl+c":
		if m.execution != nil && !m.cancelling {
			m.execution.cancel()
			m.cancelling = true
			m.notice = notice{text: "Cancellation requested. Waiting for the current operation to stop...", kind: noticeInfo}
		}
	case "pgup":
		m.moveChatSelection(-m.chatEntryLimit())
	case "pgdown":
		m.moveChatSelection(m.chatEntryLimit())
	}
	return m, nil
}

func (m Model) updateReview(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "n":
		m.returnToComposer()
		return m, nil
	case "esc":
		m.screen = composeScreen
		m.focus = taskField
		return m, m.focusField()
	case "c":
		return m.prepareContinuation()
	case "d":
		if m.outcome == nil || m.outcome.Worktree.Path == "" {
			m.notice = notice{text: "No retained worktree is available for diff review.", kind: noticeError}
			return m, nil
		}
		m.notice = notice{text: "Refreshing the current worktree diff...", kind: noticeInfo}
		return m, loadDiff(m.outcome.Worktree.Root)
	case "e":
		if m.outcome == nil || m.outcome.StatePath == "" {
			m.notice = notice{text: "No retained run record is available for patch handoff.", kind: noticeError}
			return m, nil
		}
		m.commandOutput = "Export:  gator export " + m.outcome.StatePath + " > gator-review.patch\nCheck:   gator apply --check " + m.outcome.StatePath + "\nApply:   gator apply " + m.outcome.StatePath
		m.notice = notice{text: "Patch handoff commands shown. Apply remains explicit and requires a clean compatible checkout.", kind: noticeInfo}
		return m, nil
	case "t":
		m.transcriptReturn = reviewScreen
		m.transcriptIndex = 0
		m.screen = transcriptScreen
		return m, nil
	}
	return m, nil
}

func (m Model) updateTranscript(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc", "t", "q":
		m.screen = m.transcriptReturn
	case "up", "ctrl+p":
		if m.transcriptIndex > 0 {
			m.transcriptIndex--
		}
	case "down", "ctrl+n":
		if m.transcriptIndex < len(m.events)-1 {
			m.transcriptIndex++
		}
	}
	return m, nil
}

func (m *Model) moveChatSelection(delta int) {
	if len(m.chat) == 0 {
		m.chatIndex = 0
		return
	}
	m.chatIndex = min(max(0, m.chatIndex+delta), len(m.chat)-1)
}

func (m Model) updateHelp(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc", "f1", "?", "q":
		m.screen = m.helpReturn
	}
	return m, nil
}

func (m Model) openRecentRuns() (tea.Model, tea.Cmd) {
	threads, err := journal.ListRecentThreads(m.config.StateDir, m.config.RepositoryPath, 20)
	if err != nil {
		m.notice = notice{text: "Load recent threads: " + err.Error(), kind: noticeError}
		return m, nil
	}
	if len(threads) == 0 {
		m.notice = notice{text: "No retained threads are available for this repository.", kind: noticeInfo}
		return m, nil
	}
	m.recentThreads = threads
	m.recentIndex = 0
	m.screen = recentScreen
	return m, nil
}

func (m Model) updateRecentRuns(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc", "q":
		m.screen = composeScreen
		return m, m.focusField()
	case "r", "ctrl+r":
		m.screen = composeScreen
		return m.openRecentRuns()
	case "up", "ctrl+p":
		if len(m.recentThreads) > 0 {
			m.recentIndex = (m.recentIndex - 1 + len(m.recentThreads)) % len(m.recentThreads)
		}
	case "down", "ctrl+n":
		if len(m.recentThreads) > 0 {
			m.recentIndex = (m.recentIndex + 1) % len(m.recentThreads)
		}
	case "enter":
		if len(m.recentThreads) == 0 {
			return m, nil
		}
		selected := m.recentThreads[m.recentIndex]
		if !selected.Available {
			m.notice = notice{text: "The selected retained worktree no longer exists.", kind: noticeError}
			return m, nil
		}
		return m.beginContinuation(selected.HeadStatePath)
	}
	return m, nil
}
