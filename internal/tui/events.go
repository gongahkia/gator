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
				return
			}
		}
		m.appendChat(chatEntry{author: chatAgent, text: event.Text, streaming: true})
	case agent.EventText:
		m.closeStreamingChatEntry()
		if event.Text != "" {
			m.appendChat(chatEntry{author: chatAgent, text: event.Text})
		}
	case agent.EventSteeringApplied:
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
		m.appendChat(chatEntry{author: chatSystem, text: "Run stopped: " + runErr.Error()})
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
	if call.Name == "read_file" || call.Name == "list_files" || call.Name == "search_files" {
		var arguments struct {
			Path  string `json:"path"`
			Query string `json:"query"`
		}
		if json.Unmarshal(call.Arguments, &arguments) == nil {
			if arguments.Path != "" {
				return "path: " + arguments.Path
			}
			if arguments.Query != "" {
				return "query: " + arguments.Query
			}
		}
	}
	if call.Name == "run_command" {
		var arguments struct {
			Argv []string `json:"argv"`
		}
		if json.Unmarshal(call.Arguments, &arguments) == nil {
			return "command: " + strings.Join(arguments.Argv, " ")
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

func parseVerification(value string) ([][]string, error) {
	var commands [][]string
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		argv := strings.Fields(line)
		if len(argv) == 0 {
			continue
		}
		commands = append(commands, argv)
	}
	if len(commands) == 0 {
		return nil, errors.New("add at least one verification command before starting a run")
	}
	return commands, nil
}
