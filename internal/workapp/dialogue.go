package workapp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

// ConversationInput is the immutable envelope captured by admission. Public
// conversation text is context, never replay, tool results or approval grants.
type ConversationInput struct {
	Version          int                           `json:"version"`
	Source           string                        `json:"source"`
	GoalID           string                        `json:"goal_id,omitempty"`
	RunOptions       RunOptions                    `json:"run_options"`
	Recent           []workrun.ConversationMessage `json:"recent,omitempty"`
	TaskContext      []workrun.ConversationTask    `json:"task_context,omitempty"`
	ScopeDigest      string                        `json:"scope_digest,omitempty"`
	OAuthRefreshPins map[string]string             `json:"oauth_refresh_pins,omitempty"`
}

// DialoguePeer comes from another native owner through the host-only channel.
// Every peer is checked against this conversation's durable task origin.
type DialoguePeer struct {
	ConversationID string `json:"conversation_id"`
	TurnID         string `json:"turn_id"`
	RunID          string `json:"run_id,omitempty"`
	State          string `json:"state"`
	CanGuide       bool   `json:"can_guide"`
	CanStop        bool   `json:"can_stop"`
	CanRevisePlan  bool   `json:"can_revise_plan"`
	Summary        string `json:"summary,omitempty"`
}
type DialogueHost struct {
	CanStart  bool           `json:"can_start"`
	OtherBusy bool           `json:"other_busy"`
	Peers     []DialoguePeer `json:"peers"`
}
type DialogueDispatch struct {
	Sequence    uint64 `json:"sequence"`
	Kind        string `json:"kind"`
	TurnID      string `json:"turn_id"`
	ChildID     string `json:"child_id"`
	TargetRun   string `json:"target_run,omitempty"`
	Instruction string `json:"instruction,omitempty"`
	OwnerTask   string `json:"owner_task,omitempty"`
}
type DialogueDispatchResult struct {
	TurnID        string `json:"turn_id"`
	ChildID       string `json:"child_id"`
	CancelledPlan bool   `json:"cancelled_plan,omitempty"`
	RevisingPlan  bool   `json:"revising_plan,omitempty"`
	Error         string `json:"error,omitempty"`
}
type DialogueControl struct {
	ParentID    string `json:"parent_id"`
	TurnID      string `json:"turn_id"`
	RequestID   string `json:"request_id"`
	RunID       string `json:"run_id"`
	Action      string `json:"action"`
	Instruction string `json:"instruction,omitempty"`
}
type DialogueControlResult struct {
	RequestID     string `json:"request_id"`
	CancelledPlan bool   `json:"cancelled_plan,omitempty"`
	RevisingPlan  bool   `json:"revising_plan,omitempty"`
	Error         string `json:"error,omitempty"`
	control       DialogueControl
}
type conversationRouted struct {
	Conversation string
	TurnID       string
	Request      workrun.ConversationRequest
	Decision     workrun.ConversationDecision
	Err          error
}

