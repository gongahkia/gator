// Package worktui implements Gator's conversation-first Work terminal application.
package worktui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
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

type Config struct {
	CurrentFolder       string
	Conversations       []worksession.Conversation
	StartConversationID string
	Jobs                []jobs.Definition
	Inbox               []inbox.Entry
	Run                 func(source, conversationID, prompt string) RunResult
	MoveBack            func(conversationID string) (string, error)
	MoveForward         func(conversationID string) (string, error)
	MoveToRevision      func(conversationID, revisionID string) (string, error)
	History             func(conversationID string) (string, error)
	CodeCommand         func() *exec.Cmd
	FirstRun            bool
	SetupCommand        func(provider string) *exec.Cmd
	CompleteSetup       func(provider string) error
}

type entry struct{ title, subtitle, kind, id, source string }
type message struct{ role, text string }
type runDone RunResult
type setupDone struct {
	provider string
	err      error
}

type Model struct {
	config        Config
	width         int
	height        int
	home          bool
	launcher      bool
	selected      int
	entries       []entry
	source        string
	conversation  string
	title         string
	input         string
	messages      []message
	running       bool
	status        string
	onboarding    bool
	firstRun      bool
	pendingPrompt string
	scroll        int
}

func New(config Config) Model {
	model := Model{
		config:   config,
		home:     true,
		firstRun: config.FirstRun,
		source:   config.CurrentFolder,
		title:    "Work in " + filepath.Base(config.CurrentFolder),
	}
	if config.FirstRun {
		model.entries = append(model.entries, entry{title: "Start guided setup", subtitle: "Connect a model, then begin your first task", kind: "onboarding", source: config.CurrentFolder})
	}
	model.entries = append(model.entries, entry{title: "Work in " + filepath.Base(config.CurrentFolder), subtitle: config.CurrentFolder, kind: "source", source: config.CurrentFolder})
	for _, conversation := range config.Conversations {
		model.entries = append(model.entries, entry{title: conversation.Title, subtitle: "Resume · " + conversation.SourcePath, kind: "conversation", id: conversation.ID, source: conversation.SourcePath})
	}
	model.entries = append(model.entries,
		entry{title: "Inbox", subtitle: fmt.Sprintf("%d recent results", len(config.Inbox)), kind: "inbox"},
		entry{title: "Scheduled jobs", subtitle: fmt.Sprintf("%d configured", len(config.Jobs)), kind: "jobs"},
		entry{title: "Code", subtitle: "Open the isolated coding workflow", kind: "code"},
	)
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

func (m Model) Update(messageValue tea.Msg) (tea.Model, tea.Cmd) {
	switch value := messageValue.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = value.Width, value.Height
	case runDone:
		m.running = false
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
		if value.RevisionID != "" || value.SnapshotID != "" {
			m.status = "Revision " + value.RevisionID + " · snapshot " + value.SnapshotID
		}
		m.scroll = 0
	case setupDone:
		if value.err != nil {
			m.messages = append(m.messages, message{role: "Gator", text: "Setup did not finish: " + value.err.Error() + "\nYou can try another provider name."})
			return m, nil
		}
		if m.config.CompleteSetup != nil {
			if err := m.config.CompleteSetup(value.provider); err != nil {
				m.messages = append(m.messages, message{role: "Gator", text: "Setup did not finish: " + err.Error() + "\nYou can try another provider name."})
				return m, nil
			}
		}
		m.onboarding = false
		m.firstRun = false
		entries := m.entries[:0]
		for _, item := range m.entries {
			if item.kind != "onboarding" {
				entries = append(entries, item)
			}
		}
		m.entries = entries
		m.selected = 0
		m.home = true
		m.source = m.config.CurrentFolder
		m.title = "Work in " + filepath.Base(m.source)
		m.messages = nil
		m.status = "Connected to " + value.provider
		if m.pendingPrompt != "" {
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
			return m, func() tea.Msg { return runDone(m.config.Run(source, conversation, prompt)) }
		}
	case tea.KeyMsg:
		if value.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if value.String() == "ctrl+p" {
			m.launcher = true
			m.selected = 0
			return m, nil
		}
		if m.launcher {
			return m.updateLauncher(value)
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
			return m, nil
		}
		switch value.String() {
		case "enter":
			prompt := strings.TrimSpace(m.input)
			if prompt == "" {
				return m, nil
			}
			m.input = ""
			if m.onboarding {
				if m.config.SetupCommand == nil {
					m.messages = append(m.messages, message{role: "Gator", text: "Setup is unavailable in this build."})
					return m, nil
				}
				provider := strings.ToLower(prompt)
				m.messages = append(m.messages, message{role: "You", text: provider})
				return m, tea.ExecProcess(m.config.SetupCommand(provider), func(err error) tea.Msg { return setupDone{provider: provider, err: err} })
			}
			if strings.HasPrefix(prompt, "/") {
				return m.runLocalCommand(prompt)
			}
			m.messages = append(m.messages, message{role: "You", text: prompt})
			m.home = false
			m.running = true
			m.status = "Working from an immutable snapshot…"
			run := m.config.Run
			source, conversation := m.source, m.conversation
			return m, func() tea.Msg { return runDone(run(source, conversation, prompt)) }
		case "backspace":
			runes := []rune(m.input)
			if len(runes) > 0 {
				m.input = string(runes[:len(runes)-1])
			}
		case "esc":
			m.launcher = true
			m.selected = 0
		default:
			if value.Type == tea.KeyRunes || value.String() == " " {
				m.input += value.String()
			}
		}
	}
	return m, nil
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
		if m.firstRun {
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
		return m, func() tea.Msg { return runDone(run(source, conversation, prompt)) }
	case "backspace":
		runes := []rune(m.input)
		if len(runes) > 0 {
			m.input = string(runes[:len(runes)-1])
		}
	case "esc":
		m.launcher = true
		m.selected = 0
	default:
		if key.Type == tea.KeyRunes || key.String() == " " {
			m.input += key.String()
		}
	}
	return m, nil
}

