package workapp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/rattles"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workrun"
)

func (m Model) Update(messageValue tea.Msg) (tea.Model, tea.Cmd) {
	switch value := messageValue.(type) {
	case catalogLoaded:
		return m.finishCatalog(value)
	case receiptChecked:
		if value.Result.RequestID != m.receiptPendingID {
			return m, nil
		}
		m.receiptBusy = false
		m.receiptPendingID = ""
		if value.ConversationID == m.conversation {
			m.receiptResult = &value.Result
		}
		return m, nil
	case overviewRefreshed:
		if value.Generation != m.overviewGeneration {
			return m, nil
		}
		m.overviewBusy = false
		if value.Err != nil {
			m.overviewIssue = value.Err.Error()
		} else {
			m.overview = value.View
			m.overviewIssue = ""
		}
		return m, nil
	case controlExternalDone:
		m.controlRootArgs = nil
		if value.Err != nil {
			m.controlResultNotice = value.Err.Error()
		} else {
			m.controlResultNotice = "Review finished. The overview will reflect the retained result."
		}
		return m, tea.Batch(m.controlCommand("status", nil), m.refreshOverviewCommand())

	case connectionTick:
		pending := m.pendingConnection() != ""
		m.connectionPolling = pending
		var commands []tea.Cmd
		if pending {
			commands = append(commands, nextConnectionTick())
		}
		if m.section == "controls" && m.controlFamily == "connections" && !m.controlBusy && !m.controlBackground && m.controlForm == nil {
			commands = append(commands, m.refreshControlBackground())
		}
		return m, tea.Batch(commands...)
	case controlDone:
		if value.Generation != m.controlGeneration {
			return m, nil
		}
		m.controlBusy = false
		m.controlBackground = false
		if m.controlCancel != nil {
			m.controlCancel()
			m.controlCancel = nil
		}
		if value.Err != nil {
			m.sectionNotice = value.Err.Error()
		} else {
			selection := 0
			if m.controlFamily == "overview" || m.controlFamily == "connections" {
				selection = preservedControlSelection(m.controlScreen, value.Screen, m.selected)
			}
			m.controlScreen = value.Screen
			if m.controlFamily == "connections" {
				m.controlRootArgs = nil
				if value.Screen.Location != "" {
					m.controlRootArgs = []string{value.Screen.Location}
				}
			}
			m.selected = selection
			m.sectionNotice = ""
			m.clearRemovedData(value.Screen)
			m.refreshBridgeConversations()
		}
		if m.pendingConnection() != "" && !m.connectionPolling {
			m.connectionPolling = true
			return m, nextConnectionTick()
		}
		return m, nil
	case followupSaved:
		return m.finishFollowup(value)
	case recallStatusDone:
		if value.Err != nil {
			m.sectionNotice = "Recall status unavailable: " + value.Err.Error()
		} else {
			m.recall = value.Status
			m.sectionNotice = ""
		}
		return m, nil
	case recallProgress:
		m.sectionNotice = string(value)
		return m, waitWorkEvent(m.recallEvents)
	case recallInstallDone:
		m.recallBusy = false
		if m.recallCancel != nil {
			m.recallCancel()
		}
		m.recallCancel = nil
		m.recallEvents = nil
		if value.Err != nil {
			m.sectionNotice = "Download stopped: " + value.Err.Error()
			return m, nil
		}
		m.sectionNotice = "Local recall model installed."
		return m, m.recallStatusCommand()
	case checksRefreshed:
		if value.Err == nil {
			m.checksIssue = ""
			m.checks, m.config.SupervisorRunning = value.Checks, value.Running
			m.config.Jobs = nil
			for _, check := range value.Checks {
				m.config.Jobs = append(m.config.Jobs, check.Definition)
			}
			if m.checkDetail != nil {
				for _, check := range value.Checks {
					if check.ID == m.checkDetail.ID {
						copy := check
						m.checkDetail = &copy
						break
					}
				}
			}
		} else {
			m.checksIssue = "Check health is unavailable: " + value.Err.Error()
			if m.section == "followup" {
				m.sectionNotice = "Health unavailable: " + value.Err.Error()
			}
		}
		return m, nil
	case checksTick:
		m.refreshChatUpdates()
		m.refreshChatMemories()
		m.refreshBridgeConversations()
		m.refreshBridgeFollowing()
		if m.config.ReadChecks == nil {
			return m, nil
		}
		commands := []tea.Cmd{m.refreshChecksCommand(), m.refreshOverviewCommand(), nextChecksTick()}
		if m.section == "controls" && m.controlFamily == "overview" && !m.controlBusy && !m.controlBackground && m.controlForm == nil {
			commands = append(commands, m.refreshControlBackground())
		}
		return m, tea.Batch(commands...)
	case checkActionDone:
		m.sectionNotice = value.Text
		if value.Err != nil {
			m.sectionNotice = value.Err.Error()
		}
		return m, m.refreshChecksCommand()
	}
	if m.models != nil {
		return m.updateModelPanel(messageValue)
	}
	switch value := messageValue.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = value.Width, value.Height
	case loadingTickMsg:
		return m.updateLoading(value)
	case agent.Event:
		return m.updateAgentEvent(value)
	case *workrun.Operation:
		m.operation = value
		return m, waitWorkEvent(m.live)
	case workrun.Interaction:
		return m.updateInteraction(value)
	case runDone:
		next, command := m.updateRunDone(value)
		model := next.(Model)
		model.refreshBridgeConversations()
		return model, command
	case bundleActionDone:
		return m.updateBundleActionDone(value)
	case connectorActionDone:
		return m.updateConnectorActionDone(value)
	case composerEditorDone:
		return m.updateComposerEditorDone(value)
	case tea.KeyMsg:
		return m.updateKey(value)
	}
	return m, nil
}

