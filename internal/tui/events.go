package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	gatorrun "github.com/gongahkia/gator/internal/run"
)

func (m *Model) appendEvent(event agent.Event) {
	m.events = append(m.events, renderEvent(event))
	m.observeActivity(event)
	m.appendChatEvent(event)
}

func (m *Model) appendChatEvent(event agent.Event) {
	switch event.Kind {
	case agent.EventTextDelta:
		if event.Text == "" {
			return
		}
		if len(m.chat) > 0 {
			last := &m.chat[len(m.chat)-1]
			if last.author == chatAgent && last.streaming {
				last.text += event.Text
				m.chatIndex = len(m.chat) - 1
				if m.followTranscript {
					m.syncTranscript(true)
				} else {
					m.syncTranscript(false)
					m.transcriptUnread = true
				}
				return
			}
		}
		m.appendChat(chatEntry{author: chatAgent, text: event.Text, streaming: true})
	case agent.EventText:
		m.closeStreamingChatEntry()
		if event.Text != "" {
			m.appendChat(chatEntry{author: chatAgent, text: event.Text})
		}
	case agent.EventSteeringApplied, agent.EventContextCompacted, agent.EventCommandApprovalRequested, agent.EventCommandApprovalResolved, agent.EventHook, agent.EventSubagent, agent.EventTerminal, agent.EventWorktreeSetup:
		m.closeStreamingChatEntry()
		entry := renderEvent(event)
		m.appendChat(chatEntry{author: chatSystem, text: entry.text})
	case agent.EventToolCalled, agent.EventToolFinished, agent.EventCompletionBlocked:
		m.closeStreamingChatEntry()
		entry := renderEvent(event)
		m.appendChat(chatEntry{author: chatTool, text: entry.text, detail: entry.detail})
	case agent.EventTurnStarted, agent.EventRunFinished:
		m.closeStreamingChatEntry()
	}
}

func (m *Model) appendCompletion(outcome gatorrun.Outcome, runErr error) {
	m.closeStreamingChatEntry()
	if runErr != nil {
		m.appendChat(chatEntry{author: chatSystem, text: "Run stopped: " + runErr.Error(), isError: true})
		return
	}
	if strings.TrimSpace(outcome.Result.FinalText) != "" {
		m.appendChat(chatEntry{author: chatAgent, text: outcome.Result.FinalText})
	}
	if outcome.Worktree.Path != "" {
		m.appendChat(chatEntry{author: chatSystem, text: "Run complete. Use /review to inspect the diff and patch handoff commands."})
	}
}

func (m *Model) appendChat(entry chatEntry) {
	m.chat = append(m.chat, entry)
	m.chatIndex = len(m.chat) - 1
	if m.followTranscript {
		m.syncTranscript(true)
	} else {
		m.syncTranscript(false)
		m.transcriptUnread = true
	}
}

func (m *Model) closeStreamingChatEntry() {
	if len(m.chat) == 0 {
		return
	}
	m.chat[len(m.chat)-1].streaming = false
}

func renderEvent(event agent.Event) timelineEntry {
	prefix := fmt.Sprintf("[%02d]", event.Step)
	var text, detail string
	switch event.Kind {
	case agent.EventTurnStarted:
		text = prefix + " agent turn started"
	case agent.EventTextDelta:
		text = prefix + " agent: " + compact(event.Text, 120)
		detail = event.Text
	case agent.EventText:
		text = prefix + " agent: " + compact(event.Text, 120)
		detail = event.Text
	case agent.EventToolCalled:
		if event.ToolCall != nil {
			text = prefix + " tool -> " + event.ToolCall.Name
			detail = describeToolCall(*event.ToolCall)
		} else {
			text = prefix + " tool -> unknown"
		}
	case agent.EventToolFinished:
		name := "unknown"
		if event.ToolCall != nil {
			name = event.ToolCall.Name
		}
		if event.ToolError == "" {
			text = prefix + " tool ok " + name
			detail = describeToolResult(name, event.ToolResult)
		} else {
			text = prefix + " tool failed " + name + ": " + compact(event.ToolError, 100)
			detail = event.ToolError
		}
	case agent.EventCompletionBlocked:
		text = prefix + " evidence required: " + compact(event.Text, 120)
	case agent.EventSteeringApplied:
		text = prefix + " steering accepted: " + compact(event.Text, 120)
	case agent.EventContextCompacted:
		text = prefix + " context compacted: " + compact(event.Text, 120)
	case agent.EventCommandApprovalRequested:
		text = prefix + " command approval: " + compact(event.Text, 120)
		if len(event.Argv) > 0 {
			detail = strings.Join(event.Argv, " ")
		}
	case agent.EventCommandApprovalResolved:
		text = prefix + " command " + compact(event.Text, 40)
		if len(event.Argv) > 0 {
			detail = strings.Join(event.Argv, " ")
		}
	case agent.EventHook:
		text = prefix + " hook " + compact(event.Text, 120)
	case agent.EventSubagent:
		text = prefix + " subagents " + compact(event.Text, 120)
	case agent.EventTerminal:
		text = prefix + " terminal " + compact(event.Text, 120)
	case agent.EventWorktreeSetup:
		text = prefix + " worktree setup " + compact(event.Text, 120)
	case agent.EventRunFinished:
		text = prefix + " completion proposed"
	default:
		text = prefix + " event"
	}
	return timelineEntry{step: event.Step, text: text, detail: detail, kind: event.Kind}
}