func (m Model) updateLauncher(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.selected+1 < len(m.entries) {
			m.selected++
		}
	case "esc":
		m.launcher = false
	case "enter":
		selected := m.entries[m.selected]
		switch selected.kind {
		case "source":
			m.launcher = false
			m.home = true
			m.onboarding = false
			m.source = selected.source
			m.conversation = ""
			m.title = selected.title
			m.messages = nil
			m.status = ""
			m.scroll = 0
		case "conversation":
			switching := m.source != selected.source || m.conversation != selected.id
			m.launcher = false
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
		case "inbox":
			m.launcher = false
			m.home = false
			m.source = ""
			m.conversation = ""
			m.title = "Inbox"
			m.messages = nil
			if len(m.config.Inbox) == 0 {
				m.messages = append(m.messages, message{role: "Gator", text: "Your inbox is empty."})
			}
			for _, item := range m.config.Inbox {
				m.messages = append(m.messages, message{role: item.Status, text: item.Title + "\n" + item.Summary})
			}
		case "jobs":
			m.launcher = false
			m.home = false
			m.source = ""
			m.conversation = ""
			m.title = "Scheduled jobs"
			m.messages = nil
			if len(m.config.Jobs) == 0 {
				m.messages = append(m.messages, message{role: "Gator", text: "No jobs yet. Create one with `gator job add`."})
			}
			for _, job := range m.config.Jobs {
				state := "disabled"
				if job.Enabled {
					state = "enabled"
				}
				m.messages = append(m.messages, message{role: state, text: job.Name + "\n" + job.Schedule + " · " + job.Timezone})
			}
		case "code":
			if m.config.CodeCommand == nil {
				return m, nil
			}
			return m, tea.ExecProcess(m.config.CodeCommand(), func(error) tea.Msg { return tea.Quit() })
		case "onboarding":
			m.launcher = false
			m.home = false
			m.onboarding = true
			m.source = selected.source
			m.title = "Welcome to Gator"
			m.messages = []message{{role: "Gator", text: "Hi — I’ll help you set up Gator. Which model provider do you want to use? Try openai, anthropic, or gemini. API-key entry is hidden."}}
		}
	}
	return m, nil
}

func (m Model) runLocalCommand(command string) (tea.Model, tea.Cmd) {
	if m.conversation == "" {
		m.messages = append(m.messages, message{role: "Gator", text: "Start or resume a conversation before using revision history."})
		return m, nil
	}
	var result string
	var err error
	fields := strings.Fields(command)
	switch command {
	case "/back":
		result, err = m.config.MoveBack(m.conversation)
	case "/forward":
		result, err = m.config.MoveForward(m.conversation)
	case "/history":
		result, err = m.config.History(m.conversation)
	default:
		if len(fields) == 2 && fields[0] == "/forward" && m.config.MoveToRevision != nil {
			result, err = m.config.MoveToRevision(m.conversation, fields[1])
		} else {
			err = fmt.Errorf("unknown command %s", command)
		}
	}
	if err != nil {
		result = err.Error()
	}
	m.messages = append(m.messages, message{role: "Gator", text: result})
	m.scroll = 0
	return m, nil
}

