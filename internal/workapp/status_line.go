package workapp

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type statusLineOption struct {
	id          string
	label       string
	description string
}

var statusLineOptions = []statusLineOption{
	{id: "queue", label: "Queued prompts", description: "Show only when prompts are queued"},
	{id: "send", label: "Send shortcut", description: "enter send"},
	{id: "editor", label: "Edit composer", description: "ctrl+g editor"},
	{id: "source", label: "Change workspace", description: "ctrl+i workspace"},
	{id: "commands", label: "Command palette", description: "ctrl+p commands"},
	{id: "conversations", label: "Conversations", description: "ctrl+x conversations"},
	{id: "inbox", label: "Inbox", description: "ctrl+b inbox"},
	{id: "jobs", label: "Scheduled jobs", description: "/jobs"},
	{id: "model", label: "Model", description: "Selected provider and model"},
	{id: "model-access", label: "Model access", description: "Login, credential, or local-model readiness"},
	{id: "current-dir", label: "Current directory", description: "Selected source directory"},
	{id: "mode", label: "Work mode", description: "Automatic, inspect, draft, or act"},
	{id: "artifacts", label: "Deliverables", description: "Expected output files"},
	{id: "connectors", label: "Connected sources", description: "Selected connector IDs"},
	{id: "conversation", label: "Conversation", description: "Current conversation ID"},
	{id: "effort", label: "Effort", description: "Current manager effort"},
	{id: "status", label: "Activity status", description: "Most recent transient status"},
}

var defaultStatusLine = []string{"model", "queue", "current-dir", "mode", "artifacts", "connectors", "send", "editor", "source", "commands", "conversations", "inbox", "jobs"}

func resolveStatusLine(configured *[]string) ([]string, bool) {
	if configured == nil {
		return append([]string(nil), defaultStatusLine...), false
	}
	return append([]string(nil), (*configured)...), true
}

func (m *Model) refreshModelStatus() {
	status := ModelStatus{}
	var err error
	if m.config.ModelStatus != nil {
		status, err = m.config.ModelStatus()
	} else if m.config.SelectedModel != nil {
		status.Provider, status.Model, err = m.config.SelectedModel()
	}
	if err != nil {
		status.Access = "status unavailable: " + singleLine(err.Error())
	}
	m.modelStatus = status
	if m.config.ComposerModels != nil {
		m.composerModels, err = m.config.ComposerModels()
		m.composerModelsIssue = ""
		if err != nil {
			m.composerModelsIssue = "Models could not load. Open Manage models to try again."
		}
	}
}

func (m Model) statusLineItems() []string {
	items := make([]string, 0, len(m.statusLine))
	for _, id := range m.statusLine {
		if value := m.statusLineValue(id); value != "" {
			items = append(items, value)
		}
	}
	return items
}

func (m Model) statusLineValue(id string) string {
	switch id {
	case "queue":
		if len(m.queue) == 0 {
			return ""
		}
		return fmt.Sprintf("%d queued", len(m.queue))
	case "send":
		return "enter send"
	case "editor":
		return "ctrl+g editor"
	case "source":
		return "ctrl+i workspace"
	case "commands":
		return "ctrl+p commands"
	case "conversations":
		return "ctrl+x conversations"
	case "inbox":
		return "ctrl+b inbox"
	case "jobs":
		return "/jobs scheduled jobs"
	case "model":
		provider, modelName := singleLine(m.modelStatus.Provider), singleLine(m.modelStatus.Model)
		if provider == "" {
			return ""
		}
		if modelName == "" {
			return "model " + provider
		}
		return "model " + provider + "/" + modelName
	case "model-access":
		access := singleLine(m.modelStatus.Access)
		if access == "" {
			access = "not configured"
		}
		return "access " + access
	case "current-dir":
		name := filepath.Base(m.source)
		if name == "." || name == "" {
			name = "current folder"
		}
		return "dir " + name
	case "mode":
		return "mode " + valueOrNone(m.options.Mode)
	case "artifacts":
		if len(m.options.Artifacts) == 0 {
			return ""
		}
		return "files " + strings.Join(m.options.Artifacts, ",")
	case "connectors":
		if len(m.options.ConnectorIDs) == 0 {
			return ""
		}
		return "sources " + strings.Join(m.options.ConnectorIDs, ",")
	case "conversation":
		return "conversation " + valueOrNone(singleLine(m.conversation))
	case "effort":
		return "effort " + effortName(m.options.MaxSteps)
	case "status":
		return singleLine(m.status)
	default:
		return ""
	}
}

