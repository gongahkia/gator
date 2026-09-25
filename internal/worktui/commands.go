package worktui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/instructions"
)

func (m *Model) openCommandPalette() {
	m.launcher = true
	m.launcherMode = "commands"
	m.entries = commandPaletteEntries()
	m.paletteQuery = ""
	m.selected = 0
}

func (m *Model) openConversationPicker() {
	if m.config.ListConversations != nil {
		conversations, err := m.config.ListConversations()
		if err != nil {
			m.status = "List conversations: " + err.Error()
		} else {
			m.config.Conversations = conversations
		}
	}
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

func (m *Model) openSourceMenu() {
	m.launcher = true
	m.launcherMode = "source"
	m.entries = []entry{
		{title: "Change workspace", subtitle: "Current: " + valueOrNone(m.source), kind: "command-input", command: "/source"},
		{title: "Refresh workspace", subtitle: "Capture a new immutable snapshot on the next turn", kind: "command", command: "/source-refresh"},
		{title: "Ignore project instruction", subtitle: "Skip one AGENTS or project-rules file for this conversation", kind: "command-input", command: "/source ignore"},
		{title: "Restore project instruction", subtitle: "Read a previously ignored project instruction again", kind: "command-input", command: "/source unignore"},
		{title: "List ignored project instructions", subtitle: valueOrNone(strings.Join(m.options.IgnoredInstructionPaths, ", ")), kind: "command", command: "/source ignored"},
	}
	m.paletteQuery = ""
	m.selected = 0
}

func (m *Model) openCopyPicker() bool {
	entries := make([]entry, 0, len(m.messages)+1)
	for index := len(m.messages) - 1; index >= 0; index-- {
		item := m.messages[index]
		if item.role != "Gator" || strings.TrimSpace(item.text) == "" {
			continue
		}
		entries = append(entries, entry{
			title:    "Gator response",
			subtitle: truncate(singleLine(item.text), 80),
			kind:     "copy",
			id:       fmt.Sprintf("message:%d", index),
		})
	}
	if m.lastBundle.Path != "" {
		entries = append(entries, entry{
			title:    "Verified deliverables",
			subtitle: truncate(singleLine(formatBundleSummary(m.lastBundle)), 80),
			kind:     "copy",
			id:       "bundle",
		})
	}
	if len(entries) == 0 {
		return false
	}
	m.launcher = true
	m.launcherMode = "copy"
	m.entries = entries
	m.paletteQuery = ""
	m.selected = 0
	return true
}

func (m Model) copySelection(id string) (tea.Model, tea.Cmd) {
	m.launcher = false
	m.paletteQuery = ""
	var text, label string
	switch {
	case id == "bundle":
		text, label = formatBundleSummary(m.lastBundle), "verified deliverables"
	case strings.HasPrefix(id, "message:"):
		var index int
		if _, err := fmt.Sscanf(id, "message:%d", &index); err == nil && index >= 0 && index < len(m.messages) {
			text, label = m.messages[index].text, "Gator response"
		}
	}
	if strings.TrimSpace(text) == "" {
		m.messages = append(m.messages, message{role: "Gator", text: "That item is no longer available to copy."})
		return m, nil
	}
	if err := m.config.Copy(text); err != nil {
		m.messages = append(m.messages, message{role: "Gator", text: err.Error()})
		return m, nil
	}
	m.messages = append(m.messages, message{role: "Gator", text: "Copied " + label + "."})
	m.scroll = 0
	return m, nil
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
		{title: "/model", subtitle: "Cloud and local models · configure, sign in, download, select", kind: "command", command: "/model"},
		{title: "/effort", subtitle: "Set low, standard, or high effort", kind: "command-input", command: "/effort"},
		{title: "/attach", subtitle: "Attach a source file to the next prompt", kind: "command-input", command: "/attach"},
		{title: "/detach", subtitle: "Remove a pending attachment", kind: "command-input", command: "/detach"},
		{title: "/source", subtitle: "Inspect, change, or refresh the conversation workspace", kind: "command", command: "/source"},
		{title: "/source-refresh", subtitle: "Capture the selected workspace again", kind: "command", command: "/source-refresh"},
		{title: "/source ignore", subtitle: "Skip a project instruction file for this conversation", kind: "command-input", command: "/source ignore"},
		{title: "/source unignore", subtitle: "Restore a skipped project instruction file", kind: "command-input", command: "/source unignore"},
		{title: "/mode", subtitle: "Set auto, inspect, draft, or act", kind: "command-input", command: "/mode"},
		{title: "/code", subtitle: "Require or release the Code specialist for Work", kind: "command-input", command: "/code"},
		{title: "/artifact", subtitle: "Add, remove, or list deliverables", kind: "command-input", command: "/artifact"},
		{title: "/connector", subtitle: "Select connected sources for this session", kind: "command-input", command: "/connector"},
		{title: "/web-origin", subtitle: "Select an HTTPS research origin", kind: "command-input", command: "/web-origin"},
		{title: "/status", subtitle: "Inspect Gator orchestration", kind: "command", command: "/status"},
		{title: "/statusline", subtitle: "Choose and order composer footer items", kind: "command", command: "/statusline"},
		{title: "/permissions", subtitle: "Inspect Gator's active Work authority", kind: "command", command: "/permissions"},
		{title: "/doctor", subtitle: "Inspect local prerequisites", kind: "command", command: "/doctor"},
		{title: "/agents", subtitle: "Inspect project profiles and roles", kind: "command", command: "/agents"},
		{title: "/settings", subtitle: "Inspect current settings", kind: "command", command: "/settings"},
		{title: "/theme", subtitle: "Choose gator, contrast, or mono", kind: "command-input", command: "/theme"},
		{title: "/history", subtitle: "Show Work executions in this conversation", kind: "command", command: "/history"},
		{title: "/learnings", subtitle: "Inspect and control scoped guidance for future Work", kind: "command", command: "/learnings"},
		{title: "/feedback", subtitle: "Accept, reject, correct, or remember this Work result", kind: "command-input", command: "/feedback"},
		{title: "/revision-back", subtitle: "Move to the parent revision", kind: "command", command: "/revision-back"},
		{title: "/revision-forward", subtitle: "Move to a child or named revision", kind: "command-input", command: "/revision-forward"},
		{title: "/review", subtitle: "Show latest staged output", kind: "command", command: "/review"},
		{title: "/save", subtitle: "Save verified deliverables to a folder", kind: "command-input", command: "/save"},
		{title: "/apply", subtitle: "Apply a verified Code candidate", kind: "command-input", command: "/apply"},
		{title: "/retry", subtitle: "Retry failed or pending local changes", kind: "command-input", command: "/retry"},
		{title: "/copy", subtitle: "Choose a response or deliverable summary to copy", kind: "command", command: "/copy"},
		{title: "/queue", subtitle: "Inspect queued prompts", kind: "command", command: "/queue"},
		{title: "/dequeue", subtitle: "Remove the next queued prompt", kind: "command", command: "/dequeue"},
		{title: "/clear-queue", subtitle: "Remove every queued prompt", kind: "command", command: "/clear-queue"},
		{title: "exit", subtitle: "Exit Gator", kind: "command", command: "exit"},
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
		m.home, m.conversation = true, ""
		m.section, m.paletteQuery, m.launcherMode = "", "", ""
		m.source = m.config.CurrentFolder
		m.title = "Work in " + filepath.Base(m.source)
		m.revision, m.snapshot = "", ""
		m.clearPromptHistory()
		m.messages, m.status, m.queue = nil, "", nil
		m.lastOutput, m.lastBundle = "", BundleSummary{}
		m.options = RunOptions{MaxSteps: 24, Mode: "auto", Code: CodeOptions{MaxSteps: 16, Sandbox: "strict", Network: "deny"}}
		return m, nil
	case "/model":
		if len(fields) != 1 {
			result = "Use /model to configure, sign in to, and select cloud or local models."
			break
		}
		if m.config.Models == nil {
			err = errorsUnavailable("model management")
			break
		}
		return m.openModels()
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
	case "/source":
		value := commandRemainder(command, fields[:1])
		if value == "" {
			m.openSourceMenu()
			return m, nil
		}
		if len(fields) >= 2 {
			switch strings.ToLower(fields[1]) {
			case "ignore", "unignore":
				path, normalizeErr := instructions.NormalizeIgnoredPaths([]string{commandRemainder(command, fields[:2])})
				if normalizeErr != nil {
					err = normalizeErr
					break
				}
				if strings.EqualFold(fields[1], "ignore") {
					m.options.IgnoredInstructionPaths = appendUnique(m.options.IgnoredInstructionPaths, path[0])
					result = "Gator will skip project instruction \"" + path[0] + "\" for this conversation. Resend your request."
				} else {
					m.options.IgnoredInstructionPaths = removeString(m.options.IgnoredInstructionPaths, path[0])
					result = "Gator will read project instruction \"" + path[0] + "\" again when applicable."
				}
			case "ignored":
				if len(fields) != 2 {
					err = fmt.Errorf("usage: /source ignored")
				} else if len(m.options.IgnoredInstructionPaths) == 0 {
					result = "No project instruction files are ignored for this conversation."
				} else {
					result = "Ignored project instruction files:\n  " + strings.Join(m.options.IgnoredInstructionPaths, "\n  ")
				}
			}
			if result != "" || err != nil {
				break
			}
		}
		if m.config.ResolveSource == nil {
			err = errorsUnavailable("source selection")
			break
		}
		var resolved string
		resolved, err = m.config.ResolveSource(value)
		if err == nil {
			m.source = resolved
			m.title = "Work in " + filepath.Base(resolved)
			m.options.RefreshSource = m.conversation != ""
			result = "Workspace set to " + resolved + ". It remains read-only during Work."
			if m.options.RefreshSource {
				result += "\nThe next turn will capture a new immutable snapshot."
			}
		}
	case "/source-refresh":
		m.options.RefreshSource = true
		result = "The next turn will capture a new immutable snapshot of " + m.source + "."
	case "/mode":
		if len(fields) != 2 || !sliceContains([]string{"auto", "inspect", "draft", "act"}, strings.ToLower(fields[1])) {
			err = fmt.Errorf("usage: /mode auto|inspect|draft|act")
			break
		}
		m.options.Mode = strings.ToLower(fields[1])
		result = "Work mode set to " + m.options.Mode + "."
	case "/code":
		if len(fields) != 2 || !sliceContains([]string{"on", "off"}, strings.ToLower(fields[1])) {
			err = fmt.Errorf("usage: /code on|off")
			break
		}
		m.options.RequireCode = strings.EqualFold(fields[1], "on")
		if m.options.RequireCode {
			result = "Code specialist required for subsequent Work. Gator will retain patch evidence before completion."
		} else {
			result = "Code specialist is optional for subsequent Work."
		}
	case "/artifact":
		result, err = m.configureArtifacts(fields, command)
	case "/connector":
		result, err = m.configureConnectors(fields)
		if err == nil && m.pendingConnectorCmd != nil {
			command := m.pendingConnectorCmd
			m.pendingConnectorCmd = nil
			return m, command
		}
	case "/web-origin":
		result, err = m.configureWebOrigins(fields)
	case "/status":
		result = m.workStatus()
	case "/statusline", "/status-line":
		if len(fields) != 1 {
			err = fmt.Errorf("usage: /statusline")
			break
		}
		m.openStatusLineEditor()
		return m, nil
	case "/permissions":
		result = m.workPermissions()
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
		if !m.openCopyPicker() {
			err = fmt.Errorf("there is no Gator response or verified deliverable summary to copy")
			break
		}
		return m, nil
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
	case "/learnings", "/learning":
		if m.config.LearningAction == nil {
			err = errorsUnavailable("learnings")
			break
		}
		result, err = m.config.LearningAction(fields[1:])
	case "/feedback":
		if m.revision == "" {
			err = errors.New("finish or resume a Work revision before leaving feedback")
			break
		}
		if m.config.FeedbackAction == nil {
			err = errorsUnavailable("feedback")
			break
		}
		result, err = m.config.FeedbackAction(m.revision, fields[1:])
	case "/review":
		if m.lastBundle.Path == "" {
			result = "No completed Work output is available yet."
		} else if m.config.BundleAction == nil {
			result = formatBundleSummary(m.lastBundle)
		} else {
			result, err = m.config.BundleAction(BundleActionRequest{Action: "preview", BundlePath: m.lastBundle.Path})
		}
	case "/save":
		return m.prepareSave(command)
	case "/apply":
		return m.prepareCodeApply(command)
	case "/retry":
		return m.prepareRetry(command)
	case "/revision-back", "/revision-forward", "/history":
		if m.conversation == "" {
			err = fmt.Errorf("start or resume a conversation before using revision history")
			break
		}
		switch fields[0] {
		case "/revision-back":
			if m.config.MoveBack != nil {
				result, err = m.config.MoveBack(m.conversation)
			}
		case "/history":
			if m.config.History != nil {
				result, err = m.config.History(m.conversation)
			}
		case "/revision-forward":
			if len(fields) == 2 && m.config.MoveToRevision != nil {
				result, err = m.config.MoveToRevision(m.conversation, fields[1])
			} else if m.config.MoveForward != nil {
				result, err = m.config.MoveForward(m.conversation)
			}
		}
		if err == nil && fields[0] != "/history" && m.config.LoadConversation != nil {
			m.restoreConversation(m.conversation, result)
			m.scroll = 0
			return m, nil
		}
	case "exit", "/quit":
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

func (m *Model) configureArtifacts(fields []string, raw string) (string, error) {
	if len(fields) == 1 || fields[1] == "list" {
		return "Deliverables: " + valueOrNone(strings.Join(m.options.Artifacts, ", ")), nil
	}
	action := strings.ToLower(fields[1])
	value := commandRemainder(raw, fields[:2])
	switch action {
	case "add":
		if err := artifactPathOK(value); err != nil {
			return "", err
		}
		m.options.Artifacts = appendUnique(m.options.Artifacts, value)
	case "remove":
		if value == "" {
			return "", fmt.Errorf("usage: /artifact remove PATH")
		}
		m.options.Artifacts = removeString(m.options.Artifacts, value)
	case "clear":
		m.options.Artifacts = nil
	default:
		value = commandRemainder(raw, fields[:1])
		if err := artifactPathOK(value); err != nil {
			return "", err
		}
		m.options.Artifacts = appendUnique(m.options.Artifacts, value)
	}
	return "Deliverables: " + valueOrNone(strings.Join(m.options.Artifacts, ", ")), nil
}

func artifactPathOK(value string) error {
	clean := filepath.ToSlash(filepath.Clean(value))
	if value == "" || filepath.IsAbs(value) || clean != value || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("artifact path must be a clean output-relative path")
	}
	return nil
}

func (m *Model) configureConnectors(fields []string) (string, error) {
	if len(fields) == 1 || fields[1] == "list" {
		available := []string(nil)
		if m.config.ConnectorChoices != nil {
			available = m.config.ConnectorChoices()
		}
		return "Selected connectors: " + valueOrNone(strings.Join(m.options.ConnectorIDs, ", ")) +
			"\nAvailable: " + valueOrNone(strings.Join(available, ", ")) +
			"\nUse /connector setup ID GOOGLE_CLIENT_ID to configure Google Workspace.", nil
	}
	action := strings.ToLower(fields[1])
	switch action {
	case "clear":
		m.options.ConnectorIDs = nil
	case "remove":
		if len(fields) != 3 {
			return "", fmt.Errorf("usage: /connector remove ID")
		}
		m.options.ConnectorIDs = removeString(m.options.ConnectorIDs, fields[2])
	case "add":
		if len(fields) != 3 {
			return "", fmt.Errorf("usage: /connector add ID")
		}
		if m.config.ConnectorChoices != nil && !sliceContains(m.config.ConnectorChoices(), fields[2]) {
			return "", fmt.Errorf("connector %q is not configured", fields[2])
		}
		m.options.ConnectorIDs = appendUnique(m.options.ConnectorIDs, fields[2])
	case "setup":
		if len(fields) != 4 {
			return "", fmt.Errorf("usage: /connector setup ID GOOGLE_CLIENT_ID")
		}
		return "", m.startConnectorAction("setup", fields[2], []string{
			"add", fields[2], "--kind", "google", "--oauth-client-id", fields[3],
		})
	case "login":
		if len(fields) < 3 || len(fields) > 4 || len(fields) == 4 && strings.ToLower(fields[3]) != "prompt" {
			return "", fmt.Errorf("usage: /connector login ID [prompt]")
		}
		arguments := []string{"login", fields[2]}
		if len(fields) == 4 {
			arguments = append(arguments, "--prompt-client-secret")
		}
		return "", m.startConnectorCommand("login", fields[2], arguments)
	case "logout":
		if len(fields) != 3 {
			return "", fmt.Errorf("usage: /connector logout ID")
		}
		return "", m.startConnectorAction("logout", fields[2], []string{"logout", fields[2]})
	case "status":
		if len(fields) == 2 {
			return "", m.startConnectorAction("status", "", []string{"list"})
		}
		if len(fields) != 3 {
			return "", fmt.Errorf("usage: /connector status [ID]")
		}
		return "", m.startConnectorAction("status", fields[2], []string{"status", fields[2]})
	case "test":
		if len(fields) != 3 {
			return "", fmt.Errorf("usage: /connector test ID")
		}
		return "", m.startConnectorAction("test", fields[2], []string{"test", fields[2]})
	case "permission":
		if len(fields) != 6 {
			return "", fmt.Errorf("usage: /connector permission ID OPERATION read|write allow|ask|deny|draft")
		}
		return "", m.startConnectorAction("permission", fields[2], []string{
			"permission", fields[2], fields[3], strings.ToLower(fields[4]), strings.ToLower(fields[5]),
		})
	case "delete":
		if len(fields) != 3 {
			return "", fmt.Errorf("usage: /connector delete ID")
		}
		return "", m.startConnectorAction("delete", fields[2], []string{"remove", fields[2], "--yes"})
	default:
		if len(fields) != 2 {
			return "", connectorUsage()
		}
		if m.config.ConnectorChoices != nil && !sliceContains(m.config.ConnectorChoices(), fields[1]) {
			return "", fmt.Errorf("connector %q is not configured", fields[1])
		}
		m.options.ConnectorIDs = appendUnique(m.options.ConnectorIDs, fields[1])
	}
	return "Selected connectors: " + valueOrNone(strings.Join(m.options.ConnectorIDs, ", ")), nil
}

func connectorUsage() error {
	return fmt.Errorf("usage: /connector [list|add ID|remove ID|clear|setup ID CLIENT_ID|login ID [prompt]|logout ID|status [ID]|test ID|permission ID OPERATION read|write POLICY|delete ID]")
}

func (m *Model) startConnectorAction(action, id string, arguments []string) error {
	if m.config.ConnectorAction == nil {
		return errorsUnavailable("connector management")
	}
	m.status = "Connector " + action + "…"
	handler := m.config.ConnectorAction
	m.pendingConnectorCmd = func() tea.Msg {
		text, err := handler(arguments)
		return connectorActionDone{action: action, id: id, text: text, err: err}
	}
	return nil
}

func (m *Model) startConnectorCommand(action, id string, arguments []string) error {
	if m.config.ConnectorCommand == nil {
		return errorsUnavailable("connector login")
	}
	command := m.config.ConnectorCommand(arguments)
	if command == nil {
		return errorsUnavailable("connector login")
	}
	m.status = "Connector " + action + "…"
	m.pendingConnectorCmd = tea.ExecProcess(command, func(err error) tea.Msg {
		return connectorActionDone{action: action, id: id, err: err}
	})
	return nil
}

func (m *Model) configureWebOrigins(fields []string) (string, error) {
	if len(fields) == 1 || fields[1] == "list" {
		return "Selected web origins: " + valueOrNone(strings.Join(m.options.WebOrigins, ", ")), nil
	}
	switch strings.ToLower(fields[1]) {
	case "clear":
		m.options.WebOrigins = nil
	case "remove":
		if len(fields) != 3 {
			return "", fmt.Errorf("usage: /web-origin remove HTTPS_ORIGIN")
		}
		m.options.WebOrigins = removeString(m.options.WebOrigins, fields[2])
	case "add":
		if len(fields) != 3 {
			return "", fmt.Errorf("usage: /web-origin add HTTPS_ORIGIN")
		}
		m.options.WebOrigins = appendUnique(m.options.WebOrigins, fields[2])
	default:
		if len(fields) != 2 {
			return "", fmt.Errorf("usage: /web-origin [list|add ORIGIN|remove ORIGIN|clear]")
		}
		m.options.WebOrigins = appendUnique(m.options.WebOrigins, fields[1])
	}
	return "Selected web origins: " + valueOrNone(strings.Join(m.options.WebOrigins, ", ")), nil
}

func (m Model) prepareSave(raw string) (tea.Model, tea.Cmd) {
	if m.lastBundle.Path == "" {
		m.messages = append(m.messages, message{role: "Gator", text: "No verified deliverables are available yet."})
		return m, nil
	}
	if m.config.BundleAction == nil {
		m.messages = append(m.messages, message{role: "Gator", text: errorsUnavailable("save").Error()})
		return m, nil
	}
	replace := false
	target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "/save"))
	if target == "--replace" {
		replace, target = true, ""
	} else if strings.HasPrefix(target, "--replace ") {
		replace, target = true, strings.TrimSpace(strings.TrimPrefix(target, "--replace"))
	}
	if target == "" {
		target = m.source
	}
	request := BundleActionRequest{Action: "save", BundlePath: m.lastBundle.Path, Target: target, Replace: replace}
	summary, err := m.config.BundleAction(request)
	if err != nil {
		m.messages = append(m.messages, message{role: "Gator", text: err.Error()})
		return m, nil
	}
	m.pendingBundleAction = &pendingBundleAction{request: request, summary: summary}
	m.status = "Confirm saving verified deliverables"
	return m, nil
}

