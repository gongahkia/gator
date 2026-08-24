package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/tools"
)

func (m Model) handleKey(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if message.String() == "ctrl+t" && (m.screen == composeScreen || m.screen == runningScreen) && m.pendingApproval == nil {
		m.openAttachedTerminal()
		if m.screen == terminalScreen && m.execution != nil {
			return m, waitForExecution(m.execution)
		}
		return m, nil
	}
	if message.String() == "ctrl+b" && (m.screen == composeScreen || m.screen == runningScreen) {
		m.toggleDrawer()
		return m, nil
	}
	if message.String() == "f1" && !(m.screen == terminalScreen && m.terminalRawInput) {
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
		if m.drawerOpen {
			return m.updateDrawer(message)
		}
		return m.updateComposer(message)
	case attachmentConfirmScreen:
		return m.updateAttachmentConfirmation(message)
	case runningScreen:
		if m.drawerOpen {
			return m.updateDrawer(message)
		}
		return m.updateRunning(message)
	case terminalScreen:
		return m.updateAttachedTerminal(message)
	case reviewScreen:
		return m.updateReview(message)
	case transcriptScreen:
		return m.updateTranscript(message)
	case helpScreen:
		return m.updateHelp(message)
	case recentScreen:
		return m.updateRecentRuns(message)
	case threadScreen:
		return m.updateThreadTree(message)
	case localModelsScreen:
		return m.updateLocalModels(message)
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
		m.quitAfterRun = false
		m.screen = composeScreen
		m.notice = notice{text: "Attachment send cancelled. No file bytes were sent.", kind: noticeInfo}
		return m, m.focusField()
	default:
		return m, nil
	}
}

func (m Model) updateComposer(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if message.String() == "ctrl+c" && m.oauthLogin != nil {
		m.oauthCancel()
		m.oauthLogin.Cancel()
		m.notice = notice{text: "OAuth login cancellation requested.", kind: noticeInfo}
		return m, nil
	}
	if m.focus == taskField && m.vimCommand != "" {
		return m.updateVimCommand(message, false)
	}
	if m.focus == taskField && m.vim == vimInsert && message.String() == "esc" {
		m.vim = vimNormal
		m.notice = notice{text: "Vim Normal mode. Press i or a to edit; Enter sends.", kind: noticeInfo}
		return m, nil
	}
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
		case "enter":
			return m.executeSelectedCommand()
		case "tab":
			m.completeSelectedCommand()
			return m, nil
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
			if m.focus != taskField {
				m.drawerOpen = true
				m.drawerSection = drawerRuntime
				m.resizeInputs()
				m.syncTranscript(m.followTranscript)
			}
			return m, m.focusField()
		}
	}
	switch message.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "ctrl+o":
		return m.openRecentRuns()
	case "pgup":
		m.pageTranscript(false)
		return m, nil
	case "pgdown":
		m.pageTranscript(true)
		return m, nil
	case "home":
		m.transcriptTop()
		return m, nil
	case "end":
		m.transcriptBottom()
		return m, nil
	case "?":
		if m.focus == taskField && strings.TrimSpace(m.task.Value()) == "" {
			m.task.SetValue("/")
			m.commandIndex = 0
			return m, nil
		}
	case "enter":
		if m.focus == taskField && m.vim == vimOff {
			return m.startRun()
		}
	case "ctrl+r":
		if m.focus == taskField && m.vim == vimNormal {
			return m.updateVimNormal(message)
		}
		return m.startRun()
	}

	if m.focus == taskField && m.vim == vimNormal {
		return m.updateVimNormal(message)
	}

	switch message.String() {
	case "tab", "shift+tab":
		if m.resumeStatePath != "" {
			m.notice = notice{text: "A continuation inherits the model and verification policy from its original run.", kind: noticeInfo}
			return m, nil
		}
		m.focus = nextField(m.focus, message.String() == "shift+tab")
		if m.focus != taskField {
			m.drawerOpen = true
			m.drawerSection = drawerRuntime
			m.resizeInputs()
			m.syncTranscript(m.followTranscript)
		}
		return m, m.focusField()
	}

	var command tea.Cmd
	switch m.focus {
	case taskField:
		command = m.updateTask(message)
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
		previousProvider := strings.TrimSpace(m.provider.Value())
		m.provider, command = m.provider.Update(message)
		if m.delegateRuntime != "" && strings.TrimSpace(m.provider.Value()) != previousProvider {
			m.delegateRuntime = ""
		}
		m.normalizeDropdownSelection()
		m.persistDraft()
		m.refreshPreflight()
	}
	return m, command
}

