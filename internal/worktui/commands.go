package worktui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/instructions"
	"github.com/gongahkia/gator/internal/jobs"
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
		{title: "/source", subtitle: "Choose the workspace for this conversation", kind: "command", command: "/source"},
		{title: "/mode", subtitle: "Set auto, inspect, draft, or act", kind: "command-input", command: "/mode"},
		{title: "/settings", subtitle: "Inspect current settings", kind: "command", command: "/settings"},
		{title: "/theme", subtitle: "Choose gator, contrast, or mono", kind: "command-input", command: "/theme"},
		{title: "/history", subtitle: "Show Work executions in this conversation", kind: "command", command: "/history"},
		{title: "/jobs", subtitle: "Schedule, run, and review repeatable Work", kind: "command-input", command: "/jobs"},
		{title: "/learnings", subtitle: "Inspect and control scoped guidance for future Work", kind: "command", command: "/learnings"},
		{title: "/feedback", subtitle: "Accept, reject, correct, or remember this Work result", kind: "command-input", command: "/feedback"},
		{title: "/review", subtitle: "Show latest staged output", kind: "command", command: "/review"},
		{title: "/save", subtitle: "Save verified deliverables to a folder", kind: "command-input", command: "/save"},
		{title: "/apply", subtitle: "Apply a verified Code candidate", kind: "command-input", command: "/apply"},
		{title: "/retry", subtitle: "Retry failed or pending local changes", kind: "command-input", command: "/retry"},
		{title: "/copy", subtitle: "Choose a response or deliverable summary to copy", kind: "command", command: "/copy"},
		{title: "exit", subtitle: "Exit Gator", kind: "command", command: "exit"},
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
		if len(fields) == 1 {
			result = workHelp()
		} else if len(fields) == 2 && fields[1] == "advanced" {
			result = workAdvancedHelp()
		} else {
			err = fmt.Errorf("usage: /help [advanced]")
		}
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
	case "/statusline":
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
	case "/jobs":
		if len(fields) == 1 {
			m.section = "jobs"
			return m, nil
		}
		if m.config.JobAction == nil {
			err = errorsUnavailable("jobs")
			break
		}
		result, err = m.config.JobAction(fields[1:])
		if err == nil && m.config.RefreshJobs != nil {
			var refreshed []jobs.Definition
			refreshed, err = m.config.RefreshJobs()
			if err == nil {
				m.config.Jobs = refreshed
			}
		}
	case "/learnings":
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
	case "exit":
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
	return `Gator
  Start Work by writing your request below. Gator retains its output and follows
  the current workspace, model, and mode.

  /model                         choose or set up a model
  /source [PATH]                 choose the workspace
  /mode auto|inspect|draft|act   choose Work authority
  /code on|off                   require a verified code change
  /artifact [add|remove] PATH    set expected deliverables
  /history                       review this conversation's Work
  /jobs [list|add|show|edit|enable|disable|run|history|remove]
                                 schedule and review repeatable Work
  /learnings [list|show|add|approve|enable|disable|edit|remove|reject]
                                 control guidance for future Work
  /feedback accept|reject|dont-learn [NOTE]
  /feedback correct|remember [OPTIONS] TEXT
                                 record feedback for this Work
  /review · /save · /apply · /retry
                                 review and deliver a completed result
  /settings · /theme · /new · /copy · exit

Use /help advanced for workspace, integration, revision, queue, and diagnostic controls.

Navigation
  ↑/↓       recall sent prompts in this conversation
  ctrl+g    edit the composer in $VISUAL or $EDITOR (nvim fallback)
  ctrl+i    choose a workspace directory
  ctrl+x    retained conversations
  ctrl+b    inbox
  ctrl+j    scheduled jobs
  ctrl+p    searchable command palette`
}

func workAdvancedHelp() string {
	return `Advanced controls
  /effort low|standard|high · /attach PATH · /detach PATH|all
  /source ignore|unignore|ignored PATH · /source-refresh
  /connector ... · /web-origin ...
  /status · /permissions · /statusline · /doctor · /agents
  /revision-back · /revision-forward [REVISION]
  /queue · /dequeue · /clear-queue

These controls preserve the same Work semantics; they are intentionally kept
out of the normal command palette. Use /help to return to the main workflow.`
}
