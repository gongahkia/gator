// Package worktui implements Gator's conversation-first Work terminal application.
package worktui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
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

// RunOptions contains the user-selected orchestration controls that accompany
// one prompt. They configure Gator and bound its internal Code specialist;
// they are never editable by the manager model itself.
type RunOptions struct {
	MaxSteps    int
	Attachments []string
	Code        CodeOptions
}

type CodeOptions struct {
	MaxSteps               int
	Verification           []string
	Scopes                 []string
	Profile                string
	Setup                  []string
	AllowedCommands        []string
	AllowedCommandPrefixes []string
	Sandbox                string
	Network                string
	Capabilities           []string
	BrowserSession         string
}

type Config struct {
	CurrentFolder       string
	Conversations       []worksession.Conversation
	StartConversationID string
	Jobs                []jobs.Definition
	Inbox               []inbox.Entry
	Run                 func(source, conversationID, prompt string, options RunOptions) RunResult
	MoveBack            func(conversationID string) (string, error)
	MoveForward         func(conversationID string) (string, error)
	MoveToRevision      func(conversationID, revisionID string) (string, error)
	History             func(conversationID string) (string, error)
	FirstRun            bool
	SetupCommand        func(provider string) *exec.Cmd
	CompleteSetup       func(provider string) error
	Inspect             func(topic string) (string, error)
	Copy                func(text string) error
	Theme               string
	SetTheme            func(name string) error
}

type entry struct{ title, subtitle, kind, id, source string }
type message struct{ role, text string }
type runDone RunResult
type setupDone struct {
	provider string
	err      error
}

type queuedRun struct {
	prompt  string
	options RunOptions
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
	options       RunOptions
	queue         []queuedRun
	lastOutput    string
	theme         string
}

