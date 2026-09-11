package worktui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
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
	case providerActionDone:
		return m.updateProviderActionDone(value)
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
	m.onboarding = false
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
	if m.quitting {
		return m, tea.Quit
	}
	if value.ConversationID != "" {
		m.conversation = value.ConversationID
	}
	if value.Error != "" {
		m.messages = append(m.messages, message{role: "Gator", text: "I couldn't finish that run: " + value.Error})
	} else {
		text := value.FinalText
		if text == "" {
			text = "Finished the Work revision."
		}
		if value.OutputPath != "" {
			text += "\n\nArtifacts: " + value.OutputPath
		}
		m.messages = append(m.messages, message{role: "Gator", text: text})
	}
	m.lastOutput = value.OutputPath
	if value.RevisionID != "" || value.SnapshotID != "" {
		m.status = "Revision " + value.RevisionID + " · snapshot " + value.SnapshotID
	}
	m.scroll = 0
	if value.Error == "" && len(m.queue) > 0 && m.config.Run != nil {
		next := m.queue[0]
		m.queue = m.queue[1:]
		m.messages = append(m.messages, message{role: "You", text: next.prompt})
		m.running = true
		m.status = fmt.Sprintf("Working from an immutable snapshot… · %d queued", len(m.queue))
		source, conversation := m.source, m.conversation
		return m.startWork(source, conversation, next.prompt, next.options)
	}
	if value.Error != "" && len(m.queue) > 0 {
		m.status = fmt.Sprintf("Run stopped · %d queued prompt(s) paused", len(m.queue))
	}
	return m, nil
}

func (m Model) updateProviderActionDone(value providerActionDone) (tea.Model, tea.Cmd) {
	if value.err != nil {
		m.messages = append(m.messages, message{role: "Gator", text: providerActionFailure(value.action, value.err)})
		return m, nil
	}
	selectedDefault := false
	if value.action != "setup" {
		if m.firstRun && (value.action == "login" || value.action == "connect") && m.config.CompleteSetup != nil {
			if err := m.config.CompleteSetup(value.provider); err != nil {
				m.status = providerActionSuccess(value.action, value.provider)
				m.messages = append(m.messages, message{role: "Gator", text: m.status + "\nGator could not select it as the default: " + err.Error() + "\nChoose a default with /model."})
				m.scroll = 0
				return m, nil
			}
			selectedDefault = true
		} else {
			m.status = providerActionSuccess(value.action, value.provider)
			m.messages = append(m.messages, message{role: "Gator", text: m.status})
			m.scroll = 0
			return m, nil
		}
	}
	if !selectedDefault && m.config.CompleteSetup != nil {
		if err := m.config.CompleteSetup(value.provider); err != nil {
			m.messages = append(m.messages, message{role: "Gator", text: providerActionFailure(value.action, err)})
			return m, nil
		}
	}
	m.onboarding = false
	m.firstRun = false
	m.selected = 0
	m.home = true
	m.source = m.config.CurrentFolder
	m.title = "Work in " + filepath.Base(m.source)
	m.messages = nil
	m.status = "Connected to " + value.provider
	if m.pendingPrompt == "" {
		return m, nil
	}
	prompt := m.pendingPrompt
	m.pendingPrompt = ""
	m.home = false
	m.messages = append(m.messages, message{role: "You", text: prompt})
	m.running = true
	m.status = "Working from an immutable snapshot…"
	if m.config.Run == nil {
		m.running = false
		m.messages = append(m.messages, message{role: "Gator", text: "Work execution is unavailable in this build."})
		return m, nil
	}
	source, conversation := m.source, m.conversation
	options := cloneRunOptions(m.options)
	m.options.Attachments = nil
	return m.startWork(source, conversation, prompt, options)
}