func (m Model) prepareCodeApply(raw string) (tea.Model, tea.Cmd) {
	if m.lastBundle.Path == "" || len(m.lastBundle.Candidates) == 0 {
		m.messages = append(m.messages, message{role: "Gator", text: "No verified Code candidate is available yet."})
		return m, nil
	}
	candidate := CandidateSummary{}
	target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "/apply"))
	for _, item := range m.lastBundle.Candidates {
		if target == item.ID || strings.HasPrefix(target, item.ID+" ") {
			candidate = item
			target = strings.TrimSpace(strings.TrimPrefix(target, item.ID))
			break
		}
	}
	if candidate.ID == "" {
		for _, item := range m.lastBundle.Candidates {
			if item.Status == "verified" {
				candidate = item
				break
			}
		}
	}
	if candidate.ID == "" {
		m.messages = append(m.messages, message{role: "Gator", text: "No verified Code candidate is available yet."})
		return m, nil
	}
	if target == "" {
		target = m.source
	}
	request := BundleActionRequest{Action: "apply-code", BundlePath: m.lastBundle.Path, Target: target, CandidateID: candidate.ID}
	summary, err := m.config.BundleAction(request)
	if err != nil {
		m.messages = append(m.messages, message{role: "Gator", text: err.Error()})
		return m, nil
	}
	m.pendingBundleAction = &pendingBundleAction{request: request, summary: summary}
	m.status = "Confirm applying verified Code candidate"
	return m, nil
}