func (m Model) updateModelPanel(messageValue tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := messageValue.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
	}
	next, command := m.models.Update(messageValue)
	m.models = next.(ModelPanel)
	if !m.models.Closed() {
		return m, command
	}
	m.closeModelPanel()
	if m.section == "memory" {
		return m, tea.Batch(tea.ClearScreen, m.recallStatusCommand())
	}
	return m, tea.ClearScreen
}

func (m Model) updateLoading(value loadingTickMsg) (tea.Model, tea.Cmd) {
	if !m.running || value.run != m.loadingRun {
		return m, nil
	}
	m.loadingFrame = (m.loadingFrame + 1) % len(rattles.BrailleDots.Frames)
	return m, m.nextLoadingTick()
}

func (m Model) updateAgentEvent(value agent.Event) (tea.Model, tea.Cmd) {
	if m.cancelling {
		return m, waitWorkEvent(m.live)
	}
	m.activity.Record(value)
	if value.Kind == agent.EventSubagent && value.TaskID != "" {
		var task struct{ ID, Role, Status, Error string }
		if json.Unmarshal([]byte(value.Text), &task) == nil {
			if m.tasks == nil {
				m.tasks = map[string]string{}
			}
			m.tasks[task.ID] = task.Role + ": " + task.Status
			m.status = specialistProgress(task.Role, task.Status)
			if task.Error != "" {
				m.messages = append(m.messages, noticeMessage("Part of this work needs review", "Gator retained progress. Inspect /activity for the outcome or /tasks for diagnostics.", "warning"))
			}
			return m, waitWorkEvent(m.live)
		}
	}
	m.status = workProgress(value)
	return m, waitWorkEvent(m.live)
}

func (m Model) updateInteraction(value workrun.Interaction) (tea.Model, tea.Cmd) {
	if m.cancelling {
		return m, waitWorkEvent(m.live)
	}
	if m.interaction != nil {
		m.pendingInteractions = append(m.pendingInteractions, value)
		return m, waitWorkEvent(m.live)
	}
	m.interaction = &value
	m.messages = append(m.messages, approvalMessage(value))
	return m, waitWorkEvent(m.live)
}

