package worktui

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
	"github.com/gongahkia/gator/internal/workrun"
)

func (m Model) Update(messageValue tea.Msg) (tea.Model, tea.Cmd) {
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
		return m.updateRunDone(value)
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
	m.models.Close()
	m.models = nil
	if m.config.SelectedModel != nil {
		provider, model, err := m.config.SelectedModel()
		if err != nil {
			m.status = "Read model selection: " + err.Error()
		} else {
			m.firstRun = provider == ""
			m.status = "Selected model: " + provider + " / " + model
			if m.firstRun {
				m.status = "Choose a model with /model before starting work."
			}
		}
	}
	m.refreshModelStatus()
	if m.pendingPrompt != "" {
		m.input, m.pendingPrompt = m.pendingPrompt, ""
	}
	return m, nil
}

func (m Model) updateLoading(value loadingTickMsg) (tea.Model, tea.Cmd) {
	if !m.running || value.run != m.loadingRun {
		return m, nil
	}
	m.loadingFrame = (m.loadingFrame + 1) % len(rattles.BrailleDots.Frames)
	return m, m.nextLoadingTick()
}

func (m Model) updateAgentEvent(value agent.Event) (tea.Model, tea.Cmd) {
	if value.Kind == agent.EventSubagent && value.TaskID != "" {
		var task struct{ ID, Role, Status, Error string }
		if json.Unmarshal([]byte(value.Text), &task) == nil {
			if m.tasks == nil {
				m.tasks = map[string]string{}
			}
			m.tasks[task.ID] = task.Role + ": " + task.Status
			m.status = task.ID + " " + m.tasks[task.ID]
			if task.Error != "" {
				m.messages = append(m.messages, message{role: "Specialist", text: task.ID + ": " + task.Error})
			}
			return m, waitWorkEvent(m.live)
		}
	}
	if value.Kind == agent.EventTextDelta {
		m.status = "Working: " + value.Text
	} else if value.Text != "" {
		m.status = value.Text
	} else {
		m.status = string(value.Kind)
	}
	return m, waitWorkEvent(m.live)
}

func (m Model) updateInteraction(value workrun.Interaction) (tea.Model, tea.Cmd) {
	if m.interaction != nil {
		m.pendingInteractions = append(m.pendingInteractions, value)
		return m, waitWorkEvent(m.live)
	}
	m.interaction = &value
	preview, _ := json.Marshal(value.Preview)
	m.messages = append(m.messages, message{role: "Approval", text: string(preview) + "\nUse /approve or /deny."})
	return m, waitWorkEvent(m.live)
}

func (m Model) updateRunDone(value runDone) (tea.Model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
	}
	m.operation = nil
	m.interaction = nil
	m.pendingInteractions = nil
	m.live = nil
	m.running = false
	m.runStarted = time.Time{}
	if m.quitting {
		return m, tea.Quit
	}
	if value.ConversationID != "" {
		m.conversation = value.ConversationID
	}
	if value.RevisionID != "" {
		m.revision = value.RevisionID
	}
	if value.SnapshotID != "" {
		m.snapshot = value.SnapshotID
	}
	if value.Error != "" {
		m.messages = append(m.messages, message{role: "Gator", text: explainRunFailure(value.Error, m.modelStatus)})
	} else {
		text := value.FinalText
		if text == "" {
			text = "Finished the Work revision."
		}
		m.messages = append(m.messages, message{role: "Gator", text: text})
		if value.Bundle.Path != "" {
			bundle := cloneBundleSummary(value.Bundle)
			m.messages = append(m.messages, message{role: "Deliverables", bundle: &bundle})
		}
	}
	m.lastOutput = value.OutputPath
	m.lastBundle = value.Bundle
	if m.revisionStatus() != "" {
		m.status = m.revisionStatus()
	}
	m.scroll = 0
	if value.Error == "" && len(m.queue) > 0 && m.config.Run != nil {
		next := m.queue[0]
		m.queue = m.queue[1:]
		m.messages = append(m.messages, message{role: "You", text: next.prompt})
		m.running = true
		m.status = fmt.Sprintf("Working from an immutable snapshot… · %d queued", len(m.queue))
		next.options.PreviousArtifacts = m.previousArtifactPaths()
		source, conversation := m.source, m.conversation
		return m.startWork(source, conversation, next.prompt, next.options)
	}
	if value.Error != "" && len(m.queue) > 0 {
		m.status = fmt.Sprintf("Run stopped · %d queued prompt(s) paused", len(m.queue))
	}
	return m, nil
}

func (m Model) updateBundleActionDone(value bundleActionDone) (tea.Model, tea.Cmd) {
	if value.err != nil {
		m.messages = append(m.messages, message{role: "Gator", text: value.err.Error()})
	} else {
		m.messages = append(m.messages, message{role: "Gator", text: value.text})
	}
	m.status = ""
	m.scroll = 0
	return m, nil
}

