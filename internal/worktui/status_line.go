package worktui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const statusLineSeparator = "  ·  "

type statusLineOption struct {
	id          string
	label       string
	description string
}

var statusLineOptions = []statusLineOption{
	{id: "queue", label: "Queued prompts", description: "Show only when prompts are queued"},
	{id: "send", label: "Send shortcut", description: "enter send"},
	{id: "commands", label: "Command palette", description: "ctrl+p commands"},
	{id: "conversations", label: "Conversations", description: "ctrl+x conversations"},
	{id: "inbox", label: "Inbox", description: "ctrl+b inbox"},
	{id: "jobs", label: "Scheduled jobs", description: "ctrl+j jobs"},
	{id: "model", label: "Model", description: "Selected provider and model"},
	{id: "model-access", label: "Model access", description: "Login, credential, or local-model readiness"},
	{id: "current-dir", label: "Current directory", description: "Selected source directory"},
	{id: "conversation", label: "Conversation", description: "Current conversation ID"},
	{id: "effort", label: "Effort", description: "Current manager effort"},
	{id: "sandbox", label: "Sandbox", description: "Code sandbox and network policy"},
	{id: "status", label: "Activity status", description: "Most recent transient status"},
}

var defaultStatusLine = []string{"queue", "send", "commands", "conversations", "inbox", "jobs"}

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
	case "commands":
		return "ctrl+p commands"
	case "conversations":
		return "ctrl+x conversations"
	case "inbox":
		return "ctrl+b inbox"
	case "jobs":
		return "ctrl+j jobs"
	case "model":
		provider, modelName := singleLine(m.modelStatus.Provider), singleLine(m.modelStatus.Model)
		if provider == "" {
			return "model none"
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
	case "conversation":
		return "conversation " + valueOrNone(singleLine(m.conversation))
	case "effort":
		return "effort " + effortName(m.options.MaxSteps)
	case "sandbox":
		return "code " + valueOrNone(m.options.Code.Sandbox) + "/" + valueOrNone(m.options.Code.Network)
	case "status":
		return singleLine(m.status)
	default:
		return ""
	}
}

// wrapStatusLine packs complete items onto each row when possible, then wraps
// an individual oversized item without relying on terminal-side clipping.
func wrapStatusLine(items []string, width int) string {
	if width <= 0 || len(items) == 0 {
		return ""
	}
	var rows []string
	current := ""
	flush := func() {
		if current != "" {
			rows = append(rows, current)
			current = ""
		}
	}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if current != "" && ansi.StringWidth(current+statusLineSeparator+item) <= width {
			current += statusLineSeparator + item
			continue
		}
		flush()
		chunks := splitDisplayWidth(item, width)
		for _, chunk := range chunks[:max(0, len(chunks)-1)] {
			rows = append(rows, chunk)
		}
		if len(chunks) > 0 {
			current = chunks[len(chunks)-1]
		}
	}
	flush()
	return strings.Join(rows, "\n")
}

func splitDisplayWidth(value string, width int) []string {
	if width <= 0 || value == "" {
		return nil
	}
	return strings.Split(ansi.Wrap(value, width, " "), "\n")
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
}

func (m Model) updateStatusLineEditor(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
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
			if err := m.config.SetStatusLine(configured); err != nil {
				m.status = "Save status line: " + err.Error()
				return m, nil
			}
		}
		m.statusLine = append([]string(nil), m.statusLineDraft...)
		m.statusLineConfigured = m.statusLineDraftSet
		m.launcher = false
		m.launcherMode = ""
		m.statusLineDraft = nil
		if m.statusLineConfigured && len(m.statusLine) == 0 {
			m.status = "Status line hidden."
		} else if !m.statusLineConfigured {
			m.status = "Status line restored to defaults."
		} else {
			m.status = "Status line updated."
		}
	}
	return m, nil
}

func (m Model) renderStatusLineEditor(width, height int, accent, dim, selectedStyle lipgloss.Style) string {
	panelWidth := min(76, max(16, width-4))
	maximum := max(1, height-13)
	selected := min(m.selected, len(statusLineOptions)-1)
	start := max(0, selected-maximum+1)
	end := min(len(statusLineOptions), start+maximum)
	var panel strings.Builder
	panel.WriteString(accent.Render(ansi.Wrap("Configure status line", panelWidth, " ")) + "\n")
	panel.WriteString(dim.Render(ansi.Wrap("Choose items shown below the composer; selected order is preserved.", panelWidth, " ")) + "\n\n")
	for index := start; index < end; index++ {
		option := statusLineOptions[index]
		order := selectedStatusLineIndex(m.statusLineDraft, option.id)
		marker := "[ ]"
		position := "  "
		if order >= 0 {
			marker = "[x]"
			position = fmt.Sprintf("%2d", order+1)
		}
		prefix := fmt.Sprintf("%s %s ", marker, position)
		labelWidth := min(19, max(8, panelWidth-len(prefix)-4))
		descriptionWidth := max(0, panelWidth-len(prefix)-labelWidth-1)
		line := prefix + fmt.Sprintf("%-*s %s", labelWidth, truncate(option.label, labelWidth), truncate(option.description, descriptionWidth))
		if index == selected {
			line = selectedStyle.Width(panelWidth).Render("› " + truncate(line, max(1, panelWidth-2)))
		} else {
			line = "  " + truncate(line, max(1, panelWidth-2))
		}
		panel.WriteString(line + "\n")
	}
	preview := m.statusLineDraftValues()
	if len(preview) == 0 {
		panel.WriteString("\n" + dim.Render("Preview: hidden"))
	} else {
		panel.WriteString("\n" + dim.Render("Preview\n"+wrapStatusLine(preview, panelWidth)))
	}
	controls := wrapStatusLine([]string{"space toggle", "←/→ reorder", "r defaults", "enter save", "esc cancel"}, panelWidth)
	panel.WriteString("\n\n" + dim.Render(controls))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel.String())
}

func (m Model) statusLineDraftValues() []string {
	m.statusLine = m.statusLineDraft
	return m.statusLineItems()
}