func (m Model) updateRunDone(value runDone) (tea.Model, tea.Cmd) {
	if value.WorkID != "" {
		m.lastWorkID = value.WorkID
	} else if m.operation != nil {
		m.lastWorkID = m.operation.RunID
	}
	wasCancelling := m.cancelling
	m.finishRequest(value, wasCancelling)
	if m.cancel != nil {
		m.cancel()
	}
	m.cancel = nil
	m.cancelling = false
	m.operation = nil
	m.interaction = nil
	m.pendingInteractions = nil
	m.live = nil
	m.running = false
	for _, change := range value.AppChanges {
		switch change.Setting {
		case "theme":
			var v struct {
				Theme string `json:"theme"`
			}
			if json.Unmarshal(change.Value, &v) == nil {
				m.theme = normalizeTheme(v.Theme)
			}
		case "connections":
			var v struct {
				IDs []string `json:"ids"`
			}
			if json.Unmarshal(change.Value, &v) == nil {
				m.options.ConnectorIDs = append([]string{}, v.IDs...)
			}
		}
		text := change.Summary
		if text == "" {
			text = "Saved " + strings.ReplaceAll(change.Setting, "_", " ") + ". The change applies to new requests."
		}
		m.messages = append(m.messages, reportMessage("Gator app updated", text, "success"))
	}
	if len(value.AppChanges) > 0 {
		m.refreshModelStatus()
		if m.config.ReadDesktop != nil {
			if preferences, err := m.config.ReadDesktop(); err == nil {
				m.desktop = preferences
			}
		}
	}
	activity := m.activity
	activityState := "completed"
	if value.Error != "" {
		activityState = "failed"
	}
	if wasCancelling {
		activityState = "stopped"
	}
	activity.Finish(time.Now(), activityState, value.FinalText)
	if value.Activity != nil {
		activity = value.Activity.Clone()
	}
	m.activity = nil
	m.runStarted = time.Time{}
	if value.ConversationID != "" {
		m.conversation = value.ConversationID
		m.rememberChat()
		if m.primary && m.primaryID == "" && m.config.BindPrimaryConversation != nil {
			if err := m.config.BindPrimaryConversation(value.ConversationID); err != nil {
				m.primary = false
				m.bridgeNotice = "Your conversation is saved, but couldn't become the main conversation: " + err.Error()
			} else {
				m.primaryID = value.ConversationID
			}
		}
	}
	if m.quitting {
		return m, tea.Quit
	}
	if value.RevisionID != "" {
		m.revision = value.RevisionID
	}
	if value.SnapshotID != "" {
		m.snapshot = value.SnapshotID
	}
	responseIndex := len(m.messages)
	uncertainRequest := false
	for _, receipt := range value.Bundle.Actions {
		uncertainRequest = uncertainRequest || receipt.Status == "unknown"
	}
	if value.Error != "" && uncertainRequest {
		m.messages = append(m.messages, noticeMessage("Request outcome unconfirmed", "The service may already have acted. Check the saved request receipt before trying again.", "warning"))
	} else if wasCancelling && value.Error != "" && strings.Contains(strings.ToLower(value.Error), "cancel") {
		m.messages = append(m.messages, noticeMessage("Work interrupted", "Review any retained work in /history.", "warning"))
	} else if value.Error != "" {
		m.messages = append(m.messages, noticeMessage("Work failed", explainRunFailure(value.Error, m.modelStatus), "error"))
	} else {
		text := value.FinalText
		if text == "" {
			text = "Finished the Work revision."
		}
		m.messages = append(m.messages, message{role: "Gator", text: text})
	}
	// A failed or stopped task can still have verified partial files and
	// external request evidence. Keep that bundle attached to its owning turn
	// so an uncertain send can be inspected without becoming an orphan receipt.
	if value.Bundle.Path != "" {
		bundle := cloneBundleSummary(value.Bundle)
		m.messages = append(m.messages, message{role: "Deliverables", bundle: &bundle})
	}
	if responseIndex < len(m.messages) {
		m.messages[responseIndex].activity = activity
		m.messages[responseIndex].runID = value.WorkID
	}
	m.refreshChatMemories()
	m.refreshActionRecoveries()
	m.lastOutput = value.OutputPath
	m.lastBundle = value.Bundle
	if m.lastWorkID != "" {
		m.messages = append(m.messages, message{role: "Context", result: contextLink(m.lastWorkID)})
	}
	if m.revisionStatus() != "" {
		m.status = m.revisionStatus()
	}
	if wasCancelling {
		m.status = "Run stopped"
	}
	m.scroll = 0
	if value.AppScreen != nil {
		m.section = "controls"
		m.home = false
		m.controlFamily = value.AppScreenFamily
		m.controlScreen = *value.AppScreen
		m.controlForm = nil
		m.controlReview = false
		m.controlBusy = false
		m.controlBackground = false
		m.controlRootArgs = nil
		m.selected = 0
		if value.AppScreenAction != "" {
			for _, action := range m.controlScreen.Actions {
				if action.Action == value.AppScreenAction {
					copy := action
					copy.Fields = append([]ControlField{}, action.Fields...)
					m.controlForm = &copy
					m.controlField = 0
					break
				}
			}
		}
		return m, nil
	}
	if value.Error == "" && !wasCancelling && len(m.queue) > 0 && m.config.Run != nil && !m.requestsRecovered && m.requestsIssue == "" && !m.queue[0].interrupted {
		return m.runQueued(0)
	}
	if len(m.queue) > 0 && (value.Error != "" || wasCancelling) {
		m.status = fmt.Sprintf("Run stopped · %d queued prompt(s) paused", len(m.queue))
	}
	return m, nil
}

