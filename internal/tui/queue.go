package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) selectedCommandIsExact() bool {
	matches := m.matchingCommands()
	if len(matches) == 0 || m.commandIndex < 0 || m.commandIndex >= len(matches) {
		return false
	}
	return strings.TrimSpace(m.task.Value()) == matches[m.commandIndex].name
}

func queuedInputForText(text string) (queuedInput, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return queuedInput{}, fmt.Errorf("write an instruction before queueing it")
	}
	if strings.HasPrefix(text, "!") {
		return queuedInput{}, fmt.Errorf("direct shell commands are not supported; configure a verifier with /verify instead")
	}
	if !strings.HasPrefix(text, "/") {
		return queuedInput{kind: queuedPrompt, text: text}, nil
	}
	for _, command := range slashCommands {
		if text == command.name {
			return queuedInput{kind: queuedCommand, text: text}, nil
		}
	}
	return queuedInput{}, fmt.Errorf("unknown Gator command %q", text)
}

func (m Model) queueCurrentInput() (tea.Model, tea.Cmd) {
	item, err := queuedInputForText(m.task.Value())
	if err != nil {
		m.notice = notice{text: err.Error(), kind: noticeError}
		return m, nil
	}
	if len(m.queue) >= maxQueuedInputs {
		m.notice = notice{text: fmt.Sprintf("Queue is full (%d instructions). Use /queue, /dequeue, or /clear-queue.", maxQueuedInputs), kind: noticeError}
		return m, nil
	}
	m.queue = append(m.queue, item)
	m.appendChat(chatEntry{author: chatSystem, text: "Queued next " + queuedInputLabel(item) + ": " + compact(item.text, 160)})
	m.task.Reset()
	m.commandIndex = 0
	m.contextClosed = false
	m.normalizeContextSelection()
	m.notice = notice{text: m.queueSummary() + ". It will start after this run completes successfully.", kind: noticeSuccess}
	return m, nil
}

func (m Model) steerCurrentInput() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.task.Value())
	if text == "" {
		m.notice = notice{text: "Write an instruction to steer the active run.", kind: noticeError}
		return m, nil
	}
	if strings.HasPrefix(text, "!") {
		m.notice = notice{text: "Direct shell commands are not supported; configure a verifier with /verify instead.", kind: noticeError}
		return m, nil
	}
	if strings.HasPrefix(text, "/") {
		return m.queueCurrentInput()
	}
	if m.execution == nil {
		m.notice = notice{text: "No active run is available to steer.", kind: noticeError}
		return m, nil
	}
	select {
	case m.execution.steering <- text:
		m.appendChat(chatEntry{author: chatUser, text: "Steer: " + text})
		m.task.Reset()
		m.contextClosed = false
		m.normalizeContextSelection()
		m.notice = notice{text: "Steering instruction accepted; Gator applies it at the next model or tool boundary.", kind: noticeSuccess}
	default:
		m.notice = notice{text: "Steering buffer is full. Wait for Gator to accept the current instruction or press Tab to queue a follow-up.", kind: noticeError}
	}
	return m, nil
}

func (m Model) queueSummary() string {
	if len(m.queue) == 0 {
		return "No prompts queued"
	}
	if len(m.queue) == 1 {
		return "1 prompt queued"
	}
	return fmt.Sprintf("%d prompts queued", len(m.queue))
}

func (m Model) queueStatus() string {
	if len(m.queue) == 0 {
		return "No prompts queued."
	}
	lines := []string{"Queued next turns (local only; cleared when this TUI exits):"}
	for index, item := range m.queue {
		lines = append(lines, fmt.Sprintf("%d. [%s] %s", index+1, queuedInputLabel(item), compact(item.text, 180)))
	}
	return strings.Join(lines, "\n")
}

func queuedInputLabel(item queuedInput) string {
	if item.kind == queuedCommand {
		return "command"
	}
	return "prompt"
}

func queuedCommandContinues(command string) bool {
	switch command {
	case "/clear", "/clear-queue", "/dequeue", "/execute", "/help", "/permissions", "/plan", "/queue", "/status", "/vim", "/worktree":
		return true
	default:
		return false
	}
}

// dispatchNextQueued starts one queued prompt or applies queued local commands
// until an interactive command needs the developer's attention.
func (m Model) dispatchNextQueued() (tea.Model, tea.Cmd, bool) {
	for len(m.queue) > 0 {
		item := m.queue[0]
		m.queue = m.queue[1:]
		m.task.SetValue(item.text)
		m.commandIndex = 0
		m.normalizeCommandSelection()
		if item.kind == queuedPrompt {
			next, command := m.startRun()
			updated := next.(Model)
			if updated.screen != runningScreen && updated.screen != attachmentConfirmScreen {
				updated.queue = append([]queuedInput{item}, updated.queue...)
				updated.notice = notice{text: "Could not start queued instruction: " + updated.notice.text, kind: noticeError}
			}
			return updated, command, true
		}

		next, command := m.executeSelectedCommand()
		updated := next.(Model)
		if !queuedCommandContinues(item.text) {
			return updated, command, true
		}
		m = updated
	}
	return m, nil, false
}