func (m Model) prepareRetry(raw string) (tea.Model, tea.Cmd) {
	if m.lastBundle.Path == "" {
		m.messages = append(m.messages, message{role: "Gator", text: "No completed Work output is available yet."})
		return m, nil
	}
	if m.config.BundleAction == nil {
		m.messages = append(m.messages, message{role: "Gator", text: errorsUnavailable("retry").Error()})
		return m, nil
	}
	deliveryID := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "/retry"))
	request := BundleActionRequest{Action: "retry", BundlePath: m.lastBundle.Path, DeliveryID: deliveryID}
	summary, err := m.config.BundleAction(request)
	if err != nil {
		m.messages = append(m.messages, message{role: "Gator", text: err.Error()})
		return m, nil
	}
	m.pendingBundleAction = &pendingBundleAction{request: request, summary: summary}
	m.status = "Confirm retrying remaining local changes"
	return m, nil
}

func (m Model) updateBundleConfirmation(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch strings.ToLower(key.String()) {
	case "y", "enter":
		pending := *m.pendingBundleAction
		m.pendingBundleAction = nil
		pending.request.Execute = true
		m.status = "Applying reviewed output…"
		return m, func() tea.Msg {
			text, err := m.config.BundleAction(pending.request)
			return bundleActionDone{action: pending.request.Action, text: text, err: err}
		}
	case "n", "esc":
		m.pendingBundleAction = nil
		m.status = "Transfer cancelled"
	}
	return m, nil
}