func (m Model) updateBundleActionDone(value bundleActionDone) (tea.Model, tea.Cmd) {
	if value.err != nil {
		m.messages = append(m.messages, noticeMessage("Delivery failed", value.err.Error(), "error"))
	} else {
		m.messages = append(m.messages, reportMessage("Delivery", value.text, "success"))
	}
	m.status = ""
	m.scroll = 0
	return m, nil
}

func (m Model) updateConnectorActionDone(value connectorActionDone) (tea.Model, tea.Cmd) {
	m.status = ""
	if value.err != nil {
		m.messages = append(m.messages, noticeMessage("Connector", "Connector "+value.action+" did not finish: "+value.err.Error(), "error"))
		m.scroll = 0
		return m, nil
	}
	text := strings.TrimSpace(value.text)
	switch value.action {
	case "setup":
		text += "\nNext: /connector login " + value.id + " prompt\nUse the prompt form when your Google desktop client has a client secret."
	case "login":
		m.options.ConnectorIDs = appendUnique(m.options.ConnectorIDs, value.id)
		text = "Connector " + value.id + " is authenticated and selected for this conversation."
	case "logout":
		m.options.ConnectorIDs = removeString(m.options.ConnectorIDs, value.id)
	case "delete":
		m.options.ConnectorIDs = removeString(m.options.ConnectorIDs, value.id)
	}
	if text == "" {
		text = "Connector " + value.action + " finished for " + value.id + "."
	}
	m.messages = append(m.messages, reportMessage("Connector", text, "success"))
	m.scroll = 0
	if m.section == "controls" && m.controlFamily == "recovery" {
		return m, m.controlCommand("status", m.controlRootArgs)
	}
	return m, nil
}

func (m Model) updateKey(value tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.controlBusy {
		if value.String() == "ctrl+c" {
			if m.controlCancel != nil {
				m.controlCancel()
			}
			return m, tea.Quit
		}
		return m.updateControls(value)
	}
	if m.followupSaving {
		if value.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}
	if m.productHelp {
		if value.String() == "esc" || value.String() == "f1" {
			m.productHelp = false
		} else if value.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}
	if value.String() == "f1" && (m.section != "" || m.launcher) {
		m.productHelp = true
		return m, nil
	}
	if m.pendingBundleAction != nil {
		return m.updateBundleConfirmation(value)
	}
	if value.String() == "ctrl+c" {
		if m.running && m.cancel != nil {
			m.interruptRun()
			return m, nil
		}
		return m, tea.Quit
	}
	if value.String() == "esc" && m.launcher {
		return m.updateLauncher(value)
	}
	if value.String() == "esc" && m.section != "" {
		return m.updateSectionKey(value)
	}
	if value.String() == "esc" && m.running && m.cancel != nil {
		m.interruptRun()
		return m, nil
	}
	if value.Type == tea.KeyCtrlX && !m.running {
		cmd := m.openConversationPicker()
		return m, cmd
	}
	// Tab and Ctrl+I share a terminal key code. Forms own Tab; the workspace
	// shortcut is available only from the chat composer.
	if value.Type == tea.KeyCtrlI && !m.running && m.section == "" && !m.launcher {
		m.openSourceMenu()
		return m, nil
	}
	if value.Type == tea.KeyCtrlB && !m.running {
		m.launcher = false
		m.section = "inbox"
		m.inboxDetail, m.selected = "", 0
		m.sectionNotice = ""
		if err := m.refreshInbox(); err != nil {
			m.sectionNotice = "Inbox refresh failed: " + err.Error()
		}
		return m, nil
	}
	if value.String() == "ctrl+o" && m.config.Controls != nil && !m.running {
		cmd := m.openControls("overview", nil)
		return m, cmd
	}
	if value.String() == "ctrl+p" {
		m.openCommandPalette()
		return m, nil
	}
	if m.launcher {
		return m.updateLauncher(value)
	}
	if m.section != "" {
		return m.updateSectionKey(value)
	}
	if value.Type == tea.KeyCtrlG {
		return m.openComposerEditor()
	}
	if m.home {
		return m.updateHome(value)
	}
	switch value.String() {
	case "pgup":
		m.scroll += max(1, m.height/2)
		return m, nil
	case "pgdown":
		m.scroll -= max(1, m.height/2)
		if m.scroll < 0 {
			m.scroll = 0
		}
		return m, nil
	}
	if m.running {
		return m.updateRunningKey(value)
	}
	return m.updateComposerKey(value)
}