func (m *Model) updateTask(message tea.Msg) tea.Cmd {
	before := m.vimSnapshot()
	var command tea.Cmd
	m.task, command = m.task.Update(message)
	if m.vim != vimOff {
		m.rememberVimEdit(before)
	}
	m.normalizeCommandSelection()
	m.contextClosed = false
	m.normalizeContextSelection()
	m.persistDraft()
	m.refreshPreflight()
	m.syncVimLineNumbers()
	return command
}

func (m Model) updateVimNormal(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.updateVimNormalMode(message)
}

func (m Model) updateRunning(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pendingApproval != nil {
		return m.updateCommandApproval(message)
	}
	if m.vimCommand != "" {
		return m.updateVimCommand(message, true)
	}
	if m.vim == vimInsert && message.String() == "esc" {
		m.vim = vimNormal
		m.notice = notice{text: "Vim Normal mode. Press i or a to edit; Enter steers the active run.", kind: noticeInfo}
		return m, nil
	}
	if message.Type == tea.KeyCtrlAt {
		m.contextClosed = false
		m.normalizeContextSelection()
		if !m.contextCompletionVisible() {
			m.notice = notice{text: "Type @ followed by a repository path to open path suggestions.", kind: noticeInfo}
		}
		return m, nil
	}
	if m.commandPaletteVisible() {
		switch message.String() {
		case "up", "ctrl+p":
			m.moveCommandSelection(-1)
			return m, nil
		case "down", "ctrl+n":
			m.moveCommandSelection(1)
			return m, nil
		case "tab":
			if m.selectedCommandIsExact() {
				return m.queueCurrentInput()
			}
			m.completeSelectedCommand()
			return m, nil
		case "enter":
			matches := m.matchingCommands()
			if len(matches) == 0 {
				return m, nil
			}
			selected := matches[m.commandIndex].name
			m.task.SetValue(selected)
			if runningLocalCommand(selected) {
				return m.executeSelectedCommand()
			}
			return m.queueCurrentInput()
		case "esc":
			m.task.Reset()
			m.commandIndex = 0
			m.notice = notice{text: "Command palette dismissed.", kind: noticeInfo}
			return m, nil
		}
	}
	if m.contextCompletionVisible() {
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
	switch message.String() {
	case "ctrl+c":
		if m.execution != nil && !m.cancelling {
			m.execution.cancel()
			m.cancelling = true
			m.notice = notice{text: "Cancellation requested. Waiting for the current operation to stop...", kind: noticeInfo}
		}
		return m, nil
	case "pgup":
		m.pageTranscript(false)
		return m, nil
	case "pgdown":
		m.pageTranscript(true)
		return m, nil
	case "home":
		m.transcriptTop()
		return m, nil
	case "end":
		m.transcriptBottom()
		return m, nil
	case "?":
		if strings.TrimSpace(m.task.Value()) == "" {
			m.task.SetValue("/")
			m.commandIndex = 0
			return m, nil
		}
	case "tab":
		return m.queueCurrentInput()
	case "ctrl+r":
		if m.vim == vimNormal {
			return m.updateVimNormal(message)
		}
		return m.steerCurrentInput()
	case "enter":
		if m.vim != vimInsert {
			return m.steerCurrentInput()
		}
	}
	if m.vim == vimNormal {
		return m.updateVimNormal(message)
	}
	command := m.updateTask(message)
	return m, command
}

func (m Model) updateCommandApproval(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "y", "enter":
		m.resolvePendingApproval(tools.CommandAllowOnce)
		m.notice = notice{text: "Command allowed once.", kind: noticeInfo}
		return m, nil
	case "a":
		m.resolvePendingApproval(tools.CommandAllowAlways)
		m.notice = notice{text: "Command allowed for the rest of this thread.", kind: noticeInfo}
		return m, nil
	case "n", "d":
		m.resolvePendingApproval(tools.CommandDeny)
		m.notice = notice{text: "Command denied.", kind: noticeInfo}
		return m, nil
	case "ctrl+c":
		m.resolvePendingApproval(tools.CommandDeny)
		if m.execution != nil && !m.cancelling {
			m.execution.cancel()
			m.cancelling = true
			m.notice = notice{text: "Cancellation requested. Waiting for the current operation to stop...", kind: noticeInfo}
		}
		return m, nil
	default:
		return m, nil
	}
}