func formatBundleSummary(bundle BundleSummary) string {
	lines := []string{"Artifacts:"}
	for _, file := range bundle.Artifacts {
		mark := "✓"
		if !file.Valid {
			mark = "!"
		}
		lines = append(lines, fmt.Sprintf("%s %s · %s · %d bytes", mark, file.Path, file.MediaType, file.Bytes))
	}
	for _, candidate := range bundle.Candidates {
		lines = append(lines, fmt.Sprintf("patch %s · %s · %s", candidate.ID, candidate.Status, strings.Join(candidate.ChangedPaths, ", ")))
	}
	if len(lines) == 0 {
		lines = append(lines, "No files were required for this inspection turn.")
	}
	lines = append(lines, "Use /review to preview, /save to keep files, /apply for a verified patch, or /retry after a failed delivery.")
	return strings.Join(lines, "\n")
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

func (m Model) configuredModelStatus() (string, string) {
	status := ModelStatus{}
	var err error
	if m.config.ModelStatus != nil {
		status, err = m.config.ModelStatus()
	} else if m.config.SelectedModel != nil {
		status.Provider, status.Model, err = m.config.SelectedModel()
	}
	if err != nil {
		return "unavailable", "status unavailable: " + singleLine(err.Error())
	}
	provider, modelName := singleLine(status.Provider), singleLine(status.Model)
	selection := valueOrNone(provider)
	if modelName != "" {
		selection += " / " + modelName
	}
	access := singleLine(status.Access)
	if access == "" {
		if provider == "" {
			access = "not configured"
		} else {
			access = "configuration selected; authentication status unavailable"
		}
	}
	return selection, access
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func (m Model) workStatus() string {
	modelSelection, modelAccess := m.configuredModelStatus()
	return fmt.Sprintf("Gator orchestration\n  model: %s\n  model access: %s\n  workspace: %s%s\n  ignored project instructions: %s\n  conversation: %s\n  selected revision: %s\n  mode: %s\n  code specialist: %s\n  deliverables: %s\n  connectors: %s\n  web origins: %s\n  effort: %s (%d manager steps)\n  pending attachments: %s\n  queued prompts: %d\n  internal specialists: managed by Gator",
		modelSelection, modelAccess,
		valueOrNone(m.source), map[bool]string{true: " (refresh next turn)", false: " (frozen per turn)"}[m.options.RefreshSource],
		valueOrNone(strings.Join(m.options.IgnoredInstructionPaths, ", ")),
		valueOrNone(m.conversation), valueOrNone(m.revision), valueOrNone(m.options.Mode), map[bool]string{true: "required", false: "optional"}[m.options.RequireCode], valueOrNone(strings.Join(m.options.Artifacts, ", ")),
		valueOrNone(strings.Join(m.options.ConnectorIDs, ", ")), valueOrNone(strings.Join(m.options.WebOrigins, ", ")),
		effortName(m.options.MaxSteps), m.options.MaxSteps,
		valueOrNone(strings.Join(m.options.Attachments, ", ")), len(m.queue))
}

func (m Model) workPermissions() string {
	return fmt.Sprintf("Gator Work authority\n  workspace: read-only immutable snapshots from %s\n  ignored project instructions: %s\n  mode: %s\n  connectors: %s\n  web origins: %s\n  internal specialists: managed by Gator",
		valueOrNone(m.source), valueOrNone(strings.Join(m.options.IgnoredInstructionPaths, ", ")), valueOrNone(m.options.Mode), valueOrNone(strings.Join(m.options.ConnectorIDs, ", ")),
		valueOrNone(strings.Join(m.options.WebOrigins, ", ")))
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

func cloneRunOptions(options RunOptions) RunOptions {
	result := options
	result.Attachments = append([]string(nil), options.Attachments...)
	result.Artifacts = append([]string(nil), options.Artifacts...)
	result.PreviousArtifacts = append([]string(nil), options.PreviousArtifacts...)
	result.ConnectorIDs = append([]string(nil), options.ConnectorIDs...)
	result.WebOrigins = append([]string(nil), options.WebOrigins...)
	result.IgnoredInstructionPaths = append([]string(nil), options.IgnoredInstructionPaths...)
	result.Code.Verification = append([]string(nil), options.Code.Verification...)
	result.Code.Scopes = append([]string(nil), options.Code.Scopes...)
	result.Code.Setup = append([]string(nil), options.Code.Setup...)
	result.Code.AllowedCommands = append([]string(nil), options.Code.AllowedCommands...)
	result.Code.AllowedCommandPrefixes = append([]string(nil), options.Code.AllowedCommandPrefixes...)
	result.Code.Capabilities = append([]string(nil), options.Code.Capabilities...)
	return result
}

func (m Model) previousArtifactPaths() []string {
	candidatePaths := map[string]bool{}
	for _, candidate := range m.lastBundle.Candidates {
		candidatePaths[candidate.PatchPath] = true
	}
	var paths []string
	for _, file := range m.lastBundle.Artifacts {
		if !candidatePaths[file.Path] {
			paths = append(paths, file.Path)
		}
	}
	if len(paths) == 0 {
		paths = append(paths, m.options.PreviousArtifacts...)
	}
	return paths
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
  /model                         manage cloud/local models, setup, sign-in, downloads and deletion
  /effort low|standard|high      set manager and Code turn budgets
  /attach PATH                   send one source file with the next prompt
  /detach PATH|all               remove pending attachments
  /source [PATH]                 inspect, select, or refresh the read-only workspace
  /source ignore PATH            skip one project instruction file for this conversation
  /source unignore PATH          read a skipped project instruction file again
  /source ignored                list skipped project instruction files
  /source-refresh                capture changed workspace files next turn
  /mode auto|inspect|draft|act   choose automatic or explicit authority
  /code on|off                   require retained Code patch evidence for Work
  /artifact [add|remove] PATH    manage expected deliverables
  /connector [add|remove] ID     select connected sources for this session
  /connector setup ID CLIENT_ID  configure Google Workspace with desktop OAuth
  /connector login ID [prompt]   authenticate and select a connector
  /connector status [ID]         inspect configured connectors and operations
  /connector test ID             verify live connector access
  /connector permission ID OPERATION read|write POLICY
                                  set allow/ask/deny/draft policy
  /connector logout|delete ID    remove credentials or the connector
  /web-origin [add|remove] URL   manage bounded web research origins
  /status · /permissions         inspect the active orchestration envelope
  /statusline                    choose, order, or hide composer footer items
  /doctor · /agents · /settings inspect local configuration
  /history · /revision-back · /revision-forward
                                 navigate retained Gator revisions
  /learnings [list|show|add|enable|disable|edit|remove|reject]
                                 inspect and control scoped guidance
  /feedback accept|reject|dont-learn [NOTE]
  /feedback correct|remember [--type TYPE] [--key KEY] [--scope SCOPE] TEXT
                                 record explicit feedback for this Work
  /steer TEXT                    steer the running task
  /tasks · /cancel-task ID        inspect or cancel active specialists
  /approve · /deny                respond to the displayed exact request
  /cancel                        cancel and retain the running outcome
  /queue · /dequeue · /clear-queue
  /review                        preview verified deliverables in this thread
  /save [--replace] [DIR]        preflight and save deliverables after confirmation
  /apply [CANDIDATE] [DIR]       preflight and apply verified code after confirmation
  /retry [DELIVERY_ID]            retry failed or pending local changes after confirmation
  /copy · /theme · /new · exit · /quit

Gator manages its internal specialists. /code makes the same bounded Code
specialist required by gator work code; it never opens a separate Code session.

Navigation
  ↑/↓       recall sent prompts in this conversation
  ctrl+g    edit the composer in $VISUAL or $EDITOR (nvim fallback)
  ctrl+i    choose a workspace directory
  ctrl+x    retained conversations
  ctrl+b    inbox
  ctrl+j    scheduled jobs
  ctrl+p    searchable command palette`
}