func (m Model) conversationAvailable() bool {
	return m.dialogueHost && m.config.RouteConversation != nil && m.retainsRequests() && m.config.ManagedSessions
}
func (m Model) conversationActive() bool {
	if !m.conversationAvailable() {
		return false
	}
	if m.running || len(m.dialogueTurns) > 0 || m.planPreparing != "" || m.dialogueRouting != "" || len(m.dialogueQueue) > 0 || m.dialogueOtherBusy {
		return true
	}
	for _, request := range m.requests.Requests {
		if _, ok := m.localConversationTask(request); ok {
			return true
		}
	}
	for _, peer := range m.dialoguePeers {
		if peer.State == "running" || peer.State == "review" || peer.State == "queued" {
			return true
		}
	}
	return false
}
func (m *Model) restoreDialogue() {
	if m.dialogueCancel != nil {
		m.dialogueCancel()
	}
	m.dialogueTurns, m.dialogueQueue, m.dialoguePeers = nil, nil, nil
	m.dialogueRouting, m.dialogueIssue, m.dialogueDispatch = "", "", nil
	if !m.retainsRequests() {
		return
	}
	turns, more, err := m.config.SessionStore.DialoguePage(m.conversation, 0, 100)
	if err != nil {
		m.dialogueIssue = "Couldn't restore conversation messages: " + err.Error()
		return
	}
	m.dialogueTurns, m.dialogueMore = turns, more
	// Restoring is inspection. In-flight routing or dispatch is unconfirmed;
	// it is never relaunched by a snapshot, focus change or restart.
}
func (m *Model) rememberDialogue(turn worksession.DialogueTurn) {
	for index := range m.dialogueTurns {
		if m.dialogueTurns[index].ID == turn.ID {
			m.dialogueTurns[index] = turn
			return
		}
	}
	m.dialogueTurns = append(m.dialogueTurns, turn)
	if len(m.dialogueTurns) > 100 {
		m.dialogueTurns = m.dialogueTurns[len(m.dialogueTurns)-100:]
		m.dialogueMore = true
	}
}
func (m Model) dialogueByID(id string) (worksession.DialogueTurn, error) {
	return m.config.SessionStore.ReadDialogue(m.conversation, id)
}
func (m *Model) saveDialogue(turn worksession.DialogueTurn) (worksession.DialogueTurn, error) {
	previous := turn.UpdatedAt
	turn.UpdatedAt = time.Now().UTC()
	if turn.UpdatedAt.Before(previous) {
		turn.UpdatedAt = previous
	}
	next, err := m.config.SessionStore.UpdateDialogue(turn, turn.Revision)
	if err == nil {
		m.rememberDialogue(next)
	}
	return next, err
}
func (m Model) conversationRecent() []workrun.ConversationMessage {
	type entry struct {
		at      time.Time
		message workrun.ConversationMessage
	}
	var entries []entry
	for _, message := range m.messages {
		if (message.role == "You" || message.role == "Gator") && message.text != "" {
			role := "assistant"
			if message.role == "You" {
				role = "user"
			}
			entries = append(entries, entry{message.createdAt, workrun.ConversationMessage{Role: role, Text: boundedConversation(message.text, 2048)}})
		}
	}
	for _, turn := range m.dialogueTurns {
		entries = append(entries, entry{turn.CreatedAt, workrun.ConversationMessage{Role: "user", Text: boundedConversation(turn.Text, 2048)}})
		if turn.Response != "" {
			entries = append(entries, entry{turn.UpdatedAt, workrun.ConversationMessage{Role: "assistant", Text: boundedConversation(turn.Response, 2048)}})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].at.Before(entries[j].at) })
	var recent []workrun.ConversationMessage
	for _, entry := range entries {
		recent = append(recent, entry.message)
	}
	if len(recent) > 12 {
		recent = recent[len(recent)-12:]
	}
	return recent
}
func boundedConversation(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	for limit > 0 && text[limit]&0xc0 == 0x80 {
		limit--
	}
	return text[:limit] + "…"
}
func (m Model) conversationTasks() []workrun.ConversationTask {
	var tasks []workrun.ConversationTask
	seen := map[string]bool{}
	// The executing owner comes first, followed by its undispatched reviews.
	for _, request := range m.requests.Requests {
		if request.ID == m.activeRequest {
			if task, ok := m.localConversationTask(request); ok {
				tasks = append(tasks, task)
				seen[request.ID] = true
			}
		}
	}
	for _, request := range m.requests.Requests {
		if !seen[request.ID] && len(tasks) < 16 {
			if task, ok := m.localConversationTask(request); ok {
				tasks = append(tasks, task)
				seen[request.ID] = true
			}
		}
	}
	for _, peer := range m.dialoguePeers {
		if peer.State == "running" || peer.State == "review" || peer.State == "queued" {
			if seen[peer.ConversationID] || len(tasks) >= 16 {
				continue
			}
			turn, err := m.dialogueByID(peer.TurnID)
			if err != nil || turn.TaskChat != peer.ConversationID {
				continue
			}
			tasks = append(tasks, workrun.ConversationTask{ID: peer.ConversationID, Title: boundedConversation(turn.Text, 256), State: peer.State, Summary: boundedConversation(peer.Summary, 2048), CanGuide: peer.CanGuide && peer.RunID != "", CanStop: peer.CanStop && peer.RunID != "", CanRevisePlan: peer.CanRevisePlan && peer.RunID != "", ControlID: peer.RunID})
			seen[peer.ConversationID] = true
		}
	}
	// Keep recent results inspectable by the conversational model as well as
	// the human. Terminal outcomes never inherit a live control capability.
	peers := map[string]DialoguePeer{}
	for _, peer := range m.dialoguePeers {
		peers[peer.ConversationID] = peer
	}
	for index := len(m.dialogueTurns) - 1; index >= 0 && len(tasks) < 16; index-- {
		turn := m.dialogueTurns[index]
		peer, ok := peers[turn.TaskChat]
		if !ok || turn.TaskChat == "" || seen[turn.TaskChat] || peer.TurnID != turn.ID {
			continue
		}
		current, err := m.dialogueByID(turn.ID)
		if err != nil || current.TaskChat != peer.ConversationID {
			continue
		}
		tasks = append(tasks, workrun.ConversationTask{ID: peer.ConversationID, Title: boundedConversation(current.Text, 256), State: peer.State, Summary: boundedConversation(peer.Summary, 2048)})
		seen[peer.ConversationID] = true
	}
	return tasks
}

