package worktui

import (
	"crypto/sha256"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type BridgeQueuedRun struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type BridgeQueueAction struct {
	RequestID string `json:"request_id"`
	ID        string `json:"id"`
	Action    string `json:"action"`
	Text      string `json:"text,omitempty"`
}

// Acknowledgements let the desktop retain an edit or guidance draft until Go
// accepts it, including when a queued message starts while the user is editing.
type BridgeQueueResult struct {
	RequestID string `json:"request_id"`
	Error     string `json:"error,omitempty"`
}

func bridgeQueuedRunID(item queuedRun) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d:%s", item.id, item.prompt))))
}

func (m Model) bridgeGuidanceID() string {
	if !m.running || m.cancelling || m.operation == nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(m.operation.RunID)))
}

// ApplyBridgeQueue changes the existing Go queue; the desktop keeps no copy of
// the scheduler. IDs survive reordering and change after edits, so an old click
// cannot change a message that has already started or a newer revision.
func (m Model) ApplyBridgeQueue(action BridgeQueueAction) (tea.Model, tea.Cmd) {
	m.bridgeQueueResult = &BridgeQueueResult{RequestID: action.RequestID}
	fail := func(text string) (tea.Model, tea.Cmd) {
		m.bridgeQueueResult.Error = text
		return m, nil
	}
	if action.RequestID == "" || len(action.RequestID) > 64 || len(action.ID) != 64 {
		return fail("Invalid queue action.")
	}
	if m.models != nil || m.launcher || m.section != "" || m.bridgeExternal != nil || m.pendingBundleAction != nil {
		return fail("Return to the conversation to change queued messages.")
	}
	if action.Action == "guide" {
		if action.ID != m.bridgeGuidanceID() || m.operation == nil {
			return fail("The task has finished or changed. Your message is still in the composer.")
		}
		if err := m.operation.Steer(strings.TrimSpace(action.Text)); err != nil {
			return fail(err.Error())
		}
		m.status = "Steering submitted to current task"
		return m, nil
	}
	index := -1
	for i, item := range m.queue {
		if bridgeQueuedRunID(item) == action.ID {
			index = i
			break
		}
	}
	if index < 0 {
		return fail("This message has already left the queue or changed. Your edits are still here.")
	}
	switch action.Action {
	case "edit":
		text := strings.TrimSpace(action.Text)
		if text == "" || len(text) > 128*1024 {
			return fail("Enter a message of up to 128 KB.")
		}
		m.queueSequence++
		m.queue[index].id, m.queue[index].prompt = m.queueSequence, text
	case "remove":
		m.queue = append(m.queue[:index], m.queue[index+1:]...)
	case "earlier":
		if index > 0 {
			m.queue[index-1], m.queue[index] = m.queue[index], m.queue[index-1]
		}
	case "later":
		if index+1 < len(m.queue) {
			m.queue[index+1], m.queue[index] = m.queue[index], m.queue[index+1]
		}
	case "run":
		if m.running || m.cancelling || m.config.Run == nil {
			return fail("Wait for the current task to stop before sending this message.")
		}
		next := m.queue[index]
		m.queue = append(m.queue[:index], m.queue[index+1:]...)
		m.home = false
		m.messages = append(m.messages, message{role: "You", text: next.prompt})
		m.running = true
		m.status = fmt.Sprintf("Working from an immutable snapshot… · %d queued", len(m.queue))
		next.options.PreviousArtifacts = m.previousArtifactPaths()
		return m.startWork(m.source, m.conversation, next.prompt, next.options)
	default:
		return fail("Unknown queue action.")
	}
	return m, nil
}