func (m Model) View() string {
	width := m.width
	if width <= 0 {
		width = 80
	} else if width < 20 {
		width = 20
	}
	height := m.height
	if height <= 0 {
		height = 24
	} else if height < 8 {
		height = 8
	}
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color("236"))
	if m.launcher {
		return m.renderPalette(width, height, accent, dim, selectedStyle)
	}
	if m.home && m.input == "" {
		return m.renderHome(width, height, accent, dim)
	}
	var view strings.Builder
	view.WriteString(accent.Render(gatorWordmark))
	if m.title != "" {
		view.WriteString(dim.Render("  " + m.title))
	}
	if m.status != "" {
		view.WriteString(dim.Render(" · " + m.status))
	}
	view.WriteString("\n\n")
	var transcript strings.Builder
	messageStyle := lipgloss.NewStyle().Width(max(20, width-4))
	for _, item := range m.messages {
		transcript.WriteString(messageStyle.Render(accent.Render(item.role+":") + " " + item.text))
		transcript.WriteString("\n\n")
	}
	lines := strings.Split(strings.TrimSuffix(transcript.String(), "\n"), "\n")
	available := max(4, height-10)
	maxScroll := max(0, len(lines)-available)
	scroll := min(m.scroll, maxScroll)
	end := len(lines) - scroll
	start := max(0, end-available)
	if len(lines) > 0 && lines[0] != "" {
		view.WriteString(strings.Join(lines[start:end], "\n") + "\n")
	}
	if m.running {
		view.WriteString(accent.Render("● Working…") + "\n\n")
	}
	view.WriteString(m.renderComposer(width, !m.running))
	view.WriteString("\n" + dim.Render("enter send  ·  ctrl+p menu  ·  pgup/pgdown scroll"))
	return view.String()
}

func (m Model) renderHome(width, height int, accent, dim lipgloss.Style) string {
	title := accent.Copy().Bold(true).Render(gatorWordmark)
	question := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Render("What do you want to accomplish?")
	body := title + "\n\n" + question + "\n\n" + m.renderComposer(width, true)
	context := filepath.Base(m.source)
	if context == "." || context == "" {
		context = "current folder"
	}
	hint := context + "  ·  ctrl+p menu"
	if m.firstRun {
		hint = "connect a model from ctrl+p  ·  " + hint
	} else if m.status != "" {
		hint = m.status + "  ·  " + hint
	}
	body += "\n" + dim.Render(hint)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, body)
}

func (m Model) renderPalette(width, height int, accent, dim, selectedStyle lipgloss.Style) string {
	panelWidth := min(76, max(16, width-4))
	titleWidth := min(28, max(8, panelWidth/2-1))
	subtitleWidth := max(0, panelWidth-titleWidth-3)
	var panel strings.Builder
	panel.WriteString(accent.Render("Open") + "\n\n")
	for index, item := range m.entries {
		title := truncate(item.title, titleWidth)
		subtitle := truncate(item.subtitle, subtitleWidth)
		line := fmt.Sprintf("  %-*s %s", titleWidth, title, dim.Render(subtitle))
		if index == m.selected {
			line = selectedStyle.Width(panelWidth).Render("› " + fmt.Sprintf("%-*s %s", titleWidth, title, subtitle))
		}
		panel.WriteString(line + "\n")
	}
	panel.WriteString("\n" + dim.Render("↑/↓ choose  ·  enter open  ·  esc close"))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel.String())
}

func (m Model) renderComposer(width int, focused bool) string {
	composerWidth := min(68, max(12, width-8))
	border := lipgloss.Color("238")
	if focused {
		border = lipgloss.Color("42")
	}
	value := m.input
	if value == "" {
		value = lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("Ask Gator to work on something…")
	}
	if focused {
		value += lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("█")
	}
	return lipgloss.NewStyle().Width(composerWidth).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(border).Render(value)
}

func truncate(value string, maximum int) string {
	if maximum <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	if maximum == 1 {
		return "…"
	}
	return string(runes[:maximum-1]) + "…"
}