func New(config Config) Model {
	model := Model{
		config:   config,
		home:     true,
		firstRun: config.FirstRun,
		source:   config.CurrentFolder,
		title:    "Work in " + filepath.Base(config.CurrentFolder),
		theme:    normalizeTheme(config.Theme),
		options: RunOptions{MaxSteps: 24, Code: CodeOptions{
			MaxSteps: 16, Sandbox: "strict", Network: "deny",
		}},
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
			source, conversation, run := m.source, m.conversation, m.config.Run
			return m, func() tea.Msg { return runDone(run(source, conversation, next.prompt, next.options)) }
		}
		if value.Error != "" && len(m.queue) > 0 {
			m.status = fmt.Sprintf("Run stopped · %d queued prompt(s) paused", len(m.queue))
		}
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
			options := cloneRunOptions(m.options)
			m.options.Attachments = nil
			return m, func() tea.Msg { return runDone(m.config.Run(source, conversation, prompt, options)) }
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
			switch value.String() {
			case "enter":
				prompt := strings.TrimSpace(m.input)
				if prompt == "" {
					return m, nil
				}
				m.input = ""
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
			if m.firstRun {
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
			run := m.config.Run
			source, conversation := m.source, m.conversation
			options := cloneRunOptions(m.options)
			m.options.Attachments = nil
			return m, func() tea.Msg { return runDone(run(source, conversation, prompt, options)) }
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
		if strings.HasPrefix(prompt, "/") {
			return m.runLocalCommand(prompt)
		}
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
		options := cloneRunOptions(m.options)
		m.options.Attachments = nil
		return m, func() tea.Msg { return runDone(run(source, conversation, prompt, options)) }
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
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return m, nil
	}
	m.home = false
	var result string
	var err error
	switch fields[0] {
	case "/help", "/?":
		result = workHelp()
	case "/new":
		m.home, m.onboarding, m.conversation = true, false, ""
		m.source = m.config.CurrentFolder
		m.title = "Work in " + filepath.Base(m.source)
		m.messages, m.status, m.queue = nil, "", nil
		return m, nil
	case "/model":
		m.onboarding = true
		m.pendingPrompt = ""
		m.title = "Choose a model provider"
		m.messages = []message{{role: "Gator", text: "Which model provider do you want to use? Try openai, anthropic, or gemini. API-key entry is hidden."}}
		return m, nil
	case "/effort":
		if len(fields) != 2 {
			result = "Usage: /effort low|standard|high\nCurrent: " + effortName(m.options.MaxSteps)
			break
		}
		switch strings.ToLower(fields[1]) {
		case "low":
			m.options.MaxSteps, m.options.Code.MaxSteps = 12, 8
		case "standard":
			m.options.MaxSteps, m.options.Code.MaxSteps = 24, 16
		case "high":
			m.options.MaxSteps, m.options.Code.MaxSteps = 48, 32
		default:
			err = fmt.Errorf("unknown effort %q; use low, standard, or high", fields[1])
		}
		if err == nil {
			result = "Effort set to " + effortName(m.options.MaxSteps) + "."
		}
	case "/attach":
		if len(fields) != 2 {
			result = "Usage: /attach SOURCE_RELATIVE_PATH\nPending: " + valueOrNone(strings.Join(m.options.Attachments, ", "))
			break
		}
		path := filepath.ToSlash(filepath.Clean(fields[1]))
		if filepath.IsAbs(fields[1]) || path == ".." || strings.HasPrefix(path, "../") {
			err = fmt.Errorf("attachment path must stay inside the selected source")
			break
		}
		if !sliceContains(m.options.Attachments, path) {
			m.options.Attachments = append(m.options.Attachments, path)
		}
		result = "Attached for the next prompt: " + path
	case "/detach":
		if len(fields) != 2 {
			err = fmt.Errorf("usage: /detach SOURCE_RELATIVE_PATH|all")
			break
		}
		if fields[1] == "all" {
			m.options.Attachments = nil
			result = "Cleared pending attachments."
			break
		}
		m.options.Attachments = removeString(m.options.Attachments, filepath.ToSlash(filepath.Clean(fields[1])))
		result = "Removed the pending attachment when present."
	case "/code":
		result, err = m.configureCode(fields, command)
	case "/status":
		result = m.workStatus()
	case "/permissions":
		result = m.codeStatus()
	case "/doctor", "/agents", "/settings":
		if m.config.Inspect == nil {
			err = errorsUnavailable(fields[0])
		} else {
			result, err = m.config.Inspect(strings.TrimPrefix(fields[0], "/"))
		}
	case "/theme":
		if len(fields) != 2 || m.config.SetTheme == nil {
			err = fmt.Errorf("usage: /theme gator|contrast|mono")
			break
		}
		name := normalizeTheme(fields[1])
		if name != strings.ToLower(fields[1]) {
			err = fmt.Errorf("usage: /theme gator|contrast|mono")
			break
		}
		err = m.config.SetTheme(name)
		if err == nil {
			m.theme, result = name, "Theme set to "+name+"."
		}
	case "/copy":
		if m.config.Copy == nil {
			err = errorsUnavailable("copy")
			break
		}
		text := m.latestGatorMessage()
		if text == "" {
			err = fmt.Errorf("there is no Gator response to copy")
		} else if err = m.config.Copy(text); err == nil {
			result = "Copied the latest Gator response."
		}
	case "/queue":
		result = m.queueStatus()
	case "/dequeue":
		if len(m.queue) == 0 {
			result = "The prompt queue is empty."
		} else {
			m.queue = m.queue[1:]
			result = m.queueStatus()
		}
	case "/clear-queue":
		m.queue = nil
		result = "Cleared the prompt queue."
	case "/review":
		if m.lastOutput == "" {
			result = "No completed Work output is available yet."
		} else {
			result = "Latest staged output: " + m.lastOutput + "\nUse `gator review " + filepath.Dir(m.lastOutput) + " --preview` for verified artifact and Code-patch evidence."
		}
	case "/back", "/forward", "/history":
		if m.conversation == "" {
			err = fmt.Errorf("start or resume a conversation before using revision history")
			break
		}
		switch fields[0] {
		case "/back":
			if m.config.MoveBack != nil {
				result, err = m.config.MoveBack(m.conversation)
			}
		case "/history":
			if m.config.History != nil {
				result, err = m.config.History(m.conversation)
			}
		case "/forward":
			if len(fields) == 2 && m.config.MoveToRevision != nil {
				result, err = m.config.MoveToRevision(m.conversation, fields[1])
			} else if m.config.MoveForward != nil {
				result, err = m.config.MoveForward(m.conversation)
			}
		}
	case "/quit":
		return m, tea.Quit
	default:
		err = fmt.Errorf("unknown command %s; use /help", fields[0])
	}
	if err != nil {
		result = err.Error()
	}
	m.messages = append(m.messages, message{role: "Gator", text: result})
	m.scroll = 0
	return m, nil
}

func (m *Model) configureCode(fields []string, raw string) (string, error) {
	if len(fields) == 1 || fields[1] == "status" {
		return m.codeStatus(), nil
	}
	action := strings.ToLower(fields[1])
	value := strings.TrimSpace(strings.TrimPrefix(raw, fields[0]+" "+fields[1]))
	requireValue := func() error {
		if value == "" {
			return fmt.Errorf("/code %s requires a value", action)
		}
		return nil
	}
	switch action {
	case "reset":
		m.options.Code = CodeOptions{MaxSteps: 16, Sandbox: "strict", Network: "deny"}
		return "Reset the internal Code specialist to strict, offline defaults.", nil
	case "verify":
		if err := requireValue(); err != nil {
			return "", err
		}
		m.options.Code.Verification = appendUnique(m.options.Code.Verification, value)
	case "scope":
		if err := requireValue(); err != nil {
			return "", err
		}
		m.options.Code.Scopes = appendUnique(m.options.Code.Scopes, value)
	case "profile":
		if err := requireValue(); err != nil {
			return "", err
		}
		m.options.Code.Profile = value
	case "setup":
		if err := requireValue(); err != nil {
			return "", err
		}
		m.options.Code.Setup = appendUnique(m.options.Code.Setup, value)
	case "allow":
		if err := requireValue(); err != nil {
			return "", err
		}
		m.options.Code.AllowedCommands = appendUnique(m.options.Code.AllowedCommands, value)
	case "allow-prefix":
		if err := requireValue(); err != nil {
			return "", err
		}
		m.options.Code.AllowedCommandPrefixes = appendUnique(m.options.Code.AllowedCommandPrefixes, value)
	case "sandbox":
		if value != "strict" && value != "off" {
			return "", fmt.Errorf("/code sandbox accepts strict or off")
		}
		m.options.Code.Sandbox = value
	case "network":
		if value != "deny" && value != "allow" {
			return "", fmt.Errorf("/code network accepts deny or allow")
		}
		m.options.Code.Network = value
	case "max-steps":
		steps, err := strconv.Atoi(value)
		if err != nil || steps < 1 || steps > 32 {
			return "", fmt.Errorf("/code max-steps requires an integer from 1 to 32")
		}
		m.options.Code.MaxSteps = steps
	case "grant", "revoke":
		capability := normalizeCodeCapability(value)
		if capability == "" {
			return "", fmt.Errorf("Code capability must be lsp, mcp, extension, http, browser, or terminal")
		}
		if action == "grant" {
			m.options.Code.Capabilities = appendUnique(m.options.Code.Capabilities, capability)
		} else {
			m.options.Code.Capabilities = removeString(m.options.Code.Capabilities, capability)
			if capability == "browser" {
				m.options.Code.BrowserSession = ""
			}
		}
	case "browser":
		if err := requireValue(); err != nil {
			return "", err
		}
		m.options.Code.BrowserSession = value
		m.options.Code.Capabilities = appendUnique(m.options.Code.Capabilities, "browser")
	default:
		return "", fmt.Errorf("unknown /code setting %q; use /code status", action)
	}
	return m.codeStatus(), nil
}

func (m Model) codeStatus() string {
	code := m.options.Code
	return fmt.Sprintf("Internal Code specialist\n  effort: %d steps\n  sandbox/network: %s/%s\n  profile: %s\n  scopes: %s\n  verification: %s\n  setup: %s\n  exact command grants: %s\n  prefix grants: %s\n  capabilities: %s\n  browser session: %s",
		code.MaxSteps, valueOrNone(code.Sandbox), valueOrNone(code.Network), valueOrNone(code.Profile), valueOrNone(strings.Join(code.Scopes, ", ")),
		valueOrNone(strings.Join(code.Verification, "; ")), valueOrNone(strings.Join(code.Setup, "; ")), valueOrNone(strings.Join(code.AllowedCommands, "; ")),
		valueOrNone(strings.Join(code.AllowedCommandPrefixes, "; ")), valueOrNone(strings.Join(code.Capabilities, ", ")), valueOrNone(code.BrowserSession))
}

func (m Model) workStatus() string {
	return fmt.Sprintf("Gator orchestration\n  source: %s\n  conversation: %s\n  effort: %s (%d manager steps)\n  pending attachments: %s\n  queued prompts: %d\n\n%s",
		valueOrNone(m.source), valueOrNone(m.conversation), effortName(m.options.MaxSteps), m.options.MaxSteps,
		valueOrNone(strings.Join(m.options.Attachments, ", ")), len(m.queue), m.codeStatus())
}

func (m Model) queueStatus() string {
	if len(m.queue) == 0 {
		return "The prompt queue is empty."
	}
	lines := make([]string, 0, len(m.queue)+1)
	lines = append(lines, fmt.Sprintf("%d queued prompt(s):", len(m.queue)))
	for index, item := range m.queue {
		lines = append(lines, fmt.Sprintf("  %d. %s", index+1, truncate(item.prompt, 80)))
	}
	return strings.Join(lines, "\n")
}

func (m Model) latestGatorMessage() string {
	for index := len(m.messages) - 1; index >= 0; index-- {
		if m.messages[index].role == "Gator" {
			return m.messages[index].text
		}
	}
	return ""
}

func cloneRunOptions(options RunOptions) RunOptions {
	result := options
	result.Attachments = append([]string(nil), options.Attachments...)
	result.Code.Verification = append([]string(nil), options.Code.Verification...)
	result.Code.Scopes = append([]string(nil), options.Code.Scopes...)
	result.Code.Setup = append([]string(nil), options.Code.Setup...)
	result.Code.AllowedCommands = append([]string(nil), options.Code.AllowedCommands...)
	result.Code.AllowedCommandPrefixes = append([]string(nil), options.Code.AllowedCommandPrefixes...)
	result.Code.Capabilities = append([]string(nil), options.Code.Capabilities...)
	return result
}

func appendUnique(values []string, value string) []string {
	if !sliceContains(values, value) {
		return append(values, value)
	}
	return values
}

func removeString(values []string, target string) []string {
	result := values[:0]
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}

func sliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func normalizeCodeCapability(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "lsp", "mcp", "http", "browser", "terminal":
		return strings.ToLower(strings.TrimSpace(value))
	case "extension", "extensions":
		return "extension"
	case "web", "research":
		return "http"
	default:
		return ""
	}
}