func describeToolCall(call agent.ToolCall) string {
	if call.Name == "apply_patch" {
		var arguments struct {
			Patch string `json:"patch"`
		}
		if json.Unmarshal(call.Arguments, &arguments) == nil {
			return patchPreview(arguments.Patch)
		}
	}
	if call.Name == "read_file" || call.Name == "list_files" || call.Name == "search_files" || call.Name == "http_fetch" || call.Name == "browser_navigate" {
		var arguments struct {
			Path  string `json:"path"`
			Query string `json:"query"`
			URL   string `json:"url"`
		}
		if json.Unmarshal(call.Arguments, &arguments) == nil {
			if arguments.Path != "" {
				return "path: " + arguments.Path
			}
			if arguments.Query != "" {
				return "query: " + arguments.Query
			}
			if arguments.URL != "" {
				return "url: " + arguments.URL
			}
		}
	}
	if call.Name == "browser_tabs" {
		return "developer-selected browser tabs"
	}
	if call.Name == "browser_snapshot" {
		return "current bounded browser snapshot"
	}
	if call.Name == "browser_screenshot" {
		return "browser screenshot (session visual grant required)"
	}
	if call.Name == "browser_click" || call.Name == "browser_download" {
		var arguments struct {
			TabID string `json:"tab_id"`
			Ref   string `json:"ref"`
		}
		if json.Unmarshal(call.Arguments, &arguments) == nil {
			return fmt.Sprintf("tab: %s · ref: %s", arguments.TabID, arguments.Ref)
		}
	}
	if call.Name == "browser_fill" || call.Name == "browser_select" {
		var arguments struct {
			TabID string `json:"tab_id"`
			Ref   string `json:"ref"`
		}
		if json.Unmarshal(call.Arguments, &arguments) == nil {
			return fmt.Sprintf("tab: %s · ref: %s · value hidden", arguments.TabID, arguments.Ref)
		}
	}
	if call.Name == "browser_press" {
		var arguments struct {
			TabID string `json:"tab_id"`
			Key   string `json:"key"`
		}
		if json.Unmarshal(call.Arguments, &arguments) == nil {
			return fmt.Sprintf("tab: %s · key: %s", arguments.TabID, arguments.Key)
		}
	}
	if call.Name == "browser_upload" {
		var arguments struct {
			TabID    string `json:"tab_id"`
			Ref      string `json:"ref"`
			UploadID string `json:"upload_id"`
		}
		if json.Unmarshal(call.Arguments, &arguments) == nil {
			return fmt.Sprintf("tab: %s · ref: %s · upload: %s", arguments.TabID, arguments.Ref, arguments.UploadID)
		}
	}
	if call.Name == "browser_extract" {
		var arguments struct {
			Kind  string `json:"kind"`
			Query string `json:"query"`
		}
		if json.Unmarshal(call.Arguments, &arguments) == nil {
			detail := "kind: " + arguments.Kind
			if arguments.Query != "" {
				detail += " · query: " + arguments.Query
			}
			return detail
		}
		return "bounded document extraction"
	}
	if call.Name == "browser_act" {
		var arguments struct {
			Action string            `json:"action"`
			Ref    string            `json:"ref"`
			Fields map[string]string `json:"fields"`
		}
		if json.Unmarshal(call.Arguments, &arguments) == nil {
			return fmt.Sprintf("action: %s · ref: %s · fields: %d (values hidden)", arguments.Action, arguments.Ref, len(arguments.Fields))
		}
		return "bounded browser action (field values hidden)"
	}
	if call.Name == "run_command" {
		var arguments struct {
			Argv    []string `json:"argv"`
			Command string   `json:"command"`
		}
		if json.Unmarshal(call.Arguments, &arguments) == nil {
			if len(arguments.Argv) > 0 {
				return "command: " + strings.Join(arguments.Argv, " ")
			}
			if arguments.Command != "" {
				return "command: " + arguments.Command
			}
		}
	}
	return string(call.Arguments)
}

func describeToolResult(name, result string) string {
	if result == "" {
		return ""
	}
	if name == "apply_patch" {
		return "patch applied"
	}
	return compact(result, 2_000)
}

func patchPreview(patch string) string {
	var files []string
	var lines []string
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			files = append(files, strings.TrimPrefix(line, "+++ b/"))
			continue
		}
		if (strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++")) || (strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---")) {
			lines = append(lines, line)
		}
	}
	if len(lines) > 12 {
		lines = append(lines[:12], "…")
	}
	summary := "edit"
	if len(files) > 0 {
		summary += " " + strings.Join(files, ", ")
	}
	if len(lines) > 0 {
		summary += "\n" + strings.Join(lines, "\n")
	}
	return summary
}

func compact(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	if limit == 1 {
		return "…"
	}
	return value[:limit-1] + "…"
}

func nextField(current field, reverse bool) field {
	if reverse {
		if current == taskField {
			return modelField
		}
		return current - 1
	}
	if current == modelField {
		return taskField
	}
	return current + 1
}

func formatVerification(commands [][]string) string {
	lines := make([]string, 0, len(commands))
	for _, command := range commands {
		if len(command) > 0 {
			lines = append(lines, strings.Join(command, " "))
		}
	}
	return strings.Join(lines, "\n")
}

func parseArgvLines(value string) ([][]string, error) {
	var commands [][]string
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		argv := strings.Fields(line)
		if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
			return nil, errors.New("each command line must start with a program token")
		}
		commands = append(commands, argv)
	}
	return commands, nil
}

func parseLineList(value string) []string {
	var items []string
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			items = append(items, line)
		}
	}
	return items
}

func parseVerification(value string) ([][]string, error) {
	commands, err := parseArgvLines(value)
	if err != nil {
		return nil, err
	}
	if len(commands) == 0 {
		return nil, errors.New("add at least one verification command before starting a run")
	}
	return commands, nil
}
