package worktui

import (
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/uisnapshot"
)

var commandResultTitles = map[string]string{
	"/help": "Gator commands", "/?": "Gator commands", "/effort": "Effort", "/mode": "Work mode",
	"/status": "Current session", "/permissions": "Work authority", "/settings": "Settings",
	"/agents": "Internal specialists", "/queue": "Queued prompts", "/dequeue": "Queued prompts",
	"/clear-queue": "Queued prompts", "/plan": "Goal plan", "/activity": "Activity",
	"/notifications": "Notifications", "/suggestions": "Suggestions", "/memory": "Memory",
	"/note": "Notes", "/skill": "Skills", "/review": "Deliverable preview",
	"/feedback": "Feedback", "/artifact": "Expected files", "/connector": "Connected sources",
	"/web-origin": "Allowed web origins", "/source": "Workspace", "/source-refresh": "Workspace",
	"/attach": "Attachment", "/detach": "Attachment", "/theme": "Theme", "/jobs": "Scheduled jobs",
	"/inbox": "Inbox", "/copy": "Copy", "/revision-back": "Conversation revision",
	"/revision-forward": "Conversation revision", "/model": "Model",
}

func commandResultTitle(command string) string {
	if title, ok := commandResultTitles[command]; ok {
		return title
	}
	return strings.TrimPrefix(command, "/")
}

func effortIndex(steps int) int {
	switch effortName(steps) {
	case "low":
		return 0
	case "high":
		return 2
	default:
		return 1
	}
}

func (m Model) commandResult(command string, fields []string, text string, runErr error) *uisnapshot.Result {
	name := fields[0]
	if name == "/doctor" && runErr == nil {
		return nil // The diagnostic report has its own React presentation.
	}
	title := commandResultTitle(name)
	if runErr != nil {
		return &uisnapshot.Result{Kind: "notice", Title: title, Tone: "error", Summary: text}
	}
	switch name {
	case "/status":
		return m.statusResult()
	case "/permissions":
		return m.permissionsResult()
	case "/queue", "/dequeue", "/clear-queue":
		if len(m.queue) == 0 {
			return &uisnapshot.Result{Kind: "notice", Title: title, Summary: "No prompts are queued."}
		}
		items := make([]uisnapshot.ResultItem, 0, len(m.queue))
		for index, item := range m.queue {
			items = append(items, uisnapshot.ResultItem{Label: fmt.Sprintf("%d", index+1), Detail: truncate(item.prompt, 160)})
		}
		return &uisnapshot.Result{Kind: "list", Title: title, Summary: fmt.Sprintf("%d pending", len(items)), Items: items}
	case "/memory":
		if m.config.MemoryResult != nil {
			if presented, err := m.config.MemoryResult(fields[1:]); err == nil && presented != nil {
				return presented
			}
		}
	case "/note":
		if m.config.NoteResult != nil {
			if presented, err := m.config.NoteResult(commandRemainder(command, fields[:1])); err == nil && presented != nil {
				return presented
			}
		}
	case "/skill":
		if m.config.SkillResult != nil {
			if presented, err := m.config.SkillResult(commandRemainder(command, fields[:1])); err == nil && presented != nil {
				return presented
			}
		}
	case "/plan":
		if m.config.PlanResult != nil {
			if presented, err := m.config.PlanResult(commandRemainder(command, fields[:1])); err == nil && presented != nil {
				return presented
			}
		}
	case "/activity":
		if m.config.ActivityResult != nil {
			if presented, err := m.config.ActivityResult(); err == nil && presented != nil {
				return presented
			}
		}
	case "/review":
		if m.lastBundle.Path != "" && m.config.ReviewBundle != nil {
			if presented, err := m.config.ReviewBundle(m.lastBundle.Path); err == nil && presented != nil {
				return presented
			}
		}
	}
	if strings.TrimSpace(text) == "" {
		return &uisnapshot.Result{Kind: "notice", Title: title, Summary: "Done."}
	}
	if !strings.Contains(text, "\n") {
		tone := ""
		switch name {
		case "/effort", "/mode", "/theme", "/attach", "/detach", "/source-refresh", "/feedback":
			tone = "success"
		}
		return &uisnapshot.Result{Kind: "notice", Title: title, Tone: tone, Summary: text}
	}
	return &uisnapshot.Result{Kind: "report", Title: title, Lines: strings.Split(strings.TrimSpace(text), "\n")}
}

func (m Model) statusResult() *uisnapshot.Result {
	selection, access := m.configuredModelStatus()
	work := "Idle"
	if m.running {
		work = "Running"
	}
	result := &uisnapshot.Result{Kind: "details", Title: "Current session", Fields: []uisnapshot.Field{
		{Label: "Model", Value: selection}, {Label: "Access", Value: access},
		{Label: "Workspace", Value: valueOrNone(m.source)},
		{Label: "Workspace refresh", Value: map[bool]string{true: "Next turn", false: "Frozen per turn"}[m.options.RefreshSource]},
		{Label: "Ignored instructions", Value: valueOrNone(strings.Join(m.options.IgnoredInstructionPaths, ", "))},
		{Label: "Conversation", Value: valueOrNone(m.conversation)},
		{Label: "Revision", Value: valueOrNone(m.revision)},
		{Label: "Mode", Value: m.options.Mode},
		{Label: "Deliverables", Value: valueOrNone(strings.Join(m.options.Artifacts, ", "))},
		{Label: "Effort", Value: fmt.Sprintf("%s · %d manager steps", effortName(m.options.MaxSteps), m.options.MaxSteps)},
		{Label: "Connectors", Value: valueOrNone(strings.Join(m.options.ConnectorIDs, ", "))},
		{Label: "Web origins", Value: valueOrNone(strings.Join(m.options.WebOrigins, ", "))},
		{Label: "Attachments", Value: valueOrNone(strings.Join(m.options.Attachments, ", "))},
		{Label: "Queued prompts", Value: fmt.Sprint(len(m.queue))},
		{Label: "Current work", Value: work},
	}}
	if m.running {
		if m.operation != nil {
			result.Fields = append(result.Fields, uisnapshot.Field{Label: "Run", Value: valueOrNone(m.operation.RunID)})
		}
		pending := len(m.pendingInteractions)
		if m.interaction != nil {
			pending++
		}
		result.Fields = append(result.Fields, uisnapshot.Field{Label: "Pending approvals", Value: fmt.Sprint(pending)})
		for id, status := range m.tasks {
			if strings.HasSuffix(status, ": queued") || strings.HasSuffix(status, ": running") {
				result.Items = append(result.Items, uisnapshot.ResultItem{Label: id, Detail: status})
			}
		}
		result.Footer = "/cancel to stop · /tasks to inspect specialists"
	}
	return result
}

func (m Model) permissionsResult() *uisnapshot.Result {
	return &uisnapshot.Result{Kind: "details", Title: "Work authority", Summary: "Workspace snapshots are read-only.", Fields: []uisnapshot.Field{
		{Label: "Workspace", Value: valueOrNone(m.source)},
		{Label: "Ignored instructions", Value: valueOrNone(strings.Join(m.options.IgnoredInstructionPaths, ", "))},
		{Label: "Mode", Value: m.options.Mode},
		{Label: "Connectors", Value: valueOrNone(strings.Join(m.options.ConnectorIDs, ", "))},
		{Label: "Web origins", Value: valueOrNone(strings.Join(m.options.WebOrigins, ", "))},
	}}
}
