package worktui

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/instructions"
	"github.com/gongahkia/gator/internal/jobs"
)

func (m *Model) openCommandPalette() {
	m.launcher = true
	m.launcherMode = "commands"
	m.entries = m.availableCommandEntries()
	m.paletteQuery = ""
	m.selected = 0
}

func (m *Model) openSlashPalette() {
	m.openCommandPalette()
	m.launcherMode = "slash"
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

func (m *Model) openGoalPicker() {
	if m.config.ListGoals != nil {
		goals, err := m.config.ListGoals()
		if err != nil {
			m.status = "List goals: " + err.Error()
		} else {
			m.config.Goals = goals
		}
	}
	m.launcher = true
	m.launcherMode = "goals"
	m.entries = nil
	for _, goal := range m.config.Goals {
		if goal.Status != "active" {
			continue
		}
		health := "No scheduled check · /followup new"
		for _, check := range m.checks {
			if check.GoalID == goal.ID {
				health = check.HealthLine()
				break
			}
		}
		m.entries = append(m.entries, entry{title: goal.Title, subtitle: health, kind: "goal", id: goal.ID})
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
		m.messages = append(m.messages, noticeMessage("Copy", "That item is no longer available to copy.", "warning"))
		return m, nil
	}
	if err := m.config.Copy(text); err != nil {
		m.messages = append(m.messages, noticeMessage("Copy failed", err.Error(), "error"))
		return m, nil
	}
	m.messages = append(m.messages, noticeMessage("Copied", "Copied "+label+".", "success"))
	m.scroll = 0
	return m, nil
}

func (m Model) filteredEntries() []entry {
	query := strings.ToLower(strings.TrimSpace(m.paletteQuery))
	entries := m.entries
	if m.launcherMode == "commands" || m.launcherMode == "slash" {
		entries = m.availableCommandEntries()
	}
	if query == "" && m.launcherMode != "commands" && m.launcherMode != "slash" {
		return entries
	}
	result := make([]entry, 0, len(entries))
	for _, item := range entries {
		if query == "" && item.advanced {
			continue
		}
		searchable := strings.ToLower(item.title + " " + item.subtitle + " " + item.command)
		if strings.Contains(searchable, query) {
			result = append(result, item)
		}
	}
	return result
}

func commandPaletteEntries() []entry {
	return []entry{
		{title: "/help", subtitle: "Show shortcuts and common commands", kind: "command", command: "/help"},
		{title: "/new", subtitle: "Start a new conversation", kind: "command", command: "/new"},
		{title: "/model", subtitle: "Choose a cloud or local model", kind: "command", command: "/model"},
		{title: "/source", subtitle: "Change the workspace", kind: "command", command: "/source"},
		{title: "/mode", subtitle: "Choose what Gator may do: auto, inspect, draft, or act", kind: "command-input", command: "/mode"},
		{title: "/attach", subtitle: "Attach a workspace file to the next request", kind: "command-input", command: "/attach"},
		{title: "/detach", subtitle: "Remove a pending attachment", kind: "command-input", command: "/detach"},
		{title: "/status", subtitle: "Show current model, workspace, and run", kind: "command", command: "/status"},
		{title: "/settings", subtitle: "Inspect Gator settings", kind: "command", command: "/settings"},
		{title: "/theme", subtitle: "Choose gator (green), contrast, or mono", kind: "command-input", command: "/theme"},
		{title: "/doctor", subtitle: "Check setup and device readiness", kind: "command", command: "/doctor"},
		{title: "/history", subtitle: "Browse retained work", kind: "command", command: "/history"},
		{title: "/jobs", subtitle: "Choose when Gator works: schedule and review checks", kind: "command-input", command: "/jobs"},
		{title: "/inbox", subtitle: "Review follow-up results", kind: "command-input", command: "/inbox"},
		{title: "/goals", subtitle: "Resume an ongoing goal", kind: "command", command: "/goals"},
		{title: "/followup", subtitle: "Watch project changes and review the next step", kind: "command", command: "/followup"},
		{title: "/overview", subtitle: "Changes, decisions, next actions, and blockers", kind: "command", command: "/overview"},
		{title: "/recovery", subtitle: "Reconnect sources and review offline checks", kind: "command", command: "/recovery"},
		{title: "/usage", subtitle: "Shared usage and limits across all work", kind: "command", command: "/usage"},
		{title: "/data", subtitle: "Retention, private backups, and deletion", kind: "command", command: "/data"},
		{title: "/context", subtitle: "Inspect sources and saved context supplied to this work", kind: "command", command: "/context"},
		{title: "/pilot", subtitle: "Review a project follow-up pilot and record outcomes", kind: "command-input", command: "/pilot", advanced: true},
		{title: "/plan", subtitle: "Show or update this goal's plan", kind: "command-input", command: "/plan"},
		{title: "/activity", subtitle: "Show checks and work for this goal", kind: "command", command: "/activity"},
		{title: "/notifications", subtitle: "Set alerts for this goal", kind: "command-input", command: "/notifications"},
		{title: "/suggestions", subtitle: "Review proposed lasting changes", kind: "command-input", command: "/suggestions"},
		{title: "/memory", subtitle: "Set up recall and manage saved context", kind: "command", command: "/memory"},
		{title: "/note", subtitle: "Save or search personal notes", kind: "command-input", command: "/note"},
		{title: "/skill", subtitle: "Create or manage reusable procedures", kind: "command-input", command: "/skill"},
		{title: "/learnings", subtitle: "Review guidance from earlier work", kind: "command", command: "/learnings"},
		{title: "/feedback", subtitle: "Rate or correct this result", kind: "command-input", command: "/feedback"},
		{title: "/review", subtitle: "Preview the latest output", kind: "command", command: "/review"},
		{title: "/save", subtitle: "Save deliverables to a folder", kind: "command-input", command: "/save"},
		{title: "/apply", subtitle: "Apply a verified code change", kind: "command-input", command: "/apply", advanced: true},
		{title: "/retry", subtitle: "Retry pending delivery", kind: "command-input", command: "/retry", advanced: true},
		{title: "/copy", subtitle: "Copy a response or output summary", kind: "command", command: "/copy"},
		{title: "/exit", subtitle: "Leave Gator", kind: "command", command: "/exit"},
		{title: "/effort", subtitle: "Choose low, standard, or high effort", kind: "command", command: "/effort", advanced: true},
		{title: "/artifact", subtitle: "Set expected output files", kind: "command-input", command: "/artifact", advanced: true},
		{title: "/connector", subtitle: "Select or manage connected sources", kind: "command-input", command: "/connector", advanced: true},
		{title: "/web-origin", subtitle: "Set allowed web origins", kind: "command-input", command: "/web-origin", advanced: true},
		{title: "/source-refresh", subtitle: "Refresh workspace snapshot next turn", kind: "command", command: "/source-refresh", advanced: true},
		{title: "/source ignore", subtitle: "Ignore a project instruction", kind: "command-input", command: "/source ignore", advanced: true},
		{title: "/source unignore", subtitle: "Restore a project instruction", kind: "command-input", command: "/source unignore", advanced: true},
		{title: "/source ignored", subtitle: "List ignored project instructions", kind: "command", command: "/source ignored", advanced: true},
		{title: "/statusline", subtitle: "Configure the optional status line", kind: "command", command: "/statusline", advanced: true},
		{title: "/permissions", subtitle: "Inspect current work authority", kind: "command", command: "/permissions", advanced: true},
		{title: "/agents", subtitle: "Inspect internal specialists", kind: "command", command: "/agents", advanced: true},
		{title: "/queue", subtitle: "Show queued prompts", kind: "command", command: "/queue"},
		{title: "/dequeue", subtitle: "Remove the next queued prompt", kind: "command", command: "/dequeue", advanced: true},
		{title: "/clear-queue", subtitle: "Remove all queued prompts", kind: "command", command: "/clear-queue", advanced: true},
		{title: "/revision-back", subtitle: "Move to the previous conversation revision", kind: "command", command: "/revision-back", advanced: true},
		{title: "/revision-forward", subtitle: "Move to the next conversation revision", kind: "command", command: "/revision-forward", advanced: true},
		{title: "/tasks", subtitle: "Show active specialists", kind: "command", command: "/tasks", advanced: true},
		{title: "/steer", subtitle: "Redirect the current run", kind: "command-input", command: "/steer", advanced: true},
		{title: "/cancel", subtitle: "Stop the current run", kind: "command", command: "/cancel"},
		{title: "/cancel-task", subtitle: "Stop a specialist task", kind: "command-input", command: "/cancel-task", advanced: true},
		{title: "/steer-task", subtitle: "Redirect a specialist task", kind: "command-input", command: "/steer-task", advanced: true},
		{title: "/approve", subtitle: "Approve the pending action", kind: "command", command: "/approve"},
		{title: "/deny", subtitle: "Deny the pending action", kind: "command", command: "/deny"},
	}
}

// The catalogue includes every TUI command. The open menu only contains
// commands that can be used in the current session; direct command entry still
// reaches the existing handlers and their detailed error messages.
func (m Model) availableCommandEntries() []entry {
	var available []entry
	for _, item := range commandPaletteEntries() {
		if m.commandAvailable(item.command) {
			available = append(available, item)
		}
	}
	return available
}

func (m Model) goalForConversation(conversationID string) string {
	for _, record := range m.config.Goals {
		if record.ConversationID == conversationID && record.Status == goal.Active {
			return record.ID
		}
	}
	return ""
}

func (m Model) commandAvailable(command string) bool {
	if m.running {
		switch command {
		case "/help", "/status":
			return true
		case "/tasks":
			return len(m.tasks) > 0
		case "/queue", "/dequeue", "/clear-queue":
			return len(m.queue) > 0
		case "/cancel":
			return m.cancel != nil
		case "/steer":
			return m.operation != nil
		case "/cancel-task", "/steer-task":
			return m.operation != nil && len(m.tasks) > 0
		case "/approve", "/deny":
			return m.operation != nil && m.interaction != nil
		default:
			return false
		}
	}
	switch command {
	case "/model":
		return m.config.Models != nil
	case "/source":
		return m.config.ResolveSource != nil
	case "/source-refresh", "/source ignore", "/attach":
		return m.source != ""
	case "/source unignore", "/source ignored":
		return len(m.options.IgnoredInstructionPaths) > 0
	case "/detach":
		return len(m.options.Attachments) > 0
	case "/settings", "/doctor", "/agents":
		return m.config.Inspect != nil
	case "/theme":
		return m.config.SetTheme != nil
	case "/statusline":
		return m.config.SetStatusLine != nil
	case "/connector":
		return m.config.ConnectorAction != nil || m.config.ConnectorChoices != nil
	case "/copy":
		if m.config.Copy == nil {
			return false
		}
		if m.lastBundle.Path != "" {
			return true
		}
		for _, item := range m.messages {
			if item.role == "Gator" && strings.TrimSpace(item.text) != "" {
				return true
			}
		}
		return false
	case "/queue", "/dequeue", "/clear-queue":
		return len(m.queue) > 0
	case "/jobs":
		return m.config.JobAction != nil || m.config.RefreshJobs != nil || len(m.config.Jobs) > 0
	case "/inbox":
		return m.config.InboxAction != nil || m.config.RefreshInbox != nil || len(m.config.Inbox) > 0
	case "/goals":
		return m.config.ListGoals != nil || len(m.config.Goals) > 0
	case "/plan":
		return m.goalID != "" && m.config.PlanAction != nil
	case "/activity":
		return m.goalID != "" && m.config.ActivityAction != nil
	case "/notifications":
		return m.goalID != "" && m.config.NotificationAction != nil
	case "/suggestions":
		return m.config.SuggestionAction != nil
	case "/memory":
		return m.config.MemoryAction != nil || m.config.RecallStatus != nil
	case "/followup":
		return m.config.SaveFollowup != nil
	case "/context":
		return m.config.ReadContext != nil
	case "/pilot":
		return m.config.PilotAction != nil
	case "/note":
		return m.config.NoteAction != nil
	case "/skill":
		return m.config.SkillAction != nil
	case "/history":
		return m.config.HistoryStore != nil
	case "/learnings":
		return m.config.LearningStore != nil
	case "/feedback":
		return m.revision != "" && m.config.FeedbackAction != nil
	case "/review":
		return m.lastBundle.Path != ""
	case "/save", "/retry":
		return m.lastBundle.Path != "" && m.config.BundleAction != nil
	case "/apply":
		if m.lastBundle.Path == "" || m.config.BundleAction == nil {
			return false
		}
		for _, candidate := range m.lastBundle.Candidates {
			if candidate.Status == "verified" {
				return true
			}
		}
		return false
	case "/revision-back":
		return m.conversation != "" && m.config.MoveBack != nil
	case "/revision-forward":
		return m.conversation != "" && (m.config.MoveForward != nil || m.config.MoveToRevision != nil)
	case "/tasks", "/steer", "/cancel", "/cancel-task", "/steer-task", "/approve", "/deny":
		return false
	default:
		return true
	}
}

func (m Model) runLocalCommand(command string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return m, nil
	}
	if m.config.BeforeLocalCommand != nil && fields[0] != "/data" && fields[0] != "/usage" && fields[0] != "/recovery" && fields[0] != "/overview" {
		release, err := m.config.BeforeLocalCommand()
		if err != nil {
			m.messages = append(m.messages, noticeMessage("Private data", err.Error(), "error"))
			return m, nil
		}
		defer release()
	}
	m.home = false
	m.section = ""
	var result string
	var err error
	switch fields[0] {
	case "/usage", "/data", "/recovery", "/overview":
		if m.config.Controls == nil {
			err = errorsUnavailable("product controls")
			break
		}
		cmd := m.openControls(strings.TrimPrefix(fields[0], "/"), fields[1:])
		return m, cmd
	case "/followup":
		if m.config.SaveFollowup == nil {
			err = errorsUnavailable("follow-up setup")
			break
		}
		cmd := m.openFollowup()
		if len(fields) == 2 && fields[1] == "new" {
			m.newFollowup()
		} else if len(fields) != 1 {
			err = fmt.Errorf("usage: /followup [new]")
			m.section = ""
			break
		}
		return m, cmd
	case "/context":
		if len(fields) > 2 {
			err = fmt.Errorf("usage: /context [WORK_ID]")
			break
		}
		id := ""
		if len(fields) == 2 {
			id = fields[1]
		}
		return m.openContext(id, false)
	case "/pilot":
		if m.config.PilotAction == nil {
			err = errorsUnavailable("pilot review")
			break
		}
		pilotArgs := fields[1:]
		for i, arg := range pilotArgs {
			if arg == "|" {
				pilotArgs = append(append([]string(nil), pilotArgs[:i]...), "--note", strings.Join(pilotArgs[i+1:], " "))
				break
			}
		}
		presented, pilotErr := m.config.PilotAction(m.lastWorkID, pilotArgs)
		if pilotErr != nil {
			err = pilotErr
			break
		}
		m.messages = append(m.messages, message{role: "Pilot", result: presented})
		return m, nil
	case "/help", "/?":
		if len(fields) == 1 {
			result = workHelp()
		} else if len(fields) == 2 && fields[1] == "advanced" {
			result = workAdvancedHelp()
		} else {
			err = fmt.Errorf("usage: /help [advanced]")
		}
	case "/new":
		if m.config.ClearGoal != nil {
			m.config.ClearGoal()
		}
		m.home, m.conversation, m.goalID = true, "", ""
		m.section, m.paletteQuery, m.launcherMode = "", "", ""
		m.source = m.config.CurrentFolder
		m.title = "Work in " + filepath.Base(m.source)
		m.revision, m.snapshot = "", ""
		m.clearPromptHistory()
		m.messages, m.status, m.queue = nil, "", nil
		m.lastOutput, m.lastBundle = "", BundleSummary{}
		m.options = RunOptions{MaxSteps: 24, Mode: "auto", Code: CodeOptions{MaxSteps: 16, Sandbox: "strict", Network: "deny"}}
		m.refreshAttention()
		return m, nil
	case "/model", "/models":
		if len(fields) != 1 {
			result = "Use /model to choose a cloud API key or local model."
			break
		}
		if m.config.Models == nil {
			err = errorsUnavailable("model management")
			break
		}
		return m.openModels()
	case "/effort":
		if len(fields) == 1 {
			m.section = "effort"
			m.selected = effortIndex(m.options.MaxSteps)
			return m, nil
		}
		if len(fields) != 2 {
			err = fmt.Errorf("usage: /effort low|standard|high")
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
			if m.config.ClearGoal != nil {
				m.config.ClearGoal()
			}
			m.goalID = ""
			if m.source != resolved {
				m.conversation = ""
				m.revision = ""
				m.messages = nil
			}
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
	case "/settings":
		if len(fields) == 2 && fields[1] == "statusline" {
			m.openStatusLineEditor()
			return m, nil
		}
		if len(fields) != 1 {
			err = fmt.Errorf("usage: /settings [statusline]")
			break
		}
		fallthrough
	case "/doctor", "/agents":
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
			if m.config.ReadChecks != nil {
				cmd := m.openFollowup()
				return m, cmd
			}
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
		if err == nil {
			if refreshErr := m.refreshInbox(); refreshErr != nil {
				result += "\nInbox refresh failed: " + refreshErr.Error()
			}
		}
	case "/inbox":
		if len(fields) == 1 {
			if err = m.refreshInbox(); err == nil {
				m.section = "inbox"
				m.sectionNotice = ""
				return m, nil
			}
			break
		}
		if m.config.InboxAction == nil {
			err = errorsUnavailable("inbox")
			break
		}
		result, err = m.config.InboxAction(fields[1:])
		if err == nil {
			if refreshErr := m.refreshInbox(); refreshErr != nil {
				result += "\nInbox refresh failed: " + refreshErr.Error()
			}
		}
	case "/goals":
		if len(fields) != 1 {
			err = fmt.Errorf("usage: /goals")
			break
		}
		m.openGoalPicker()
		return m, nil
	case "/plan":
		if m.config.PlanAction == nil {
			err = errorsUnavailable("goal plans")
			break
		}
		result, err = m.config.PlanAction(commandRemainder(command, fields[:1]))
	case "/activity":
		if len(fields) != 1 {
			err = fmt.Errorf("usage: /activity")
			break
		}
		if m.config.ActivityAction == nil {
			err = errorsUnavailable("goal activity")
			break
		}
		result, err = m.config.ActivityAction()
	case "/notifications":
		if m.config.NotificationAction == nil {
			err = errorsUnavailable("goal notifications")
			break
		}
		result, err = m.config.NotificationAction(fields[1:])
	case "/suggestions":
		if m.config.SuggestionAction == nil {
			err = errorsUnavailable("suggestions")
			break
		}
		result, err = m.config.SuggestionAction(fields[1:])
	case "/memory":
		if len(fields) == 1 && m.config.RecallStatus != nil {
			cmd := m.openRecall()
			return m, cmd
		}
		if len(fields) >= 3 && fields[1] == "embeddings" && fields[2] == "install" && m.config.InstallRecall != nil {
			m.section, m.home = "memory", false
			if len(fields) == 4 && fields[3] == "--yes" {
				return m.startRecallInstall()
			}
			if len(fields) == 3 {
				m.recallConfirm = "install"
				return m, nil
			}
		}
		if m.config.MemoryAction == nil {
			err = errorsUnavailable("memory")
			break
		}
		result, err = m.config.MemoryAction(fields[1:])
	case "/note":
		if m.config.NoteAction == nil {
			err = errorsUnavailable("notes")
			break
		}
		result, err = m.config.NoteAction(commandRemainder(command, fields[:1]))
	case "/skill":
		if m.config.SkillAction == nil {
			err = errorsUnavailable("skills")
			break
		}
		result, err = m.config.SkillAction(commandRemainder(command, fields[:1]))
	case "/history":
		if len(fields) > 2 {
			err = fmt.Errorf("usage: /history [all|running|completed|failed]")
			break
		}
		if len(fields) == 1 {
			m.historyFilter = ""
		} else {
			err = m.setHistoryFilter(fields[1])
			if err != nil {
				break
			}
		}
		m.openGlobalHistory()
		return m, nil
	case "/learnings":
		if len(fields) > 2 {
			err = fmt.Errorf("usage: /learnings [all|active|candidate|disabled|rejected]")
			break
		}
		if len(fields) == 1 {
			m.learningFilter = ""
		} else {
			err = m.setLearningFilter(fields[1])
			if err != nil {
				break
			}
		}
		m.openLearnings()
		return m, nil
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
	case "/revision-back", "/revision-forward":
		if m.conversation == "" {
			err = fmt.Errorf("start or resume a conversation before using revision history")
			break
		}
		switch fields[0] {
		case "/revision-back":
			if m.config.MoveBack != nil {
				result, err = m.config.MoveBack(m.conversation)
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
	case "exit", "/exit":
		return m, tea.Quit
	default:
		err = fmt.Errorf("unknown command %s; use /help", fields[0])
	}
	if err != nil {
		result = err.Error()
	}
	role := "Gator"
	if fields[0] == "/doctor" && err == nil {
		role = "Doctor"
	}
	m.messages = append(m.messages, message{role: role, text: result, result: m.commandResult(command, fields, result, err)})
	m.scroll = 0
	if fields[0] == "/inbox" || fields[0] == "/suggestions" {
		m.refreshAttention()
	}
	return m, nil
}

func (m *Model) refreshAttention() {
	if m.config.RefreshAttention == nil {
		return
	}
	attention, err := m.config.RefreshAttention()
	if err != nil {
		m.status = "Home summary: " + err.Error()
		return
	}
	m.config.Attention = attention
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
	if m.config.BridgeMode {
		m.bridgeExternal = bridgeExternal("connector", action, id, command)
		return nil
	}
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
		m.messages = append(m.messages, noticeMessage("Save", "No verified deliverables are available yet.", "warning"))
		return m, nil
	}
	if m.config.BundleAction == nil {
		m.messages = append(m.messages, noticeMessage("Save unavailable", errorsUnavailable("save").Error(), "error"))
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
		m.messages = append(m.messages, noticeMessage("Save failed", err.Error(), "error"))
		return m, nil
	}
	m.pendingBundleAction = &pendingBundleAction{request: request, summary: summary}
	m.status = "Confirm saving verified deliverables"
	return m, nil
}

func (m Model) prepareCodeApply(raw string) (tea.Model, tea.Cmd) {
	if m.lastBundle.Path == "" || len(m.lastBundle.Candidates) == 0 {
		m.messages = append(m.messages, noticeMessage("Apply", "No verified Code candidate is available yet.", "warning"))
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
		m.messages = append(m.messages, noticeMessage("Apply", "No verified Code candidate is available yet.", "warning"))
		return m, nil
	}
	if target == "" {
		target = m.source
	}
	request := BundleActionRequest{Action: "apply-code", BundlePath: m.lastBundle.Path, Target: target, CandidateID: candidate.ID}
	summary, err := m.config.BundleAction(request)
	if err != nil {
		m.messages = append(m.messages, noticeMessage("Apply failed", err.Error(), "error"))
		return m, nil
	}
	m.pendingBundleAction = &pendingBundleAction{request: request, summary: summary}
	m.status = "Confirm applying verified Code candidate"
	return m, nil
}

func (m Model) prepareRetry(raw string) (tea.Model, tea.Cmd) {
	if m.lastBundle.Path == "" {
		m.messages = append(m.messages, noticeMessage("Retry", "No completed Work output is available yet.", "warning"))
		return m, nil
	}
	if m.config.BundleAction == nil {
		m.messages = append(m.messages, noticeMessage("Retry unavailable", errorsUnavailable("retry").Error(), "error"))
		return m, nil
	}
	deliveryID := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "/retry"))
	request := BundleActionRequest{Action: "retry", BundlePath: m.lastBundle.Path, DeliveryID: deliveryID}
	summary, err := m.config.BundleAction(request)
	if err != nil {
		m.messages = append(m.messages, noticeMessage("Retry failed", err.Error(), "error"))
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

func (m Model) workTiming() string {
	enabled := 0
	for _, job := range m.config.Jobs {
		if job.Enabled {
			enabled++
		}
	}
	if enabled == 0 {
		return "on request; no enabled scheduled checks (/jobs)"
	}
	if m.config.SupervisorRunning {
		return fmt.Sprintf("on request; %d enabled scheduled checks (/jobs)", enabled)
	}
	return fmt.Sprintf("on request; %d enabled scheduled checks, supervisor stopped (/jobs)", enabled)
}

func (m Model) workAuthority() string {
	switch m.options.Mode {
	case "", "auto":
		return "auto: inspect or draft; external actions require foreground approval"
	case "inspect":
		return "inspect only"
	case "draft":
		return "draft for review"
	case "act":
		return "act with foreground approval"
	default:
		return valueOrNone(m.options.Mode)
	}
}

func (m Model) workStatus() string {
	modelSelection, modelAccess := m.configuredModelStatus()
	status := fmt.Sprintf("Gator orchestration\n  when Gator works: %s\n  what Gator may do: %s (/mode)\n  model: %s\n  model access: %s\n  workspace: %s%s\n  ignored project instructions: %s\n  conversation: %s\n  selected revision: %s\n  mode: %s\n  deliverables: %s\n  connectors: %s\n  web origins: %s\n  effort: %s (%d manager steps)\n  pending attachments: %s\n  queued prompts: %d\n  internal specialists: managed by Gator",
		m.workTiming(), m.workAuthority(),
		modelSelection, modelAccess,
		valueOrNone(m.source), map[bool]string{true: " (refresh next turn)", false: " (frozen per turn)"}[m.options.RefreshSource],
		valueOrNone(strings.Join(m.options.IgnoredInstructionPaths, ", ")),
		valueOrNone(m.conversation), valueOrNone(m.revision), valueOrNone(m.options.Mode), valueOrNone(strings.Join(m.options.Artifacts, ", ")),
		valueOrNone(strings.Join(m.options.ConnectorIDs, ", ")), valueOrNone(strings.Join(m.options.WebOrigins, ", ")),
		effortName(m.options.MaxSteps), m.options.MaxSteps,
		valueOrNone(strings.Join(m.options.Attachments, ", ")), len(m.queue))
	if !m.running {
		return status + "\n  current work: idle"
	}
	status += "\n  current work: running (/cancel)"
	if m.operation != nil {
		status += "; steer with /steer MESSAGE"
		if m.operation.RunID != "" {
			status += "\n  work run: " + m.operation.RunID + " (inspect from another terminal: gator work decisions " + m.operation.RunID + ")"
		}
	}
	activeTasks := make([]string, 0, len(m.tasks))
	for id, taskStatus := range m.tasks {
		if strings.HasSuffix(taskStatus, ": queued") || strings.HasSuffix(taskStatus, ": running") {
			activeTasks = append(activeTasks, id+" "+taskStatus)
		}
	}
	sort.Strings(activeTasks)
	status += fmt.Sprintf("\n  live specialists: %d", len(activeTasks))
	if m.operation != nil {
		status += " (/tasks, /cancel-task ID, /steer-task ID MESSAGE)"
	}
	for index, task := range activeTasks {
		if index == 3 {
			status += fmt.Sprintf("\n    ... and %d more", len(activeTasks)-3)
			break
		}
		status += "\n    " + task
	}
	pending := len(m.pendingInteractions)
	if m.interaction != nil {
		pending++
	}
	status += fmt.Sprintf("\n  pending approvals: %d", pending)
	if m.interaction != nil {
		status += fmt.Sprintf(" (current: %s #%d; /approve or /deny)", m.interaction.Kind, m.interaction.ID)
	}
	return status
}

func (m Model) workPermissions() string {
	return fmt.Sprintf("Gator Work authority\n  when Gator works: %s\n  what Gator may do: %s (/mode)\n  workspace: read-only immutable snapshots from %s\n  ignored project instructions: %s\n  mode: %s\n  connectors: %s\n  web origins: %s\n  internal specialists: managed by Gator",
		m.workTiming(), m.workAuthority(),
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
	result.ExcludedKnowledgeIDs = append([]string(nil), options.ExcludedKnowledgeIDs...)
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

func (m *Model) refreshInbox() error {
	if m.config.RefreshInbox == nil {
		return nil
	}
	entries, err := m.config.RefreshInbox()
	if err != nil {
		return err
	}
	m.config.Inbox = entries
	return nil
}

func workHelp() string {
	return `Gator
  Start Work by writing your request below. Gator retains its output and follows
  the current workspace, model, and mode.

  /model                         choose Cloud API key or Local model
  /source [PATH]                 choose the workspace
  /mode auto|inspect|draft|act   choose Work authority
  /artifact [add|remove] PATH    set expected deliverables
  /history [STATUS]              browse all retained Work
  /overview                     changes, decisions, next actions, and blockers
  /followup [new]                set up and manage project follow-ups
  /recovery                     reconnect, retry, and review offline coverage
  /usage                        review shared usage, limits, and pause
  /data                         retain, export, verify, or delete private content
  /jobs [list|add|show|edit|enable|disable|run|history|remove]
                                 schedule and review repeatable Work
  /inbox [show|read|rate|report] review and rate follow-up results
  /goals                         resume an ongoing goal
  /plan [outcome|add|step|review-at|reviewed|remove]
                                 inspect or update the current goal plan
  /activity                     show goal checks, running Work, and decisions
  /notifications [--desktop on|off] [--urgent-only on|off] [--quiet HH:MM-HH:MM --timezone AREA/CITY | --clear-quiet]
                                 set desktop alerts for the current goal
  /suggestions [list|show|accept|reject]
                                 review proposed durable changes
  /memory                        set up recall; inspect data destinations
  /context [WORK_ID]              review sources and saved context; correct or omit
  /memory add|list|search|show|correct|forget ...
                                 manage explicit saved knowledge
  /note add TITLE | BODY          save a Markdown note in the current goal
  /note list|search|show|edit     find and revise saved notes
  /skill create NAME | WHEN | BODY
                                 save a reusable text procedure
  /skill list|show|edit|enable|disable
                                 inspect and manage procedures
  /learnings [STATUS]            browse and manage guidance for future Work
  /feedback accept|reject|dont-learn [NOTE]
  /feedback correct|remember [OPTIONS] TEXT
                                 record feedback for this Work
  /review · /save · /apply · /retry
                                 review and deliver a completed result
  /status · /doctor · /settings [statusline] · /theme · /new · /copy · /exit

Type / in an empty prompt to browse commands. Search also finds advanced controls.
Use /help advanced for workspace, integration, revision, and queue details.

Navigation
  ↑/↓       recall sent prompts in this conversation
  ctrl+g    edit the composer in $VISUAL or $EDITOR (nvim fallback)
  ctrl+i    choose a workspace directory
  ctrl+x    retained conversations
  ctrl+b    inbox
  ctrl+j    insert a new line in the composer
  /followup scheduled checks and recovery
  ctrl+p    searchable command palette`
}

func workAdvancedHelp() string {
	return `Advanced controls
  /effort low|standard|high · /attach PATH · /detach PATH|all
  /source ignore|unignore|ignored PATH · /source-refresh
  /connector ... · /web-origin ...
  /status · /permissions · /statusline · /doctor · /agents
  /pilot guide|record|report      review observed project follow-up outcomes
  /revision-back · /revision-forward [REVISION]
  /queue · /dequeue · /clear-queue
  /tasks · /cancel-task ID · /steer-task ID MESSAGE · /steer MESSAGE

These controls preserve the same Work semantics. Type / and search for a name
to find a command when it applies to the current conversation.`
}