func (m Model) updateSectionKey(value tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.section {
	case "controls":
		return m.updateControls(value)
	case "followup":
		return m.updateFollowupKey(value)
	case "memory":
		return m.updateRecallKey(value)
	case "context":
		return m.updateContextKey(value)
	case "inbox":
		return m.updateInboxKey(value)
	case "appearance":
		return m.updateAppearanceKey(value)
	case "effort":
		switch value.String() {
		case "esc":
			m.section = ""
		case "up":
			if m.selected > 0 {
				m.selected--
			}
		case "down":
			if m.selected < 2 {
				m.selected++
			}
		case "enter":
			_ = m.setWorkEffort([]string{"low", "standard", "high"}[m.selected])
			m.section = ""
			m.messages = append(m.messages, message{role: "Gator", text: "Effort set to " + effortName(m.options.MaxSteps) + ".", result: &uisnapshot.Result{Kind: "notice", Title: "Effort", Tone: "success", Summary: "Set to " + effortName(m.options.MaxSteps)}})
		}
		return m, nil
	case "history":
		return m.updateHistorySectionKey(value)
	case "learnings":
		return m.updateLearningsSectionKey(value)
	default:
		if value.String() == "esc" {
			m.section = ""
		}
		return m, nil
	}
}

func (m Model) updateHistorySectionKey(value tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.historyDetail != nil {
		switch value.String() {
		case "esc", "backspace":
			m.historyDetail, m.sectionNotice = nil, ""
		case "r":
			m.reviewHistoryDetail()
		case "c":
			m.resumeHistoryConversation()
		case "t":
			return m.prepareHistoryRetry()
		}
		return m, nil
	}
	switch value.String() {
	case "esc":
		m.section = ""
	case "up":
		if m.selected > 0 {
			m.selected--
		}
	case "down":
		if m.selected+1 < len(m.historyItems) {
			m.selected++
		}
	case "enter":
		m.selectHistoryItem()
	case "f":
		m.cycleHistoryFilter()
	case "r":
		selectedID := ""
		if m.selected >= 0 && m.selected < len(m.historyItems) {
			selectedID = m.historyItems[m.selected].Record.ID
		}
		m.refreshGlobalHistory(selectedID)
	}
	return m, nil
}

func (m Model) updateLearningsSectionKey(value tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.learningForm != nil {
		return m.updateLearningFormKey(value)
	}
	if m.learningDetail != nil {
		switch value.String() {
		case "esc", "backspace":
			m.learningDetail, m.sectionNotice = nil, ""
		case "a":
			if m.learningDetail.Status == learning.Candidate {
				m.mutateLearning("approve")
			} else if m.learningDetail.Status == learning.Disabled {
				m.mutateLearning("enable")
			} else {
				m.sectionNotice = "Only a candidate can be approved or a disabled learning enabled."
			}
		case "d":
			m.mutateLearning("disable")
		case "r":
			m.mutateLearning("reject")
		case "e":
			m.openEditLearning()
		case "n":
			m.openNewLearning()
		}
		return m, nil
	}
	switch value.String() {
	case "esc":
		m.section = ""
	case "up":
		if m.selected > 0 {
			m.selected--
		}
	case "down":
		if m.selected+1 < len(m.learningItems) {
			m.selected++
		}
	case "enter":
		m.selectLearningItem()
	case "f":
		m.cycleLearningFilter()
	case "n":
		m.openNewLearning()
	case "r":
		selectedID := ""
		if m.selected >= 0 && m.selected < len(m.learningItems) {
			selectedID = m.learningItems[m.selected].ID
		}
		m.refreshLearnings(selectedID)
	}
	return m, nil
}