// ConfigureDialogueHost accepts no workflow state from the renderer. The host
// reports only native owner capabilities; durable provenance filters each peer.
func (m Model) ConfigureDialogueHost(host DialogueHost) Model {
	m.refreshPlanEditMessages()
	m.dialogueHost = true
	m.dialogueCanStart = host.CanStart
	m.dialogueOtherBusy = host.OtherBusy
	m.dialoguePeers = nil
	if !m.retainsRequests() || len(host.Peers) > 32 {
		return m
	}
	seen := map[string]bool{}
	for _, peer := range host.Peers {
		if seen[peer.ConversationID] || !slices.Contains([]string{"running", "review", "queued", "completed", "failed", "stopped", "unconfirmed"}, peer.State) || len(peer.Summary) > 8192 {
			continue
		}
		origin, err := m.config.SessionStore.DialogueTask(peer.ConversationID)
		if err != nil || origin == nil || origin.ParentConversationID != m.conversation || origin.TurnID != peer.TurnID {
			continue
		}
		seen[peer.ConversationID] = true
		m.dialoguePeers = append(m.dialoguePeers, peer)
	}
	// On restart, project only confirmed durable results. Stored running/queued
	// states without a live owner remain unconfirmed and are never relaunched.
	for _, turn := range m.dialogueTurns {
		if turn.TaskChat != "" && !seen[turn.TaskChat] {
			peer := m.retainedDialoguePeer(turn)
			if peer != nil {
				m.dialoguePeers = append(m.dialoguePeers, *peer)
			}
		}
	}
	return m
}