func (m Model) updateConnectorActionDone(value connectorActionDone) (tea.Model, tea.Cmd) {
	m.status = ""
	if value.err != nil {
		m.messages = append(m.messages, message{role: "Gator", text: "Connector " + value.action + " did not finish: " + value.err.Error()})
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
	m.messages = append(m.messages, message{role: "Gator", text: text})
	m.scroll = 0
	return m, nil
}

func (m Model) updateKey(value tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pendingBundleAction != nil {
		return m.updateBundleConfirmation(value)
	}
	if value.String() == "ctrl+c" {
		if m.running && m.cancel != nil {
			m.cancel()
			m.status = "Cancelling; retaining evidence…"
			return m, nil
		}
		return m, tea.Quit
	}
	if value.Type == tea.KeyCtrlX && !m.running {
		m.openConversationPicker()
		return m, nil
	}
	if value.Type == tea.KeyCtrlI && !m.running {
		m.openSourceMenu()
		return m, nil
	}
	if value.Type == tea.KeyCtrlB && !m.running {
		m.launcher = false
		m.section = "inbox"
		return m, nil
	}
	if value.Type == tea.KeyCtrlJ && !m.running {
		m.launcher = false
		m.section = "jobs"
		return m, nil
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
	if prompt == "exit" || prompt == "/quit" {
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
		m.messages = append(m.messages, message{role: "Specialists", text: strings.Join(lines, "\n")})
		return m, nil
	}
	if strings.HasPrefix(prompt, "/cancel-task ") && m.operation != nil {
		err := m.operation.CancelTask(strings.TrimSpace(strings.TrimPrefix(prompt, "/cancel-task ")))
		if err != nil {
			m.status = err.Error()
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
		}
		m.interaction = nil
		if len(m.pendingInteractions) > 0 {
			next := m.pendingInteractions[0]
			m.pendingInteractions = m.pendingInteractions[1:]
			m.interaction = &next
			preview, _ := json.Marshal(next.Preview)
			m.messages = append(m.messages, message{role: "Approval", text: string(preview) + "\nUse /approve or /deny."})
		}
		return m, nil
	}
	if prompt == "/cancel" && m.cancel != nil {
		m.cancel()
		m.status = "Cancelling…"
		return m, nil
	}
	if strings.HasPrefix(prompt, "/") {
		return m.runLocalCommand(prompt)
	}
	if len(m.queue) >= 16 {
		m.messages = append(m.messages, message{role: "Gator", text: "The local queue is full (16 prompts)."})
		return m, nil
	}
	m.queue = append(m.queue, queuedRun{prompt: prompt, options: cloneRunOptions(m.options)})
	m.recordPrompt(prompt)
	m.options.Attachments = nil
	m.options.RefreshSource = false
	m.status = fmt.Sprintf("Working from an immutable snapshot… · %d queued", len(m.queue))
	return m, nil
}

func (m Model) updateComposerKey(value tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	m.messages = append(m.messages, message{role: "You", text: prompt})
	m.recordPrompt(prompt)
	m.home = false
	m.running = true
	m.status = "Working from an immutable snapshot…"
	if m.config.Run == nil {
		m.running = false
		m.messages = append(m.messages, message{role: "Gator", text: "Work execution is unavailable in this build."})
		return m, nil
	}
	source, conversation := m.source, m.conversation
	options := cloneRunOptions(m.options)
	options.PreviousArtifacts = m.previousArtifactPaths()
	m.options.Attachments = nil
	m.options.RefreshSource = false
	return m.startWork(source, conversation, prompt, options)
}

func (m Model) updateHome(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.running {
		return m, nil
	}
	switch key.String() {
	case "up":
		m.recallPreviousPrompt()
	case "down":
		m.recallNextPrompt()
	case "enter":
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
		m.home = false
		m.messages = append(m.messages, message{role: "You", text: prompt})
		m.recordPrompt(prompt)
		m.running = true
		m.status = "Working from an immutable snapshot…"
		run := m.config.Run
		if run == nil {
			m.running = false
			m.messages = append(m.messages, message{role: "Gator", text: "Work execution is unavailable in this build."})
			return m, nil
		}
		source, conversation := m.source, m.conversation
		options := cloneRunOptions(m.options)
		options.PreviousArtifacts = m.previousArtifactPaths()
		m.options.Attachments = nil
		m.options.RefreshSource = false
		return m.startWork(source, conversation, prompt, options)
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
	case "enter":
		if len(visible) == 0 {
			return m, nil
		}
		selected := visible[min(m.selected, len(visible)-1)]
		switch selected.kind {
		case "command":
			m.launcher = false
			m.paletteQuery = ""
			return m.runLocalCommand(selected.command)
		case "command-input":
			m.launcher = false
			m.paletteQuery = ""
			m.section = ""
			m.input = selected.command + " "
		case "copy":
			return m.copySelection(selected.id)
		case "conversation":
			m.launcher = false
			m.paletteQuery = ""
			m.section = ""
			m.home = false
			m.source = selected.source
			m.conversation = selected.id
			m.title = selected.title
			m.restoreConversation(selected.id, "")
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