func (m Model) updateRunningKey(value tea.KeyMsg) (tea.Model, tea.Cmd) {
	if value.String() == "/" && m.input == "" {
		m.openSlashPalette()
		return m, nil
	}
	switch value.String() {
	case "up":
		m.recallPreviousPrompt()
	case "down":
		m.recallNextPrompt()
	case "enter":
		return m.submitRunningInput()
	case "backspace":
		m.resetPromptHistoryNavigation()
		runes := []rune(m.input)
		if len(runes) > 0 {
			m.input = string(runes[:len(runes)-1])
		}
	default:
		if value.Type == tea.KeyRunes || value.String() == " " {
			m.resetPromptHistoryNavigation()
			m.input += value.String()
		}
	}
	return m, nil
}

func (m Model) submitRunningInput() (tea.Model, tea.Cmd) {
	prompt := strings.TrimSpace(m.input)
	if prompt == "" {
		return m, nil
	}
	m.input = ""
	if prompt == "exit" || prompt == "/exit" || prompt == "/quit" {
		if m.cancel != nil {
			m.quitting = true
			m.cancel()
			m.status = "Cancelling before exit…"
			return m, nil
		}
		return m, tea.Quit
	}
	if prompt == "/tasks" {
		var lines []string
		for id, status := range m.tasks {
			lines = append(lines, id+" "+status)
		}
		sort.Strings(lines)
		items := make([]uisnapshot.ResultItem, 0, len(lines))
		for _, line := range lines {
			items = append(items, uisnapshot.ResultItem{Label: line})
		}
		summary := fmt.Sprintf("%d specialists", len(items))
		if len(items) == 0 {
			summary = "No specialists are active."
		}
		m.messages = append(m.messages, message{role: "Specialists", text: strings.Join(lines, "\n"), result: &uisnapshot.Result{Kind: "list", Title: "Internal specialists", Summary: summary, Items: items}})
		return m, nil
	}
	if strings.HasPrefix(prompt, "/cancel-task ") && m.operation != nil {
		err := m.operation.CancelTask(strings.TrimSpace(strings.TrimPrefix(prompt, "/cancel-task ")))
		if err != nil {
			m.status = err.Error()
		}
		return m, nil
	}
	if (prompt == "/steer-task" || strings.HasPrefix(prompt, "/steer-task ")) && m.operation != nil {
		remainder := strings.TrimSpace(strings.TrimPrefix(prompt, "/steer-task"))
		parts := strings.Fields(remainder)
		if len(parts) < 2 {
			m.status = "usage: /steer-task TASK_ID MESSAGE"
			return m, nil
		}
		id := parts[0]
		instruction := strings.TrimSpace(strings.TrimPrefix(remainder, id))
		if err := m.operation.SteerTask(id, strings.TrimSpace(instruction)); err != nil {
			m.status = err.Error()
		} else {
			m.status = "Steering queued for specialist " + id
		}
		return m, nil
	}
	if strings.HasPrefix(prompt, "/steer ") && m.operation != nil {
		err := m.operation.Steer(strings.TrimSpace(strings.TrimPrefix(prompt, "/steer ")))
		if err != nil {
			m.status = err.Error()
		} else {
			m.status = "Steering submitted to current task"
		}
		return m, nil
	}
	if (prompt == "/approve" || prompt == "/deny") && m.operation != nil && m.interaction != nil {
		err := m.operation.Respond(m.interaction.ID, prompt == "/approve")
		if err != nil {
			m.status = err.Error()
			if err.Error() != "interaction is no longer pending" {
				return m, nil
			}
		}
		for index := len(m.messages) - 1; index >= 0; index-- {
			item := &m.messages[index]
			if item.result != nil && item.result.Kind == "approval" && len(item.result.Fields) > 0 && item.result.Fields[0].Value == fmt.Sprint(m.interaction.ID) {
				if err != nil {
					item.result.Tone, item.result.Footer = "muted", "No longer pending"
				} else if prompt == "/approve" {
					item.result.Tone, item.result.Footer = "success", "Approved"
				} else {
					item.result.Tone, item.result.Footer = "muted", "Denied"
				}
				break
			}
		}
		m.interaction = nil
		if len(m.pendingInteractions) > 0 {
			next := m.pendingInteractions[0]
			m.pendingInteractions = m.pendingInteractions[1:]
			m.interaction = &next
			m.messages = append(m.messages, approvalMessage(next))
		}
		return m, nil
	}
	if prompt == "/cancel" && m.cancel != nil {
		m.interruptRun()
		return m, nil
	}
	if strings.HasPrefix(prompt, "/") {
		return m.runLocalCommand(prompt)
	}
	if len(m.queue) >= 16 {
		m.messages = append(m.messages, noticeMessage("Queue full", "The local queue is full (16 prompts).", "warning"))
		return m, nil
	}
	m.queueSequence++
	queue := append(append([]queuedRun(nil), m.queue...), queuedRun{id: m.queueSequence, prompt: prompt, options: m.submissionOptions()})
	if err := m.persistQueue(queue); err != nil {
		m.bridgeNotice = "Message not queued: " + err.Error()
		if m.bridgeUploadResult != nil {
			m.bridgeUploadResult.Error = m.bridgeNotice
		}
		return m, nil
	}
	m.recordPrompt(prompt)
	m.options.Attachments = nil
	m.options.Uploads = nil
	m.options.ExcludedKnowledgeIDs = nil
	m.options.RefreshSource = false
	m.status = fmt.Sprintf("Working from an immutable snapshot… · %d queued", len(m.queue))
	return m, nil
}

