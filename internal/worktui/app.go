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

type RunResult struct {
	ConversationID string
	RevisionID     string
	SnapshotID     string
	FinalText      string
	OutputPath     string
	Error          string
}

type Config struct {
	CurrentFolder string
	Conversations []worksession.Conversation
	Jobs          []jobs.Definition
	Inbox         []inbox.Entry
	Run           func(source, conversationID, prompt string) RunResult
	MoveBack      func(conversationID string) (string, error)
	MoveForward   func(conversationID string) (string, error)
	History       func(conversationID string) (string, error)
	CodeCommand   func() *exec.Cmd
	FirstRun      bool
	SetupCommand  func(provider string) *exec.Cmd
}

type entry struct{ title, subtitle, kind, id, source string }
type message struct{ role, text string }
type runDone RunResult
type setupDone struct {
	provider string
	err      error
}

type Model struct {
	config       Config
	width        int
	height       int
	launcher     bool
	selected     int
	entries      []entry
	source       string
	conversation string
	title        string
	input        string
	messages     []message
	running      bool
	status       string
	onboarding   bool
}

func New(config Config) Model {
	model := Model{config: config, launcher: true}
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
	return model
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(messageValue tea.Msg) (tea.Model, tea.Cmd) {
	switch value := messageValue.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = value.Width, value.Height
	case runDone:
		m.running = false
		m.conversation = value.ConversationID
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
		m.status = "Revision " + value.RevisionID + " · snapshot " + value.SnapshotID
	case setupDone:
		if value.err != nil {
			m.messages = append(m.messages, message{role: "Gator", text: "Setup did not finish: " + value.err.Error() + "\nYou can try another provider name."})
			return m, nil
		}
		m.onboarding = false
		m.source = m.config.CurrentFolder
		m.title = "Work in " + filepath.Base(m.source)
		m.messages = append(m.messages, message{role: "Gator", text: "You’re connected to " + value.provider + ". What would you like to get done in this folder? I’ll freeze the selected files before I begin."})
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
		case "source", "conversation":
			m.launcher = false
			m.source = selected.source
			m.conversation = selected.id
			m.title = selected.title
			if len(m.messages) == 0 {
				greeting := "What would you like to get done? I’ll work from a private, frozen copy of this folder and keep every revision undoable."
				if selected.kind == "conversation" {
					greeting = "Welcome back. Continue here, or type /back, /forward, or /history to move through revisions without deleting them."
				}
				m.messages = append(m.messages, message{role: "Gator", text: greeting})
			}
		case "inbox":
			m.launcher = false
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
			m.onboarding = true
			m.source = selected.source
			m.title = "Welcome to Gator"
			m.messages = []message{{role: "Gator", text: "Hi — I’ll help you set up Gator. Which model provider do you want to use? Type a provider such as anthropic, openai, copilot, gemini, or openrouter. The provider’s secure sign-in flow will open next."}}
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
	switch command {
	case "/back":
		result, err = m.config.MoveBack(m.conversation)
	case "/forward":
		result, err = m.config.MoveForward(m.conversation)
	case "/history":
		result, err = m.config.History(m.conversation)
	default:
		err = fmt.Errorf("unknown command %s", command)
	}
	if err != nil {
		result = err.Error()
	}
	m.messages = append(m.messages, message{role: "Gator", text: result})
	return m, nil
}

func (m Model) View() string {
	width := m.width
	if width < 40 {
		width = 80
	}
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24")).Bold(true)
	var view strings.Builder
	view.WriteString(accent.Render("Gator Work") + dim.Render("  local-first work in your terminal") + "\n\n")
	if m.launcher {
		view.WriteString("Open\n")
		for index, item := range m.entries {
			line := fmt.Sprintf("  %-28s %s", item.title, dim.Render(item.subtitle))
			if index == m.selected {
				line = selectedStyle.Render("› " + fmt.Sprintf("%-28s %s", item.title, item.subtitle))
			}
			view.WriteString(line + "\n")
		}
		view.WriteString("\n" + dim.Render("↑/↓ choose · enter open · ctrl+p palette · ctrl+c quit"))
		return view.String()
	}
	view.WriteString(accent.Render(m.title) + "\n")
	if m.status != "" {
		view.WriteString(dim.Render(m.status) + "\n")
	}
	view.WriteString("\n")
	for _, item := range m.messages {
		text := item.text
		if len(text) > width*8 {
			text = text[:width*8] + "…"
		}
		view.WriteString(accent.Render(item.role+":") + " " + text + "\n\n")
	}
	if m.running {
		view.WriteString(accent.Render("● Working…") + "\n")
	} else {
		view.WriteString("❯ " + m.input + "█\n")
	}
	view.WriteString(dim.Render("enter send · ctrl+p projects/inbox/jobs · /back /forward /history · ctrl+c quit"))
	return view.String()
}