func (m Model) updateKey(value tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		if value.String() == "esc" {
			m.section = ""
		}
		return m, nil
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

func (m Model) updateRunningKey(value tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch value.String() {
	case "enter":
		return m.submitRunningInput()
	case "backspace":
		runes := []rune(m.input)
		if len(runes) > 0 {
			m.input = string(runes[:len(runes)-1])
		}
	default:
		if value.Type == tea.KeyRunes || value.String() == " " {
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
	if prompt == "/quit" && m.cancel != nil {
		m.quitting = true
		m.cancel()
		m.status = "Cancelling before exit…"
		return m, nil
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
	m.options.Attachments = nil
	m.status = fmt.Sprintf("Working from an immutable snapshot… · %d queued", len(m.queue))
	return m, nil
}

func (m Model) updateComposerKey(value tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch value.String() {
	case "enter":
		return m.submitComposerInput()
	case "backspace":
		runes := []rune(m.input)
		if len(runes) > 0 {
			m.input = string(runes[:len(runes)-1])
		}
	case "esc":
		m.openCommandPalette()
	default:
		if value.Type == tea.KeyRunes || value.String() == " " {
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
	if strings.HasPrefix(prompt, "/") {
		return m.runLocalCommand(prompt)
	}
	if m.onboarding {
		provider := strings.ToLower(strings.TrimSpace(prompt))
		m.messages = append(m.messages, message{role: "You", text: provider})
		return m.startProviderAction("setup", provider)
	}
	if m.firstRun {
		if m.config.Models != nil {
			m.pendingPrompt = prompt
			return m.openModels()
		}
		m.onboarding = true
		m.pendingPrompt = prompt
		m.title = "Welcome to Gator"
		m.messages = append(m.messages, message{role: "Gator", text: "Before I start, which model provider do you want to use? Try openai, anthropic, or gemini. API-key entry is hidden."})
		return m, nil
	}
	m.messages = append(m.messages, message{role: "You", text: prompt})
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
	m.options.Attachments = nil
	return m.startWork(source, conversation, prompt, options)
}

func (m Model) updateHome(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.running {
		return m, nil
	}
	switch key.String() {
	case "enter":
		prompt := strings.TrimSpace(m.input)
		if prompt == "" {
			return m, nil
		}
		m.input = ""
		if strings.HasPrefix(prompt, "/") {
			return m.runLocalCommand(prompt)
		}
		if m.firstRun {
			if m.config.Models != nil {
				m.pendingPrompt = prompt
				return m.openModels()
			}
			m.home = false
			m.onboarding = true
			m.pendingPrompt = prompt
			m.title = "Welcome to Gator"
			m.messages = []message{{role: "Gator", text: "Before I start, which model provider do you want to use? Try openai, anthropic, or gemini. API-key entry is hidden."}}
			return m, nil
		}
		m.home = false
		m.messages = append(m.messages, message{role: "You", text: prompt})
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
		m.options.Attachments = nil
		return m.startWork(source, conversation, prompt, options)
	case "backspace":
		runes := []rune(m.input)
		if len(runes) > 0 {
			m.input = string(runes[:len(runes)-1])
		}
	case "esc":
		m.openCommandPalette()
	default:
		if key.Type == tea.KeyRunes || key.String() == " " {
			m.input += key.String()
		}
	}
	return m, nil
}

func (m Model) updateLauncher(key tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		case "provider":
			m.launcher = false
			m.paletteQuery = ""
			return m.startProviderAction(selected.command, selected.id)
		case "conversation":
			switching := m.source != selected.source || m.conversation != selected.id
			m.launcher = false
			m.paletteQuery = ""
			m.section = ""
			m.home = false
			m.onboarding = false
			m.source = selected.source
			m.conversation = selected.id
			m.title = selected.title
			m.scroll = 0
			if switching {
				m.messages = nil
				m.status = ""
			}
		}
	default:
		if key.Type == tea.KeyRunes || key.String() == " " {
			m.paletteQuery += key.String()
			m.selected = 0
		}
	}
	return m, nil
}