func (m *Model) interruptRun() {
	if m.cancel == nil || m.cancelling {
		return
	}
	m.cancelling = true
	m.cancel()
	m.status = "Stopping the current run; retaining evidence…"
}

func (m Model) updateComposerKey(value tea.KeyMsg) (tea.Model, tea.Cmd) {
	if value.String() == "/" && m.input == "" {
		m.openSlashPalette()
		return m, nil
	}
	switch value.String() {
	case "up":
		m.recallPreviousPrompt()
	case "down":
		m.recallNextPrompt()
	case "enter":
		return m.submitComposerInput()
	case "backspace":
		m.resetPromptHistoryNavigation()
		runes := []rune(m.input)
		if len(runes) > 0 {
			m.input = string(runes[:len(runes)-1])
		}
	case "esc":
		m.openCommandPalette()
	default:
		if value.Type == tea.KeyRunes || value.String() == " " {
			m.resetPromptHistoryNavigation()
			m.input += value.String()
		}
	}
	return m, nil
}

func (m Model) submitComposerInput() (tea.Model, tea.Cmd) {
	prompt := strings.TrimSpace(m.input)
	if prompt == "" {
		return m, nil
	}
	m.input = ""
	if prompt == "exit" {
		return m.runLocalCommand("exit")
	}
	if strings.HasPrefix(prompt, "/") {
		return m.runLocalCommand(prompt)
	}
	if m.firstRun {
		m.pendingPrompt = prompt
		return m.openModels()
	}
	if m.conversation == "" && m.config.CreateConversation != nil {
		options := cloneRunOptions(m.options)
		if !m.beginChat(m.source) {
			if m.bridgeUploadResult != nil {
				m.bridgeUploadResult.Error = m.bridgeNotice
			}
			return m, nil
		}
		m.options = options
	}
	m.messages = append(m.messages, message{role: "You", text: prompt, createdAt: time.Now().UTC(), replyToUpdateID: m.submissionReply})
	m.recordPrompt(prompt)
	m.home = false
	m.running = true
	m.status = "Working from an immutable snapshot…"
	if m.config.Run == nil {
		m.running = false
		m.messages = append(m.messages, noticeMessage("Work unavailable", "Work execution is unavailable in this build.", "error"))
		return m, nil
	}
	source, conversation := m.source, m.conversation
	options := m.submissionOptions()
	options.PreviousArtifacts = m.previousArtifactPaths()
	m.options.Attachments = nil
	m.options.Uploads = nil
	m.options.ExcludedKnowledgeIDs = nil
	m.options.RefreshSource = false
	return m.startWork(source, conversation, prompt, options)
}

