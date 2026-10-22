package workapp

import (
	"crypto/sha256"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type BridgeQueuedRun struct {
	ReplyToUpdateID string   `json:"reply_to_update_id,omitempty"`
	Interrupted     bool     `json:"interrupted,omitempty"`
	Inputs          []string `json:"inputs,omitempty"`
	ID              string   `json:"id"`
	Text            string   `json:"text"`
}

type BridgeQueueAction struct {
	RequestID string `json:"request_id"`
	ID        string `json:"id"`
	Action    string `json:"action"`
	Text      string `json:"text,omitempty"`
	TargetID  string `json:"target_id,omitempty"`
	Placement string `json:"placement,omitempty"`
}

// Acknowledgements let the desktop retain an edit or guidance draft until Go
// accepts it, including when a queued message starts while the user is editing.
type BridgeQueueResult struct {
	RequestID string `json:"request_id"`
	Error     string `json:"error,omitempty"`
}

func bridgeQueuedRunID(item queuedRun) string {
	if item.requestID != "" {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", item.requestID, item.id, item.prompt))))
	}
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
	if action.Action == "move" {
		if len(action.TargetID) != 64 || (action.Placement != "before" && action.Placement != "after") || action.Text != "" {
			return fail("Invalid queue destination.")
		}
	} else if action.TargetID != "" || action.Placement != "" {
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
	priorQueue := m.queue
	m.queue = append([]queuedRun(nil), m.queue...)
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
	case "move":
		target := -1
		for i, item := range m.queue {
			if bridgeQueuedRunID(item) == action.TargetID {
				target = i
				break
			}
		}
		if target < 0 {
			return fail("The queue changed while you were moving this message. Choose its position again.")
		}
		if target == index {
			return m, nil
		}
		// Move the whole captured request atomically in the model's event loop.
		// A drop never changes source/options or moves a different row by index.
		item := m.queue[index]
		m.queue = append(m.queue[:index], m.queue[index+1:]...)
		if index < target {
			target--
		}
		if action.Placement == "after" {
			target++
		}
		m.queue = append(m.queue, queuedRun{})
		copy(m.queue[target+1:], m.queue[target:])
		m.queue[target] = item
	case "run":
		if m.running || m.cancelling || m.config.Run == nil {
			return fail("Wait for the current task to stop before sending this message.")
		}
		return m.runQueued(index)
	default:
		return fail("Unknown queue action.")
	}
	if err := m.persistQueue(m.queue); err != nil {
		m.queue = priorQueue
		return fail(err.Error())
	}
	return m, nil
}
