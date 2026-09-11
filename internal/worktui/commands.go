package worktui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) openCommandPalette() {
	m.launcher = true
	m.launcherMode = "commands"
	m.entries = commandPaletteEntries()
	m.paletteQuery = ""
	m.selected = 0
}

func (m *Model) openConversationPicker() {
	m.launcher = true
	m.launcherMode = "conversations"
	m.entries = make([]entry, 0, len(m.config.Conversations))
	for _, conversation := range m.config.Conversations {
		m.entries = append(m.entries, entry{
			title: conversation.Title, subtitle: conversation.SourcePath,
			kind: "conversation", id: conversation.ID, source: conversation.SourcePath,
		})
	}
	m.paletteQuery = ""
	m.selected = 0
}

func (m *Model) openProviderPicker(action string) bool {
	if m.config.ProviderChoices == nil {
		return false
	}
	providers := m.config.ProviderChoices(action)
	if len(providers) == 0 {
		return false
	}
	m.launcher = true
	m.launcherMode = action
	m.entries = make([]entry, 0, len(providers))
	for _, provider := range providers {
		m.entries = append(m.entries, entry{
			title: provider, subtitle: providerActionDescription(action),
			kind: "provider", id: provider, command: action,
		})
	}
	m.paletteQuery = ""
	m.selected = 0
	return true
}

func (m Model) filteredEntries() []entry {
	query := strings.ToLower(strings.TrimSpace(m.paletteQuery))
	if query == "" {
		return m.entries
	}
	result := make([]entry, 0, len(m.entries))
	for _, item := range m.entries {
		searchable := strings.ToLower(item.title + " " + item.subtitle)
		if strings.Contains(searchable, query) {
			result = append(result, item)
		}
	}
	return result
}

func commandPaletteEntries() []entry {
	return []entry{
		{title: "/help", subtitle: "Show Gator commands", kind: "command", command: "/help"},
		{title: "/new", subtitle: "Start a clean Gator conversation", kind: "command", command: "/new"},
		{title: "/model", subtitle: "Cloud and local models · download, select, rename, delete", kind: "command", command: "/model"},
		{title: "/connect", subtitle: "Start guided provider setup", kind: "command", command: "/connect"},
		{title: "/login", subtitle: "Store a native Gator credential", kind: "command", command: "/login"},
		{title: "/logout", subtitle: "Remove a stored Gator credential", kind: "command", command: "/logout"},
		{title: "/effort", subtitle: "Set low, standard, or high effort", kind: "command-input", command: "/effort"},
		{title: "/attach", subtitle: "Attach a source file to the next prompt", kind: "command-input", command: "/attach"},
		{title: "/detach", subtitle: "Remove a pending attachment", kind: "command-input", command: "/detach"},
		{title: "/status", subtitle: "Inspect Gator orchestration", kind: "command", command: "/status"},
		{title: "/permissions", subtitle: "Inspect the Code capability envelope", kind: "command", command: "/permissions"},
		{title: "/doctor", subtitle: "Inspect local prerequisites", kind: "command", command: "/doctor"},
		{title: "/agents", subtitle: "Inspect project profiles and roles", kind: "command", command: "/agents"},
		{title: "/settings", subtitle: "Inspect current settings", kind: "command", command: "/settings"},
		{title: "/theme", subtitle: "Choose gator, contrast, or mono", kind: "command-input", command: "/theme"},
		{title: "/history", subtitle: "Show revisions in this conversation", kind: "command", command: "/history"},
		{title: "/back", subtitle: "Move to the parent revision", kind: "command", command: "/back"},
		{title: "/forward", subtitle: "Move to a child or named revision", kind: "command-input", command: "/forward"},
		{title: "/review", subtitle: "Show latest staged output", kind: "command", command: "/review"},
		{title: "/copy", subtitle: "Copy the latest Gator response", kind: "command", command: "/copy"},
		{title: "/queue", subtitle: "Inspect queued prompts", kind: "command", command: "/queue"},
		{title: "/dequeue", subtitle: "Remove the next queued prompt", kind: "command", command: "/dequeue"},
		{title: "/clear-queue", subtitle: "Remove every queued prompt", kind: "command", command: "/clear-queue"},
		{title: "/code status", subtitle: "Inspect internal Code settings", kind: "command", command: "/code status"},
		{title: "/code verify", subtitle: "Add a project verifier", kind: "command-input", command: "/code verify"},
		{title: "/code scope", subtitle: "Add a project-instruction scope", kind: "command-input", command: "/code scope"},
		{title: "/code profile", subtitle: "Select a project profile", kind: "command-input", command: "/code profile"},
		{title: "/code setup", subtitle: "Add an explicit setup command", kind: "command-input", command: "/code setup"},
		{title: "/code allow", subtitle: "Pre-approve one exact command", kind: "command-input", command: "/code allow"},
		{title: "/code allow-prefix", subtitle: "Pre-approve a literal command prefix", kind: "command-input", command: "/code allow-prefix"},
		{title: "/code sandbox", subtitle: "Set strict or off", kind: "command-input", command: "/code sandbox"},
		{title: "/code network", subtitle: "Set deny or allow", kind: "command-input", command: "/code network"},
		{title: "/code max-steps", subtitle: "Set the child turn budget", kind: "command-input", command: "/code max-steps"},
		{title: "/code grant", subtitle: "Grant an integration capability", kind: "command-input", command: "/code grant"},
		{title: "/code revoke", subtitle: "Revoke an integration capability", kind: "command-input", command: "/code revoke"},
		{title: "/code browser", subtitle: "Select a controlled browser session", kind: "command-input", command: "/code browser"},
		{title: "/code reset", subtitle: "Restore strict, offline Code defaults", kind: "command", command: "/code reset"},
		{title: "/quit", subtitle: "Exit Gator", kind: "command", command: "/quit"},
	}
}