func effortName(steps int) string {
	if steps <= 12 {
		return "low"
	}
	if steps >= 48 {
		return "high"
	}
	return "standard"
}

func valueOrNone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return value
}

func errorsUnavailable(name string) error {
	return fmt.Errorf("%s is unavailable in this Gator build", strings.TrimPrefix(name, "/"))
}

func workHelp() string {
	return `Gator commands
  /model                         connect or switch the default model
  /effort low|standard|high      set manager and Code turn budgets
  /attach PATH                   send one source file with the next prompt
  /detach PATH|all               remove pending attachments
  /code status                   inspect the internal Code envelope
  /code verify COMMAND           add required project verification
  /code scope PATH               add a project-instruction scope
  /code profile NAME             select a project profile
  /code setup COMMAND            add an explicit setup command
  /code allow COMMAND            pre-approve one exact argv
  /code allow-prefix PREFIX      pre-approve a literal argv prefix
  /code grant CAPABILITY         grant lsp/mcp/extension/http/browser/terminal
  /code sandbox strict|off       set the child process boundary
  /code network deny|allow       set child network access
  /code browser SESSION          select an already controlled browser session
  /code reset                    restore strict, offline Code defaults
  /status · /permissions         inspect the active orchestration envelope
  /doctor · /agents · /settings inspect local configuration
  /history · /back · /forward   navigate retained Gator revisions
  /queue · /dequeue · /clear-queue
  /review · /copy · /theme · /new · /quit`
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
	accent, dim, selectedStyle := workStyles(m.theme)
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
	footer := "enter send  ·  ctrl+p menu  ·  /help  ·  pgup/pgdown scroll"
	if len(m.queue) > 0 {
		footer = fmt.Sprintf("%d queued  ·  ", len(m.queue)) + footer
	}
	view.WriteString("\n" + dim.Render(footer))
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
	cursor := lipgloss.Color("42")
	if m.theme == "contrast" {
		border, cursor = lipgloss.Color("250"), lipgloss.Color("46")
	} else if m.theme == "mono" {
		border, cursor = lipgloss.Color("245"), lipgloss.Color("255")
	}
	if focused {
		border = cursor
	}
	value := m.input
	if value == "" {
		value = lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("Ask Gator to work on something…")
	}
	if focused {
		value += lipgloss.NewStyle().Foreground(cursor).Render("█")
	}
	return lipgloss.NewStyle().Width(composerWidth).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(border).Render(value)
}

func normalizeTheme(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "contrast":
		return "contrast"
	case "mono":
		return "mono"
	default:
		return "gator"
	}
}

func workStyles(theme string) (lipgloss.Style, lipgloss.Style, lipgloss.Style) {
	switch normalizeTheme(theme) {
	case "contrast":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true),
			lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
			lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("226"))
	case "mono":
		return lipgloss.NewStyle().Bold(true), lipgloss.NewStyle().Faint(true), lipgloss.NewStyle().Reverse(true)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true),
			lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
			lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color("236"))
	}
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