func (m Model) retryDialogueAdmission(request BridgeUploadSubmission, text string) (bool, error) {
	if !m.retainsRequests() {
		return false, nil
	}
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(request.RequestID)))
	turn, err := m.config.SessionStore.ReadDialogue(m.conversation, id)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var input ConversationInput
	if json.Unmarshal(turn.Options, &input) != nil || input.Version != 1 || turn.Text != text || !slices.Equal(input.RunOptions.Uploads, request.Files) || input.RunOptions.ReplyToUpdateID != request.ReplyToUpdateID || input.RunOptions.PlanFirst != request.PlanFirst {
		return true, errors.New("this message identity already belongs to different text or inputs")
	}
	return true, nil
}
func (m Model) admitConversation(request BridgeUploadSubmission, text string) (tea.Model, tea.Cmd) {
	if len(m.dialogueQueue) >= 16 {
		m.bridgeUploadResult.Error = "Gator has several messages to consider. Wait for a response before sending another."
		return m, nil
	}
	release := func() {}
	if m.config.BeforeLocalCommand != nil {
		var err error
		release, err = m.config.BeforeLocalCommand()
		if err != nil {
			m.bridgeUploadResult.Error = err.Error()
			return m, nil
		}
	}
	defer release()
	options := m.submissionOptions()
	options.Uploads = append([]string(nil), request.Files...)
	options.ReplyToUpdateID = request.ReplyToUpdateID
	options.PlanFirst = request.PlanFirst
	input := ConversationInput{Version: 1, Source: m.source, GoalID: m.goalID, RunOptions: retainedRunOptions(options), Recent: m.conversationRecent(), TaskContext: m.conversationTasks()}
	for index := range input.TaskContext {
		input.TaskContext[index].CanGuide, input.TaskContext[index].CanStop = false, false
		input.TaskContext[index].CanRevisePlan = false
	}
	if m.config.CaptureConversationInput != nil {
		var err error
		input, err = m.config.CaptureConversationInput(input)
		if err != nil {
			m.bridgeUploadResult.Error = err.Error()
			return m, nil
		}
	}
	data, err := json.Marshal(input)
	if err != nil {
		m.bridgeUploadResult.Error = err.Error()
		return m, nil
	}
	turn, err := m.config.SessionStore.AdmitDialogue(m.conversation, request.RequestID, text, data, time.Now())
	if err != nil {
		m.bridgeUploadResult.Error = err.Error()
		return m, nil
	}
	m.rememberDialogue(turn)
	m.options.Attachments, m.options.Uploads, m.options.ExcludedKnowledgeIDs = nil, nil, nil
	m.options.RefreshSource = false
	m.options.BrowserSession, m.options.BrowserPolicy = "", ""
	if turn.State == "received" {
		m.dialogueQueue = append(m.dialogueQueue, turn.ID)
	}
	return m.startConversationRoute()
}
func (m Model) startConversationRoute() (tea.Model, tea.Cmd) {
	if m.dialogueRouting != "" || m.dialogueDispatch != nil || len(m.dialogueQueue) == 0 {
		return m, nil
	}
	id := m.dialogueQueue[0]
	m.dialogueQueue = m.dialogueQueue[1:]
	turn, err := m.dialogueByID(id)
	if err != nil {
		m.dialogueIssue = err.Error()
		return m, nil
	}
	if turn.State != "received" {
		return m.startConversationRoute()
	}
	var input ConversationInput
	if json.Unmarshal(turn.Options, &input) != nil || input.Version != 1 || input.Source != m.source {
		return m.failConversation(turn, "The captured conversation scope is unavailable. Send a new message after reviewing this chat.")
	}
	turn.State = "routing"
	turn, err = m.saveDialogue(turn)
	if err != nil {
		m.dialogueIssue = err.Error()
		return m, nil
	}
	m.dialogueRouting = id
	ctx, cancel := context.WithCancel(context.Background())
	m.dialogueCancel = cancel
	request := workrun.ConversationRequest{Text: turn.Text, Recent: input.Recent, Tasks: m.conversationTasks(), HasFiles: len(input.RunOptions.Uploads) > 0 || len(input.RunOptions.Attachments) > 0 || input.RunOptions.ReplyToUpdateID != "", PlanFirst: input.RunOptions.PlanFirst, CanStart: m.dialogueCanStart}
	route, conversation := m.config.RouteConversation, m.conversation
	return m, func() tea.Msg {
		decision, err := route(ctx, request, input)
		return conversationRouted{Conversation: conversation, TurnID: id, Request: request, Decision: decision, Err: err}
	}
}
func (m Model) failConversation(turn worksession.DialogueTurn, issue string) (tea.Model, tea.Cmd) {
	turn.State, turn.Issue = "failed", boundedConversation(issue, 4000)
	turn.Response = ""
	if _, err := m.saveDialogue(turn); err != nil {
		m.dialogueIssue = "Couldn't confirm the message outcome: " + err.Error()
	}
	return m.startConversationRoute()
}
func (m Model) finishConversationRoute(value conversationRouted) (tea.Model, tea.Cmd) {
	if value.Conversation != m.conversation || value.TurnID != m.dialogueRouting {
		return m, nil
	}
	if m.dialogueCancel != nil {
		m.dialogueCancel()
	}
	m.dialogueCancel = nil
	m.dialogueRouting = ""
	turn, err := m.dialogueByID(value.TurnID)
	if err != nil {
		m.dialogueIssue = err.Error()
		return m.startConversationRoute()
	}
	if value.Err != nil {
		return m.failConversation(turn, "Couldn't respond to this message: "+value.Err.Error()+". Your original message is retained; no task was changed.")
	}
	current := value.Request
	current.Tasks = m.conversationTasks()
	current.CanStart = m.dialogueCanStart
	if err := value.Decision.Validate(current); err != nil {
		return m.failConversation(turn, "The task or available controls changed while Gator considered this message. Send a new message after reviewing its current state.")
	}
	var controlID string
	if value.Decision.Kind == "guide" || value.Decision.Kind == "stop" || value.Decision.Kind == "revise_plan" {
		for _, task := range value.Request.Tasks {
			if task.ID == value.Decision.Target {
				controlID = task.ControlID
			}
		}
		matched := false
		for _, task := range current.Tasks {
			if task.ID == value.Decision.Target && controlID != "" && task.ControlID == controlID {
				matched = true
			}
		}
		if !matched {
			return m.failConversation(turn, "That task's operation or reviewed plan changed while Gator considered your message. No task was changed. Review its current state before trying again.")
		}
	}
	decision, _ := json.Marshal(value.Decision)
	turn.Decision = decision
	if value.Decision.Kind == "reply" || value.Decision.Kind == "clarify" {
		turn.State, turn.Response = "answered", value.Decision.Text
		if value.Decision.Kind == "clarify" {
			turn.State = "clarification"
		}
		if _, err := m.saveDialogue(turn); err != nil {
			m.dialogueIssue = err.Error()
		}
		return m.startConversationRoute()
	}
	turn.State = "ready"
	turn, err = m.saveDialogue(turn)
	if err != nil {
		m.dialogueIssue = err.Error()
		return m.startConversationRoute()
	}
	if value.Decision.Kind == "task" {
		bind := m.config.BindConversationTask
		if bind == nil {
			return m.failConversation(turn, "Independent task creation is unavailable. Your original message and files are retained.")
		}
		bound, child, claimed, err := bind(m.conversation, turn.ID, m.source, turn.Revision)
		if err != nil {
			return m.failConversation(turn, "Couldn't create independent work: "+err.Error())
		}
		m.rememberDialogue(bound)
		if claimed {
			m.dialogueSequence++
			m.dialogueDispatch = &DialogueDispatch{Sequence: m.dialogueSequence, Kind: "task", TurnID: turn.ID, ChildID: child.ID}
		}
		return m, nil
	}
	turn.State = "dispatching"
	if value.Decision.Kind == "revise_plan" {
		turn.State, turn.Response = "editing_plan", "Requesting a revised plan for review…"
	}
	turn, err = m.saveDialogue(turn)
	if err != nil {
		m.dialogueIssue = err.Error()
		return m.startConversationRoute()
	}
	if m.ownsConversationTask(value.Decision.Target) {
		if value.Decision.Kind == "revise_plan" {
			// Both current-chat and delegated revisions cross the same host
			// admission boundary before their owner starts the model call.
			m.dialogueSequence++
			m.dialogueDispatch = &DialogueDispatch{Sequence: m.dialogueSequence, Kind: "revise_plan", TurnID: turn.ID, ChildID: m.conversation, OwnerTask: value.Decision.Target, TargetRun: controlID, Instruction: value.Decision.Instruction}
			return m, nil
		}
		cancelledPlan := false
		if value.Decision.Kind == "guide" {
			err = m.operation.Steer(value.Decision.Instruction)
		} else {
			cancelledPlan, err = m.stopConversationTask(value.Decision.Target, controlID)
		}
		if err != nil {
			return m.failConversation(turn, err.Error())
		}
		turn.State, turn.Response = "guided", "I passed that correction to the running task."
		if value.Decision.Kind == "stop" {
			turn.State, turn.Response = "stopped", "I'm stopping that task. Any external action already in progress may still need its receipt checked."
			if cancelledPlan {
				turn.Response = "I cancelled that plan. No implementation was started."
			}
		}
		if _, err := m.saveDialogue(turn); err != nil {
			m.dialogueIssue = "The control was accepted, but its receipt could not be confirmed. Do not replay it automatically: " + err.Error()
		}
		return m.startConversationRoute()
	}
	for _, peer := range m.dialoguePeers {
		if peer.ConversationID == value.Decision.Target {
			m.dialogueSequence++
			m.dialogueDispatch = &DialogueDispatch{Sequence: m.dialogueSequence, Kind: value.Decision.Kind, TurnID: turn.ID, ChildID: peer.ConversationID, TargetRun: controlID, Instruction: value.Decision.Instruction}
			return m, nil
		}
	}
	return m.failConversation(turn, "That task is no longer available. No different task was selected.")
}
func (m Model) FinishDialogueDispatch(result DialogueDispatchResult) (tea.Model, tea.Cmd) {
	request := m.dialogueDispatch
	if request == nil || request.TurnID != result.TurnID || request.ChildID != result.ChildID {
		return m, nil
	}
	m.dialogueDispatch = nil
	turn, err := m.dialogueByID(result.TurnID)
	if err != nil {
		m.dialogueIssue = err.Error()
		return m.startConversationRoute()
	}
	if request.Kind == "revise_plan" && (turn.State == "guided" || turn.State == "failed" || turn.State == "interrupted") {
		// The model may have completed and retained its result before an owner
		// acknowledgement was lost. Prefer that exact durable outcome; never
		// overwrite it with a transport error or repeat the proposal.
		m.rememberDialogue(turn)
		return m.startConversationRoute()
	}
	if result.Error != "" {
		return m.failConversation(turn, "The task request was not confirmed: "+result.Error+". Inspect its retained state before trying again.")
	}
	if request.Kind == "revise_plan" && result.RevisingPlan {
		// Admission is acknowledged immediately; the owner retains the actual
		// proposal outcome in this same turn after the asynchronous model call.
		m.rememberDialogue(turn)
		return m.startConversationRoute()
	}
	switch request.Kind {
	case "task":
		turn.State, turn.Response = "delegated", "I've started that as independent work. You can keep talking here."
	case "guide":
		turn.State, turn.Response = "guided", "I passed that correction to the selected task."
	case "stop":
		turn.State, turn.Response = "stopped", "I'm stopping the selected task. Retained results and external-action receipts remain available."
		if result.CancelledPlan {
			turn.Response = "I cancelled the selected plan. No implementation was started."
		}
	default:
		return m.failConversation(turn, "Unknown task dispatch result.")
	}
	if _, err := m.saveDialogue(turn); err != nil {
		m.dialogueIssue = "The task owner accepted the request, but its receipt was not confirmed. Inspect it before retrying: " + err.Error()
	}
	return m.startConversationRoute()
}