func runningLocalCommand(command string) bool {
	switch command {
	case "/queue", "/dequeue", "/clear-queue", "/tree":
		return true
	default:
		return false
	}
}

func (m Model) updateVimCommand(message tea.KeyMsg, running bool) (tea.Model, tea.Cmd) {
	return m.updateVimExCommand(message, running)
}

func (m Model) updateReview(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k", "ctrl+p":
		m.scrollReviewDiff(-1)
		return m, nil
	case "down", "j", "ctrl+n":
		m.scrollReviewDiff(1)
		return m, nil
	case "pgup", "ctrl+u":
		m.scrollReviewDiff(-m.reviewDiffPageSize())
		return m, nil
	case "pgdown", "ctrl+d":
		m.scrollReviewDiff(m.reviewDiffPageSize())
		return m, nil
	case "home", "g":
		m.diffOffset = 0
		return m, nil
	case "end", "G":
		m.jumpReviewDiffToEnd()
		return m, nil
	case "f":
		if m.diffMode == focusedDiffDisplay {
			m.diffMode = fullDiffDisplay
			m.notice = notice{text: "Showing the complete raw patch. Press f to return to focused review.", kind: noticeInfo}
		} else {
			m.diffMode = focusedDiffDisplay
			m.notice = notice{text: "Showing focused review with duplicate blocks collapsed. Press f for the full patch.", kind: noticeInfo}
		}
		m.diffOffset = 0
		return m, nil
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
	case "y":
		return m.openThreadTree(reviewScreen)
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
	var (
		threads []journal.RecentThread
		err     error
	)
	if m.recentAll {
		threads, err = journal.ListAllRecentThreads(m.config.StateDir, 20)
	} else {
		threads, err = journal.ListRecentThreads(m.config.StateDir, m.config.RepositoryPath, 20)
	}
	if err != nil {
		m.notice = notice{text: "Load recent threads: " + err.Error(), kind: noticeError}
		return m, nil
	}
	if len(threads) == 0 {
		message := "No retained threads are available for this repository."
		if m.recentAll {
			message = "No retained threads are available in this state directory."
		}
		m.notice = notice{text: message, kind: noticeInfo}
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
	case "a":
		m.recentAll = !m.recentAll
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

func (m Model) updateThreadTree(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "c":
		if m.resumeStatePath == "" {
			m.notice = notice{text: "Continue this thread before cloning its active branch.", kind: noticeInfo}
			return m, nil
		}
		return m.beginFork(m.resumeStatePath)
	case "f":
		if len(m.threadTurns) == 0 {
			return m, nil
		}
		selected := m.threadTurns[min(max(0, m.threadIndex), len(m.threadTurns)-1)]
		return m.beginFork(selected.StatePath)
	case "esc", "q", "y":
		m.screen = m.threadReturn
		if m.screen == composeScreen || m.screen == runningScreen {
			m.focus = taskField
			return m, m.focusField()
		}
	case "r", "ctrl+r":
		return m.openThreadTree(m.threadReturn)
	case "up", "ctrl+p":
		if m.threadIndex > 0 {
			m.threadIndex--
		}
	case "down", "ctrl+n":
		if m.threadIndex < len(m.threadTurns)-1 {
			m.threadIndex++
		}
	}
	return m, nil
}