func statusLineOptionIndex(id string) int {
	for index, option := range statusLineOptions {
		if option.id == id {
			return index
		}
	}
	return -1
}

func selectedStatusLineIndex(items []string, id string) int {
	for index, item := range items {
		if item == id {
			return index
		}
	}
	return -1
}

func (m *Model) openStatusLineEditor() {
	m.launcher = true
	m.launcherMode = "status-line"
	m.paletteQuery = ""
	m.selected = 0
	m.statusLineDraft = append([]string(nil), m.statusLine...)
	m.statusLineDraftSet = m.statusLineConfigured
	m.statusLineDraftEnabled = m.statusLineEnabled
}

func (m Model) updateStatusLineEditor(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "v":
		m.statusLineDraftEnabled = !m.statusLineDraftEnabled
	case "up":
		if m.selected > 0 {
			m.selected--
		}
	case "down":
		if m.selected+1 < len(statusLineOptions) {
			m.selected++
		}
	case " ":
		id := statusLineOptions[m.selected].id
		if index := selectedStatusLineIndex(m.statusLineDraft, id); index >= 0 {
			m.statusLineDraft = append(m.statusLineDraft[:index], m.statusLineDraft[index+1:]...)
		} else {
			m.statusLineDraft = append(m.statusLineDraft, id)
		}
		m.statusLineDraftSet = true
	case "left", "right":
		id := statusLineOptions[m.selected].id
		index := selectedStatusLineIndex(m.statusLineDraft, id)
		if index < 0 {
			break
		}
		target := index - 1
		if key.String() == "right" {
			target = index + 1
		}
		if target >= 0 && target < len(m.statusLineDraft) {
			m.statusLineDraft[index], m.statusLineDraft[target] = m.statusLineDraft[target], m.statusLineDraft[index]
			m.statusLineDraftSet = true
		}
	case "r":
		m.statusLineDraft = append([]string(nil), defaultStatusLine...)
		m.statusLineDraftSet = false
	case "esc":
		m.launcher = false
		m.launcherMode = ""
		m.statusLineDraft = nil
	case "enter":
		var configured *[]string
		if m.statusLineDraftSet {
			items := make([]string, len(m.statusLineDraft))
			copy(items, m.statusLineDraft)
			configured = &items
		}
		if m.config.SetStatusLine != nil {
			if err := m.config.SetStatusLine(m.statusLineDraftEnabled, configured); err != nil {
				m.status = "Save status line: " + err.Error()
				m.bridgeNotice = m.status
				return m, nil
			}
		}
		m.statusLine = append([]string(nil), m.statusLineDraft...)
		m.statusLineConfigured = m.statusLineDraftSet
		m.statusLineEnabled = m.statusLineDraftEnabled
		m.launcher = false
		m.launcherMode = ""
		m.statusLineDraft = nil
		if !m.statusLineEnabled || len(m.statusLine) == 0 {
			m.status = "Status line hidden."
		} else if !m.statusLineConfigured {
			m.status = "Default status line enabled."
		} else {
			m.status = "Status line updated."
		}
	}
	return m, nil
}

func (m Model) statusLineDraftValues() []string {
	m.statusLine = m.statusLineDraft
	return m.statusLineItems()
}