func (m Model) updateHome(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.running {
		return m, nil
	}
	if key.String() == "/" && m.input == "" {
		m.openSlashPalette()
		return m, nil
	}
	switch key.String() {
	case "up":
		m.recallPreviousPrompt()
	case "down":
		m.recallNextPrompt()
	case "enter":
		return m.submitComposerInput()
	case "backspace":
		m.resetPromptHistoryNavigation()
		runes := []rune(m.input)
		if len(runes) > 0 {
			m.input = string(runes[:len(runes)-1])
		}
	case "esc":
		m.openCommandPalette()
	default:
		if key.Type == tea.KeyRunes || key.String() == " " {
			m.resetPromptHistoryNavigation()
			m.input += key.String()
		}
	}
	return m, nil
}

func (m Model) updateLauncher(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.launcherMode == "conversations" && m.catalog != nil && key.Type == tea.KeyEsc {
		m.cancelCatalog()
	}
	if m.launcherMode == "status-line" {
		return m.updateStatusLineEditor(key)
	}
	visible := m.filteredEntries()
	switch key.String() {
	case "up":
		if m.selected > 0 {
			m.selected--
		}
	case "down":
		if m.selected+1 < len(visible) {
			m.selected++
		}
	case "esc":
		m.launcher = false
		m.paletteQuery = ""
	case "backspace":
		runes := []rune(m.paletteQuery)
		if len(runes) > 0 {
			m.paletteQuery = string(runes[:len(runes)-1])
			m.selected = 0
		} else {
			m.launcher = false
		}
	case "ctrl+u":
		m.paletteQuery = ""
		m.selected = 0
	case " ":
		if m.launcherMode == "slash" && len(visible) > 0 {
			selected := visible[min(m.selected, len(visible)-1)]
			if strings.EqualFold(strings.TrimPrefix(selected.title, "/"), strings.TrimSpace(m.paletteQuery)) {
				m.launcher = false
				m.paletteQuery = ""
				m.input = selected.command + " "
				return m, nil
			}
		}
		m.paletteQuery += " "
		m.selected = 0
	case "enter":
		if len(visible) == 0 {
			return m, nil
		}
		selected := visible[min(m.selected, len(visible)-1)]
		switch selected.kind {
		case "command":
			m.launcher = false
			m.paletteQuery = ""
			if m.running {
				m.input = selected.command
				return m.submitRunningInput()
			}
			return m.runLocalCommand(selected.command)
		case "command-input":
			m.launcher = false
			m.paletteQuery = ""
			m.section = ""
			m.input = selected.command + " "
		case "copy":
			return m.copySelection(selected.id)
		case "conversation":
			if m.config.ManagedSessions {
				m.requestSession(BridgeSessionRequest{ConversationID: selected.id})
				return m, nil
			}
			if m.running {
				m.status = "Finish the current task before switching conversations"
				return m, nil
			}
			if m.config.ClearGoal != nil {
				m.config.ClearGoal()
			}
			m.launcher = false
			m.paletteQuery = ""
			m.section = ""
			m.home = false
			m.source = selected.source
			m.conversation = selected.id
			m.primary = selected.id == m.primaryID
			m.goalID = m.goalForConversation(selected.id)
			m.title = selected.title
			m.restoreConversation(selected.id, "")
			m.scroll = 0
		case "goal":
			if m.config.ManagedSessions {
				m.requestSession(BridgeSessionRequest{ConversationID: selected.id, Goal: true})
				return m, nil
			}
			m.primary = false
			if m.running {
				m.status = "Finish the current task before switching goals"
				return m, nil
			}
			if m.config.SelectGoal == nil {
				m.status = "Goal selection is unavailable"
				return m, nil
			}
			state, err := m.config.SelectGoal(selected.id)
			if err != nil {
				m.status = "Open goal: " + err.Error()
				return m, nil
			}
			m.launcher = false
			m.paletteQuery = ""
			m.section = ""
			m.home = false
			m.source = state.SourcePath
			m.goalID = selected.id
			m.conversation = state.ConversationID
			m.title = state.Title
			m.messages = nil
			m.revision = ""
			m.snapshot = ""
			m.lastBundle = BundleSummary{}
			m.lastOutput = ""
			m.options = RunOptions{MaxSteps: 24, Mode: "auto", Code: CodeOptions{MaxSteps: 16, Sandbox: "strict", Network: "deny"}}
			if state.ConversationID != "" {
				m.restoreConversation(state.ConversationID, "")
			}
			m.title = state.Title
			m.scroll = 0
		}
	default:
		if key.Type == tea.KeyRunes || key.String() == " " {
			m.paletteQuery += key.String()
			m.selected = 0
		}
	}
	return m, nil
}