func (m Model) runLocalCommand(command string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return m, nil
	}
	m.home = false
	m.section = ""
	var result string
	var err error
	switch fields[0] {
	case "/help", "/?":
		result = workHelp()
	case "/new":
		m.home, m.onboarding, m.conversation = true, false, ""
		m.section, m.paletteQuery, m.launcherMode = "", "", ""
		m.source = m.config.CurrentFolder
		m.title = "Work in " + filepath.Base(m.source)
		m.messages, m.status, m.queue = nil, "", nil
		return m, nil
	case "/model", "/connect", "/login", "/logout":
		if fields[0] == "/model" && m.config.Models != nil {
			if len(fields) != 1 {
				result = "Use /model to select and manage cloud or local models."
				break
			}
			return m.openModels()
		}
		action := strings.TrimPrefix(fields[0], "/")
		if action == "model" {
			action = "setup"
		}
		if len(fields) > 2 {
			result = fmt.Sprintf("Usage: %s [PROVIDER]", fields[0])
			break
		}
		if len(fields) == 1 {
			if m.openProviderPicker(action) {
				return m, nil
			}
			result = fmt.Sprintf("No providers are available for %s in this build.", fields[0])
			break
		}
		provider := strings.ToLower(strings.TrimSpace(fields[1]))
		if action == "setup" {
			m.pendingPrompt = ""
		}
		return m.startProviderAction(action, provider)
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
		value := commandRemainder(command, fields[:1])
		if value == "" {
			result = "Usage: /attach SOURCE_RELATIVE_PATH\nPending: " + valueOrNone(strings.Join(m.options.Attachments, ", "))
			break
		}
		path := filepath.ToSlash(filepath.Clean(value))
		if filepath.IsAbs(value) || path == ".." || strings.HasPrefix(path, "../") {
			err = fmt.Errorf("attachment path must stay inside the selected source")
			break
		}
		if !sliceContains(m.options.Attachments, path) {
			m.options.Attachments = append(m.options.Attachments, path)
		}
		result = "Attached for the next prompt: " + path
	case "/detach":
		value := commandRemainder(command, fields[:1])
		if value == "" {
			err = fmt.Errorf("usage: /detach SOURCE_RELATIVE_PATH|all")
			break
		}
		if value == "all" {
			m.options.Attachments = nil
			result = "Cleared pending attachments."
			break
		}
		m.options.Attachments = removeString(m.options.Attachments, filepath.ToSlash(filepath.Clean(value)))
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
	value := commandRemainder(raw, fields[:2])
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

func commandRemainder(raw string, consumed []string) string {
	remainder := strings.TrimSpace(raw)
	for _, field := range consumed {
		if !strings.HasPrefix(remainder, field) {
			return ""
		}
		remainder = strings.TrimSpace(strings.TrimPrefix(remainder, field))
	}
	return remainder
}

func (m Model) startProviderAction(action, provider string) (tea.Model, tea.Cmd) {
	action = strings.ToLower(strings.TrimSpace(action))
	provider = strings.ToLower(strings.TrimSpace(provider))
	if action != "setup" && action != "connect" && action != "login" && action != "logout" {
		m.messages = append(m.messages, message{role: "Gator", text: "Unknown provider action: " + action})
		return m, nil
	}
	if provider == "" {
		m.messages = append(m.messages, message{role: "Gator", text: "Choose a provider first."})
		return m, nil
	}
	if m.config.ProviderCommand == nil {
		m.messages = append(m.messages, message{role: "Gator", text: providerActionFailure(action, errorsUnavailable(action))})
		return m, nil
	}
	command := m.config.ProviderCommand(action, provider)
	if command == nil {
		m.messages = append(m.messages, message{role: "Gator", text: providerActionFailure(action, errorsUnavailable(action))})
		return m, nil
	}
	if action == "setup" {
		m.onboarding = true
		m.title = "Choose a model provider"
	}
	return m, tea.ExecProcess(command, func(err error) tea.Msg {
		return providerActionDone{action: action, provider: provider, err: err}
	})
}

func providerActionDescription(action string) string {
	switch action {
	case "setup":
		return "Connect and use as Gator's default"
	case "connect":
		return "Start the closest supported setup flow"
	case "login":
		return "Store a native Gator credential"
	case "logout":
		return "Remove Gator's stored credential"
	default:
		return "Provider action"
	}
}

func providerActionFailure(action string, err error) string {
	label := action
	if action == "setup" {
		label = "model setup"
	}
	command := "/" + action
	if action == "setup" {
		command = "/model"
	}
	return strings.ToUpper(label[:1]) + label[1:] + " did not finish: " + err.Error() + "\nTry another provider with " + command + "."
}

func providerActionSuccess(action, provider string) string {
	switch action {
	case "connect":
		return "Connection finished for " + provider + ". Use /model " + provider + " to make it Gator's default."
	case "login":
		return "Login finished for " + provider + "."
	case "logout":
		return "Logout finished for " + provider + "."
	default:
		return "Provider action finished for " + provider + "."
	}
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
  /model                         manage cloud/local models, downloads and deletion
  /connect [PROVIDER]            start the closest supported setup flow
  /login [PROVIDER]              store a native Gator credential
  /logout [PROVIDER]             remove a stored Gator credential
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
  /code grant CAPABILITY         grant hooks/lsp/mcp/extension/http/browser/terminal
  /code sandbox strict|off       set the child process boundary
  /code network deny|allow       set child network access
  /code max-steps N              set the child turn budget
  /code revoke CAPABILITY        remove a child integration grant
  /code browser SESSION          select an already controlled browser session
  /code reset                    restore strict, offline Code defaults
  /status · /permissions         inspect the active orchestration envelope
  /doctor · /agents · /settings inspect local configuration
  /history · /back · /forward   navigate retained Gator revisions
  /steer TEXT                    steer the running task
  /tasks · /cancel-task ID        inspect or cancel active specialists
  /approve · /deny                respond to the displayed exact request
  /cancel                        cancel and retain the running outcome
  /queue · /dequeue · /clear-queue
  /review · /copy · /theme · /new · /quit

Navigation
  ctrl+x    retained conversations
	  ctrl+b    inbox
  ctrl+j    scheduled jobs
  ctrl+p    searchable command palette`
}
