package workapp

import (
	"encoding/json"
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/worksession"
)

// DialogueMessage projects public content and input references, never captured
// execution options or another owner's approval token.
type DialogueMessage struct {
	ID              string    `json:"id"`
	Sequence        uint64    `json:"sequence"`
	Text            string    `json:"text"`
	Response        string    `json:"response,omitempty"`
	State           string    `json:"state"`
	Issue           string    `json:"issue,omitempty"`
	TaskChat        string    `json:"task_chat,omitempty"`
	InputIDs        []string  `json:"input_ids,omitempty"`
	ReplyToUpdateID string    `json:"reply_to_update_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
type DialogueTaskState struct {
	DialoguePeer
	ParentID string `json:"parent_id"`
}

func (m Model) dialogueMessages() []DialogueMessage {
	var messages []DialogueMessage
	for _, turn := range m.dialogueTurns {
		var input ConversationInput
		if json.Unmarshal(turn.Options, &input) != nil || input.Version != 1 {
			continue
		}
		messages = append(messages, DialogueMessage{ID: turn.ID, Sequence: turn.Sequence, Text: turn.Text, Response: turn.Response, State: turn.State, Issue: turn.Issue, TaskChat: turn.TaskChat, InputIDs: append([]string(nil), input.RunOptions.Uploads...), ReplyToUpdateID: input.RunOptions.ReplyToUpdateID, CreatedAt: turn.CreatedAt, UpdatedAt: turn.UpdatedAt})
	}
	return messages
}

func (m Model) dialogueTaskState() *DialogueTaskState {
	if !m.retainsRequests() {
		return nil
	}
	origin, err := m.config.SessionStore.DialogueTask(m.conversation)
	if err != nil || origin == nil {
		return nil
	}
	result := &DialogueTaskState{ParentID: origin.ParentConversationID, DialoguePeer: DialoguePeer{ConversationID: m.conversation, TurnID: origin.TurnID, State: "unconfirmed"}}
	for _, r := range m.requests.Requests {
		if r.ClientID != origin.TurnID {
			continue
		}
		var options RunOptions
		if json.Unmarshal(r.Options, &options) != nil || options.DialogueParent != origin.ParentConversationID || options.DialogueTurn != origin.TurnID {
			return result
		}
		result.State = r.State
		if r.State == "discarded" {
			result.State = "stopped"
			if r.Plan != nil {
				result.Summary = "Plan cancelled before implementation."
			}
		}
		if r.Plan != nil && r.State == "queued" {
			result.State = "review"
		}
		if task, ok := m.localConversationTask(r); ok {
			result.RunID, result.CanGuide, result.CanStop, result.CanRevisePlan = task.ControlID, task.CanGuide, task.CanStop, task.CanRevisePlan
			result.Summary = task.Summary
			result.PreparationControlID = task.PreparationControlID
		}
		if r.ID == m.activeRequest && m.running {
			result.State = "running"
			if m.interaction != nil {
				result.State = "review"
			}
		}
		if (r.ID == m.planPreparing || r.ID == m.planRevising) && m.running {
			result.State = "running"
		}
		if !r.Pending() {
			if r.RevisionID != "" {
				if revision, err := m.config.SessionStore.LoadRevision(m.conversation, r.RevisionID); err == nil {
					result.Summary = boundedConversation(revision.FinalText, 8192)
				}
			}
			for i := len(m.messages) - 1; result.Summary == "" && i >= 0; i-- {
				message := m.messages[i]
				if message.role == "Gator" && message.text != "" && (r.RevisionID != "" && message.revisionID == r.RevisionID || message.requestID == r.ID) {
					result.Summary = boundedConversation(message.text, 8192)
					break
				}
			}
			if result.Summary == "" {
				result.Summary = boundedConversation(r.Issue, 8192)
			}
		}
		break
	}
	return result
}

func (m Model) retainedDialoguePeer(turn worksession.DialogueTurn) *DialoguePeer {
	origin, err := m.config.SessionStore.DialogueTask(turn.TaskChat)
	if err != nil || origin == nil || origin.ParentConversationID != m.conversation || origin.TurnID != turn.ID {
		return nil
	}
	peer := &DialoguePeer{ConversationID: turn.TaskChat, TurnID: turn.ID, State: "unconfirmed"}
	journal, err := m.config.SessionStore.LoadRequests(turn.TaskChat)
	if err != nil {
		return peer
	}
	for _, r := range journal.Requests {
		if r.ClientID == turn.ID {
			if !r.Pending() {
				peer.State = r.State
				if r.State == "discarded" {
					peer.State = "stopped"
				}
				peer.Summary = boundedConversation(r.Issue, 8192)
				if peer.Summary == "" && r.State == "discarded" && r.Plan != nil {
					peer.Summary = "Plan cancelled before implementation."
				}
				if r.RevisionID != "" {
					if revision, err := m.config.SessionStore.LoadRevision(turn.TaskChat, r.RevisionID); err == nil {
						peer.Summary = boundedConversation(revision.FinalText, 8192)
					}
				}
			}
			break
		}
	}
	return peer
}

// BootstrapDialogue only admits the immutable request bound to this exact
// empty child. A retry reports the retained admission without starting it again.
func (m Model) BootstrapDialogue(parent, turnID string) (tea.Model, tea.Cmd) {
	m.dialogueBootstrapResult = &DialogueDispatchResult{TurnID: turnID, ChildID: m.conversation}
	fail := func(err error) (tea.Model, tea.Cmd) { m.dialogueBootstrapResult.Error = err.Error(); return m, nil }
	if !m.config.ManagedSessions || !m.background || !m.retainsRequests() || m.config.Run == nil {
		return fail(errors.New("independent task ownership is unavailable"))
	}
	release := func() {}
	if m.config.BeforeLocalCommand != nil {
		var err error
		release, err = m.config.BeforeLocalCommand()
		if err != nil {
			return fail(err)
		}
	}
	defer release()
	origin, err := m.config.SessionStore.DialogueTask(m.conversation)
	if err != nil {
		return fail(err)
	}
	if origin == nil || origin.ParentConversationID != parent || origin.TurnID != turnID {
		return fail(errors.New("this chat is not the retained task owner"))
	}
	turn, err := m.config.SessionStore.ReadDialogue(parent, turnID)
	if err != nil {
		return fail(err)
	}
	var input ConversationInput
	if json.Unmarshal(turn.Options, &input) != nil || input.Version != 1 || input.Source != m.source || turn.TaskChat != m.conversation {
		return fail(errors.New("the retained task scope is unavailable"))
	}
	for _, r := range m.requests.Requests {
		if r.ClientID == turnID {
			var options RunOptions
			if json.Unmarshal(r.Options, &options) != nil || options.DialogueParent != parent || options.DialogueTurn != turnID || r.Text != turn.Text {
				return fail(errors.New("the retained task admission does not match its origin"))
			}
			return m, nil
		}
	}
	if turn.State != "dispatching" || m.running || len(m.requests.Requests) != 0 || m.revision != "" || m.requestsIssue != "" {
		return fail(errors.New("this task cannot be launched again; inspect its retained state"))
	}
	if err := m.validateDialogueInputs(input); err != nil {
		return fail(err)
	}
	options := retainedRunOptions(input.RunOptions)
	options.DialogueParent, options.DialogueTurn = parent, turnID
	m.submissionID = turnID
	m.messages = append(m.messages, message{role: "You", text: turn.Text, createdAt: time.Now().UTC(), replyToUpdateID: options.ReplyToUpdateID})
	m.running, m.home = true, false
	m.status = "Working on an independent request…"
	next, cmd := m.startWork(m.source, m.conversation, turn.Text, options)
	m = next.(Model)
	m.submissionID = ""
	found := false
	for _, r := range m.requests.Requests {
		if r.ClientID == turnID {
			found = true
			break
		}
	}
	if !found {
		return fail(errors.New("the task owner could not persist admission: " + m.bridgeNotice))
	}
	return m, cmd
}

func (m Model) validateDialogueInputs(input ConversationInput) error {
	if len(input.RunOptions.Uploads) > 0 || len(input.RunOptions.Attachments) > 0 {
		if m.config.ValidateUploads == nil {
			return errors.New("file validation is unavailable")
		}
		if err := m.config.ValidateUploads(input.Source, input.RunOptions.Attachments, input.RunOptions.Uploads); err != nil {
			return err
		}
	}
	return nil
}

// ControlDialogue checks provenance and the live operation in its own owner.
// It cannot grant approval or change an exact external-action review. A pending
// plan correction revokes the old plan review before preparing new content.
func (m Model) ControlDialogue(control DialogueControl) (Model, tea.Cmd) {
	result := DialogueControlResult{RequestID: control.RequestID, control: control}
	if m.dialogueControls == nil {
		m.dialogueControls = map[string]DialogueControlResult{}
	}
	if prior, ok := m.dialogueControls[control.RequestID]; ok {
		if prior.control != control {
			m.dialogueControlResult = &DialogueControlResult{RequestID: control.RequestID, Error: "this control identity already belongs to a different action"}
			return m, nil
		}
		m.dialogueControlResult = &prior
		return m, nil
	}
	fail := func(text string) (Model, tea.Cmd) {
		result.Error = text
		m.dialogueControlResult = &result
		return m, nil
	}
	if control.RequestID == "" || len(control.RequestID) > 96 || len(m.dialogueControls) >= 128 {
		return fail("this task-control session is unavailable; inspect the original task")
	}
	state := m.dialogueTaskState()
	ownerRequest := ""
	if control.Action == "revise_plan" && control.ParentID == m.conversation {
		state = nil
		for _, r := range m.requests.Requests {
			if r.ID == control.TurnID {
				if task, ok := m.localConversationTask(r); ok {
					state = &DialogueTaskState{ParentID: m.conversation, DialoguePeer: DialoguePeer{ConversationID: m.conversation, TurnID: r.ID, RunID: task.ControlID, CanRevisePlan: task.CanRevisePlan, PreparationControlID: task.PreparationControlID}}
					ownerRequest = r.ID
				}
				break
			}
		}
	} else if state != nil {
		for _, r := range m.requests.Requests {
			if r.ClientID == state.TurnID {
				ownerRequest = r.ID
				break
			}
		}
	}
	if !m.config.ManagedSessions || state == nil || state.ParentID != control.ParentID || state.TurnID != control.TurnID || state.RunID == "" || state.RunID != control.RunID && !(control.Action == "revise_plan" && control.RunID != "" && state.PreparationControlID == control.RunID) {
		return fail("the selected task or operation changed; no different task was controlled")
	}
	var command tea.Cmd
	switch control.Action {
	case "revise_plan":
		if !state.CanRevisePlan {
			return fail("the selected plan cannot be revised in its current state")
		}
		if ownerRequest == "" {
			return fail("the original plan admission is unavailable")
		}
		var err error
		m, command, err = m.startTaskPlanRevision(ownerRequest, control.RunID, control.ParentID, control.RequestID, control.Instruction)
		if err != nil {
			return fail(err.Error())
		}
		result.RevisingPlan = true
	case "guide":
		if !state.CanGuide {
			return fail("this task cannot accept corrections during its current review or plan")
		}
		if err := m.operation.Steer(control.Instruction); err != nil {
			return fail(err.Error())
		}
	case "stop":
		if !state.CanStop || control.Instruction != "" {
			return fail("this task cannot be stopped by this control")
		}
		found := false
		for _, r := range m.requests.Requests {
			if r.ClientID == state.TurnID {
				var err error
				result.CancelledPlan, err = m.stopConversationTask(r.ID, control.RunID)
				if err != nil {
					return fail(err.Error())
				}
				found = true
				break
			}
		}
		if !found {
			return fail("the original task admission is unavailable")
		}
	default:
		return fail("unknown task control")
	}
	m.dialogueControls[control.RequestID] = result
	m.dialogueControlResult = &result
	return m, command
}

// LoadOlderDialogue extends inspection only; it cannot revive in-flight turns.
func (m Model) LoadOlderDialogue(before uint64) Model {
	if !m.retainsRequests() || len(m.dialogueTurns) == 0 || before != m.dialogueTurns[0].Sequence {
		return m
	}
	turns, more, err := m.config.SessionStore.DialoguePage(m.conversation, before, 100)
	if err != nil {
		m.dialogueIssue = err.Error()
		return m
	}
	m.dialogueTurns = append(turns, m.dialogueTurns...)
	m.dialogueMore = more
	return m
}
