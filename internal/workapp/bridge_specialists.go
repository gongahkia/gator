package workapp

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
)

// This projection deliberately excludes worker prompts, source captures,
// credentials/configuration, native session state and private filesystem paths.
type BridgeSpecialist struct {
	ID          string `json:"id"`
	Token       string `json:"token,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Assignment  string `json:"assignment,omitempty"`
	Status      string `json:"status"`
	CanGuide    bool   `json:"can_guide"`
	CanStop     bool   `json:"can_stop"`
	parentRun   string
	steerable   bool
}

type BridgeSpecialistAction struct {
	RequestID string `json:"request_id"`
	ID        string `json:"id"`
	Token     string `json:"token"`
	Action    string `json:"action"`
	Text      string `json:"text,omitempty"`
}

type BridgeSpecialistResult struct {
	RequestID string `json:"request_id"`
	Error     string `json:"error,omitempty"`
}

type specialistActionReceipt struct {
	action BridgeSpecialistAction
	result BridgeSpecialistResult
}

var specialistRequestID = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)
var specialistTaskID = regexp.MustCompile(`^subagent-[0-9]{3,}$`)

func (m *Model) recordBridgeSpecialist(event agent.Event) {
	// Parse only the public card fields, never serialize a full orchestrator.Task
	// to the renderer. Events originate in the current operation's supervisor.
	var task struct {
		ID, Role, Description, Assignment, Status string
		ParentRun                                 string `json:"parent_run"`
		Steerable                                 bool
	}
	if json.Unmarshal([]byte(event.Text), &task) != nil || !specialistTaskID.MatchString(task.ID) || task.ParentRun == "" || event.TaskID != task.ParentRun+"/"+task.ID {
		return
	}
	if m.specialists == nil {
		m.specialists = map[string]BridgeSpecialist{}
	}
	m.specialists[task.ID] = BridgeSpecialist{ID: task.ID, Name: task.Role, Description: cardText(task.Description, 1024), Assignment: cardText(task.Assignment, 2048), Status: task.Status, parentRun: task.ParentRun, steerable: task.Steerable}
}

func cardText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit] + "…"
}

func (m Model) specialistToken(task BridgeSpecialist) string {
	if !m.running || m.cancelling || m.operation == nil || task.parentRun != m.operation.RunID {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d:%s", m.conversation, m.operation.RunID, m.loadingRun, task.ID))))
}

func specialistActive(status string) bool {
	return status == "queued" || status == "running" || status == "awaiting_input" || status == "answering"
}

func (m Model) bridgeSpecialists() []BridgeSpecialist {
	if !m.running || m.operation == nil {
		return nil
	}
	var cards []BridgeSpecialist
	for _, task := range m.specialists {
		if task.parentRun != m.operation.RunID {
			continue
		}
		task.Token = m.specialistToken(task)
		task.CanStop = task.Token != "" && specialistActive(task.Status)
		task.CanGuide = task.Token != "" && task.steerable && (task.Status == "queued" || task.Status == "running") && m.bridgeGuidanceID() != ""
		cards = append(cards, task)
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].ID < cards[j].ID })
	return cards
}

// ApplyBridgeSpecialist binds each action to the active chat/run and worker.
// The supervisor rechecks liveness and steering support under its own lock.
func (m Model) ApplyBridgeSpecialist(action BridgeSpecialistAction) (tea.Model, tea.Cmd) {
	m.bridgeSpecialistResult = &BridgeSpecialistResult{RequestID: action.RequestID}
	fail := func(message string) (tea.Model, tea.Cmd) {
		m.bridgeSpecialistResult.Error = message
		return m, nil
	}
	if !specialistRequestID.MatchString(action.RequestID) || !specialistTaskID.MatchString(action.ID) || len(action.Token) != 64 || (action.Action != "guide" && action.Action != "stop") || (action.Action == "stop" && action.Text != "") || (action.Action == "guide" && (strings.TrimSpace(action.Text) == "" || len(action.Text) > 8192 || !utf8.ValidString(action.Text) || strings.ContainsRune(action.Text, 0))) {
		return fail("Invalid specialist action.")
	}
	task, ok := m.specialists[action.ID]
	if !ok || action.Token != m.specialistToken(task) || m.models != nil || m.launcher || m.section != "" || m.bridgeExternal != nil || m.pendingBundleAction != nil {
		return fail("This specialist is no longer available in the current conversation.")
	}
	if receipt, exists := m.specialistActions[action.RequestID]; exists {
		if receipt.action != action {
			return fail("This request was already used for a different action.")
		}
		result := receipt.result
		m.bridgeSpecialistResult = &result
		return m, nil
	}
	if len(m.specialistActions) >= 256 {
		return fail("The control limit for this run has been reached.")
	}
	var err error
	if action.Action == "stop" {
		err = m.operation.CancelTask(action.ID)
	} else {
		if m.bridgeGuidanceID() == "" {
			return fail("Stop and review a new plan before guiding this specialist.")
		}
		err = m.operation.SteerTask(action.ID, strings.TrimSpace(action.Text))
	}
	if err != nil {
		m.bridgeSpecialistResult.Error = err.Error()
	}
	if m.specialistActions == nil {
		m.specialistActions = map[string]specialistActionReceipt{}
	}
	m.specialistActions[action.RequestID] = specialistActionReceipt{action, *m.bridgeSpecialistResult}
	return m, nil
}
