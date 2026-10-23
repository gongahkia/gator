package workapp

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	appconfig "github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workactivity"
	"github.com/gongahkia/gator/internal/workhome"
	"github.com/gongahkia/gator/internal/workrun"
)

// BridgeState is a display-only projection for the desktop.
// The Go model remains the sole owner of commands, runs, approvals and history.
type BridgeState struct {
	ActionRecoveries        []RecoveredAction `json:"action_recoveries,omitempty"`
	ActionRecoveryIssue     string            `json:"action_recovery_issue,omitempty"`
	ActionRecoveryTruncated bool              `json:"action_recovery_truncated,omitempty"`

	ReceiptBusy           bool                       `json:"receipt_busy,omitempty"`
	ReceiptResult         *ReceiptResult             `json:"receipt_result,omitempty"`
	ChatMemories          []ChatMemoryReceipt        `json:"chat_memories,omitempty"`
	ChatMemoryIssue       string                     `json:"chat_memory_issue,omitempty"`
	ChatMemoryResult      *ChatMemoryResult          `json:"chat_memory_result,omitempty"`
	PendingConnection     string                     `json:"pending_connection,omitempty"`
	UpdatesHaveOlder      bool                       `json:"updates_have_older,omitempty"`
	UpdatesHaveNewer      bool                       `json:"updates_have_newer,omitempty"`
	UpdateConversations   []BridgeUpdateConversation `json:"update_conversations,omitempty"`
	ChatUpdateResult      *BridgeUpdateResult        `json:"chat_update_result,omitempty"`
	ChatUpdates           []BridgeChatUpdate         `json:"chat_updates,omitempty"`
	ChatUpdatesIssue      string                     `json:"chat_updates_issue,omitempty"`
	ChatUpdatesTotal      int                        `json:"chat_updates_total,omitempty"`
	ChatChecks            []BridgeChatCheck          `json:"chat_checks,omitempty"`
	ChatChecksIssue       string                     `json:"chat_checks_issue,omitempty"`
	Foreground            bool                       `json:"foreground"`
	SessionRequest        *BridgeSessionRequest      `json:"session_request,omitempty"`
	RequestsRecovered     bool                       `json:"requests_recovered,omitempty"`
	RequestsIssue         string                     `json:"requests_issue,omitempty"`
	Composer              ComposerSettings           `json:"composer"`
	ComposerResult        *ComposerResult            `json:"composer_result,omitempty"`
	Desktop               appconfig.DesktopSettings  `json:"desktop"`
	UploadResult          *BridgeUploadResult        `json:"upload_result,omitempty"`
	Mode                  string                     `json:"mode"`
	Panel                 *uisnapshot.Panel          `json:"panel,omitempty"`
	Activity              *workactivity.Trace        `json:"activity,omitempty"`
	Messages              []BridgeMessage            `json:"messages"`
	Commands              []BridgeCommand            `json:"commands"`
	Actions               []workhome.Action          `json:"actions"`
	Title                 string                     `json:"title"`
	Source                string                     `json:"source"`
	ConversationID        string                     `json:"conversation_id,omitempty"`
	Primary               bool                       `json:"primary"`
	PrimaryConversationID string                     `json:"primary_conversation_id,omitempty"`
	RecentConversations   []BridgeConversation       `json:"recent_conversations,omitempty"`
	RecentID              string                     `json:"recent_id,omitempty"`
	RecentIssue           string                     `json:"recent_issue,omitempty"`
	GoalID                string                     `json:"goal_id,omitempty"`
	WorkDetails           BridgeWorkDetails          `json:"work_details"`
	Provider              string                     `json:"provider,omitempty"`
	Model                 string                     `json:"model,omitempty"`
	Theme                 string                     `json:"theme"`
	Status                string                     `json:"status,omitempty"`
	StatusLine            []string                   `json:"status_line"`
	StatusLineEnabled     bool                       `json:"status_line_enabled"`
	Input                 string                     `json:"input,omitempty"`
	Running               bool                       `json:"running"`
	Cancelling            bool                       `json:"cancelling"`
	Elapsed               string                     `json:"elapsed,omitempty"`
	LoadingFrame          int                        `json:"loading_frame"`
	Queued                int                        `json:"queued"`
	Queue                 []BridgeQueuedRun          `json:"queue"`
	QueueResult           *BridgeQueueResult         `json:"queue_result,omitempty"`
	GuidanceID            string                     `json:"guidance_id,omitempty"`
	Home                  bool                       `json:"home"`
	FirstRun              bool                       `json:"first_run"`
	SupervisorNeedsStart  bool                       `json:"supervisor_needs_start"`
	Checks                []BridgeCheck              `json:"checks,omitempty"`
	ChecksIssue           string                     `json:"checks_issue,omitempty"`
	FollowingGoals        []BridgeFollowingGoal      `json:"following_goals,omitempty"`
	FollowingIssue        string                     `json:"following_issue,omitempty"`
	Overview              *workhome.Overview         `json:"overview,omitempty"`
	OverviewIssue         string                     `json:"overview_issue,omitempty"`
	External              *BridgeExternal            `json:"external,omitempty"`
	PendingApproval       *BridgeApproval            `json:"pending_approval,omitempty"`
	Notice                string                     `json:"notice,omitempty"`
	Destination           string                     `json:"destination,omitempty"`
}

type BridgeApproval struct {
	ID           int    `json:"id"`
	Token        string `json:"token"`
	MessageIndex int    `json:"message_index"`
}

// BridgeExternal asks the frontend to give the terminal to a process. The
// command is constructed by the canonical Go application paths.
type BridgeExternal struct {
	Kind    string   `json:"kind"`
	Program string   `json:"program"`
	Args    []string `json:"args"`
	Dir     string   `json:"dir,omitempty"`
	Env     []string `json:"env,omitempty"`
	Action  string   `json:"-"`
	ID      string   `json:"-"`
}

func bridgeExternal(kind, action, id string, command *exec.Cmd) *BridgeExternal {
	var environment []string
	for _, value := range command.Env {
		if strings.HasPrefix(value, "GATOR_STATE_DIR=") || strings.HasPrefix(value, "GATOR_CONFIG_DIR=") {
			environment = append(environment, value)
		}
	}
	return &BridgeExternal{Kind: kind, Program: command.Path, Args: append([]string(nil), command.Args[1:]...),
		Dir: command.Dir, Env: environment, Action: action, ID: id}
}

type BridgeMessage struct {
	RunID           string              `json:"run_id,omitempty"`
	CreatedAt       time.Time           `json:"created_at,omitempty"`
	ReplyToUpdateID string              `json:"reply_to_update_id,omitempty"`
	InputIDs        []string            `json:"input_ids,omitempty"`
	Inputs          []string            `json:"inputs,omitempty"`
	Activity        *workactivity.Trace `json:"activity,omitempty"`
	Role            string              `json:"role"`
	Text            string              `json:"text"`
	Bundle          *BundleSummary      `json:"bundle,omitempty"`
	Result          *uisnapshot.Result  `json:"result,omitempty"`
}

type BridgeCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Advanced    bool   `json:"advanced"`
	NeedsInput  bool   `json:"needs_input"`
}
type BridgeCheck struct {
	GoalID string `json:"goal_id,omitempty"`
	Title  string `json:"title"`
	Health string `json:"health"`
	State  string `json:"state"`
	Tone   string `json:"tone"`
}

type BridgeFollowingGoal struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// BridgeWorkDetails contains selections for new requests and the latest
// retained result, including a failed run. It does not measure context usage
// or assert that a selected connector is authenticated or authorized.
type BridgeWorkDetails struct {
	Mode         string         `json:"mode"`
	SnapshotID   string         `json:"snapshot_id,omitempty"`
	Attachments  []string       `json:"attachments"`
	ConnectorIDs []string       `json:"connector_ids"`
	LatestBundle *BundleSummary `json:"latest_bundle,omitempty"`
}

func (m Model) BridgeState() BridgeState {
	desktop := m.desktop
	desktop.Shortcuts = m.desktop.ResolvedShortcuts()
	state := BridgeState{
		ReceiptBusy: m.receiptBusy, ReceiptResult: m.receiptResult,
		ActionRecoveries: m.actionRecoveries, ActionRecoveryIssue: m.actionRecoveryIssue, ActionRecoveryTruncated: m.actionRecoveryTruncated,
		ChatMemories: append([]ChatMemoryReceipt(nil), m.chatMemories...), ChatMemoryIssue: m.chatMemoryIssue, ChatMemoryResult: m.chatMemoryResult,
		PendingConnection: m.pendingConnection(),
		UpdatesHaveOlder:  m.updatesHaveOlder, UpdatesHaveNewer: len(m.updatePageAnchors) > 0,
		UpdateConversations: append([]BridgeUpdateConversation(nil), m.updateConversations...),
		ChatUpdateResult:    m.chatUpdateResult, ChatUpdates: m.bridgeChatUpdates(), ChatUpdatesIssue: m.chatUpdatesIssue, ChatUpdatesTotal: m.chatUpdatesTotal,
		Foreground:            !m.background,
		SessionRequest:        m.sessionRequest,
		RequestsRecovered:     m.requestsRecovered,
		RequestsIssue:         m.requestsIssue,
		Desktop:               desktop,
		UploadResult:          m.bridgeUploadResult,
		Mode:                  "chat",
		Activity:              m.activity.Clone(),
		Messages:              make([]BridgeMessage, 0, len(m.messages)),
		Commands:              make([]BridgeCommand, 0),
		Actions:               m.config.Attention.Next(),
		Title:                 m.title,
		Source:                m.source,
		ConversationID:        m.conversation,
		Primary:               m.primary,
		PrimaryConversationID: m.primaryID,
		GoalID:                m.goalID,
		Provider:              m.modelStatus.Provider,
		Composer:              m.composerSettings(),
		ComposerResult:        m.composerResult,
		Model:                 m.modelStatus.Model,
		Theme:                 m.theme,
		Status:                m.status,
		StatusLine:            m.statusLineItems(),
		StatusLineEnabled:     m.statusLineEnabled,
		Input:                 m.input,
		Running:               m.running,
		Cancelling:            m.cancelling,
		Elapsed:               m.workElapsed(),
		LoadingFrame:          m.loadingFrame,
		Queued:                len(m.queue),
		Queue:                 make([]BridgeQueuedRun, 0, len(m.queue)),
		QueueResult:           m.bridgeQueueResult,
		Home:                  m.home,
		FirstRun:              m.firstRun,
		External:              m.bridgeExternal,
		Notice:                m.bridgeNotice,
	}
	for _, item := range m.queue {
		state.Queue = append(state.Queue, BridgeQueuedRun{ReplyToUpdateID: item.options.ReplyToUpdateID, Interrupted: item.interrupted, Inputs: uploadNames(item.options.Uploads), ID: bridgeQueuedRunID(item), Text: item.prompt})
	}
	state.GuidanceID = m.bridgeGuidanceID()
	state.WorkDetails = BridgeWorkDetails{
		Mode: m.options.Mode, SnapshotID: m.snapshot,
		Attachments:  append([]string{}, m.options.Attachments...),
		ConnectorIDs: append([]string{}, m.options.ConnectorIDs...),
	}
	if m.lastBundle.Path != "" {
		bundle := cloneBundleSummary(m.lastBundle)
		state.WorkDetails.LatestBundle = &bundle
	}
	state.RecentConversations, state.RecentID = m.bridgeRecentConversations()
	state.RecentIssue = m.bridgeRecentIssue
	state.FollowingIssue = m.bridgeFollowingIssue
	state.ChecksIssue = m.checksIssue
	if m.bridgeFollowingIssue == "" {
		for _, record := range m.config.Goals {
			if record.Status == "active" || record.Status == "paused" {
				state.FollowingGoals = append(state.FollowingGoals, BridgeFollowingGoal{ID: record.ID, Title: singleLine(record.Title), Status: string(record.Status)})
			}
		}
	}
	if !m.overview.At.IsZero() {
		copy := m.overview
		state.Overview = &copy
	}
	state.OverviewIssue = m.overviewIssue
	for _, check := range m.checks {
		state.Checks = append(state.Checks, BridgeCheck{GoalID: check.GoalID, Title: singleLine(check.Title), Health: check.HealthLine(), State: check.State, Tone: checkTone(check.State)})
		if m.conversation != "" && check.Definition.OriginConversationID == m.conversation {
			state.ChatChecks = append(state.ChatChecks, BridgeChatCheck{ID: check.ID, Title: singleLine(check.Title), State: check.State, Issue: check.Issue, NextAction: check.NextAction, LastSuccess: bridgeCheckTime(check.LastSuccess), NextDue: bridgeCheckTime(check.NextDue), GapFrom: bridgeCheckTime(check.CoverageGap.From), GapTo: bridgeCheckTime(check.CoverageGap.To), Unsuccessful: check.Unsuccessful})
		}
	}
	if m.checksIssue != "" {
		owned := len(state.ChatChecks) > 0
		if !owned && m.conversation != "" {
			for _, job := range m.config.Jobs {
				if job.OriginConversationID == m.conversation {
					owned = true
					break
				}
			}
		}
		if owned {
			state.ChatChecksIssue = "Check health could not refresh. Displayed states may be out of date."
		}
	}
	if !m.config.SupervisorRunning {
		for _, job := range m.config.Jobs {
			if job.Enabled {
				state.SupervisorNeedsStart = true
				break
			}
		}
	}
	for _, item := range m.messages {
		state.Messages = append(state.Messages, BridgeMessage{RunID: item.runID, CreatedAt: item.createdAt, ReplyToUpdateID: item.replyToUpdateID, InputIDs: append([]string(nil), item.inputIDs...), Inputs: append([]string(nil), item.inputs...), Activity: item.activity.Clone(), Role: item.role, Text: item.text, Bundle: cloneBundlePointer(item.bundle), Result: item.result})
	}
	for _, item := range m.availableCommandEntries() {
		state.Commands = append(state.Commands, BridgeCommand{
			Name: strings.TrimPrefix(item.command, "/"), Description: item.subtitle,
			Advanced: item.advanced, NeedsInput: item.kind == "command-input",
		})
	}
	if panel := m.bridgePanel(); panel != nil {
		panel.ID = bridgePanelID(*panel)
		state.Mode = "panel"
		state.Panel = panel
		destination := m.section
		if m.models != nil {
			destination = "model"
		} else if m.launcher {
			destination = m.launcherMode
		} else if m.section == "controls" {
			destination = m.controlFamily
		} else if m.section == "context" && m.contextWorkID == "" {
			destination = "knowledge"
		}
		if destination != "" {
			state.Destination = "/" + destination
		}
	}
	if m.interaction != nil {
		index := -1
		for i := len(m.messages) - 1; i >= 0; i-- {
			result := m.messages[i].result
			if result != nil && result.Kind == "approval" && len(result.Fields) > 0 && result.Fields[0].Value == fmt.Sprint(m.interaction.ID) {
				index = i
				break
			}
		}
		state.PendingApproval = &BridgeApproval{ID: m.interaction.ID, Token: m.bridgeApprovalToken(), MessageIndex: index}
	}
	return state
}

func bridgePanelID(panel uisnapshot.Panel) string {
	panel.ID = ""
	data, _ := json.Marshal(panel)
	if panel.ActionIdentity != "" {
		data = append(data, []byte("\x00"+panel.ActionIdentity)...)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func (m Model) bridgeApprovalToken() string {
	if m.interaction == nil {
		return ""
	}
	digest, _ := workrun.InteractionDigest(*m.interaction)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", m.bridgeGuidanceID(), m.loadingRun, digest))))
}

// RespondBridgeApproval binds the decision to its run and exact preview, even
// when a later run reuses the same numeric interaction ID.
func (m Model) RespondBridgeApproval(id int, token string, approved bool) (tea.Model, tea.Cmd) {
	m.bridgeNotice = ""
	if m.interaction == nil || m.interaction.ID != id || token == "" || token != m.bridgeApprovalToken() {
		m.status = "This approval is no longer pending."
		m.bridgeNotice = m.status
		return m, nil
	}
	command := "/deny"
	if approved {
		command = "/approve"
	}
	m.input = command
	return m.submitRunningInput()
}

// NavigateBridge switches desktop destinations without treating a navigation
// label as a user prompt. The caller confirms abandoning an unsaved form.
func (m Model) NavigateBridge(command string) (tea.Model, tea.Cmd) {
	m.bridgeNotice = ""
	switch command {
	case "/gator", "/chat", "/knowledge", "/new", "/overview", "/goals", "/conversations", "/inbox", "/followup", "/history", "/memory", "/context", "/usage", "/data", "/recovery", "/connections", "/model", "/settings", "/help":
	default:
		known := false
		for _, entry := range m.availableCommandEntries() {
			known = known || (entry.command == command && entry.kind == "command")
		}
		if !known {
			return m, nil
		}
	}
	if m.config.ManagedSessions && (m.running && command != "/chat" || m.pendingConnection() != "" && command != "/connections") && !m.controlBusy && !m.followupSaving && !m.recallBusy && m.bridgeExternal == nil {
		if command == "/new" {
			m.requestSession(BridgeSessionRequest{New: true})
		} else {
			m.requestSession(BridgeSessionRequest{View: command})
		}
		return m, nil
	}
	if (m.running && command != "/chat") || m.controlBusy || m.followupSaving || m.recallBusy || m.bridgeExternal != nil {
		m.status = "Finish or stop the current operation before switching views."
		m.bridgeNotice = m.status
		return m, nil
	}
	if m.models != nil {
		m.closeModelPanel()
	}
	m.cancelControlBackground()
	m.controlForm, m.followupForm, m.learningForm = nil, nil, nil
	m.pendingBundleAction = nil
	m.section, m.launcher = "", false
	m.contextEditing, m.contextConfirm = false, false
	m.recallConfirm = ""
	if command == "/chat" {
		return m, nil
	}
	if command == "/gator" {
		return m.openPrimaryConversation()
	}
	if command == "/conversations" {
		m.openConversationPicker()
		return m, nil
	}
	if command == "/knowledge" {
		return m.openContext("", true)
	}
	if command == "/new" {
		// Desktop has no shell cwd to return to; keep its selected workspace.
		m.config.CurrentFolder = m.source
	}
	return m.runLocalCommand(command)
}

// SetBridgeInput synchronizes the React composer before an editor handoff.
func (m Model) SetBridgeInput(text string) Model {
	m.input = text
	return m
}

// FinishBridgeExternal returns to the same model after the frontend restores
// the terminal. The callback paths use canonical application services.
func (m Model) FinishBridgeExternal(failure string) (tea.Model, tea.Cmd) {
	job := m.bridgeExternal
	if job == nil {
		return m, nil
	}
	m.bridgeExternal = nil
	var err error
	if failure != "" {
		err = errors.New(failure)
	}
	switch job.Kind {
	case "editor":
		return m.updateComposerEditorDone(composerEditorDone{path: m.composerEditorPath, editor: composerEditor(), err: err})
	case "connector":
		return m.Update(connectorActionDone{action: job.Action, id: job.ID, err: err})
	case "control":
		return m.Update(controlExternalDone{Err: err})
	default:
		m.status = fmt.Sprintf("Unknown external action %q", job.Kind)
		return m, nil
	}
}

// SubmitText routes a complete frontend request through the canonical Work application
// command and run paths as pressing Enter in its native composer.
func (m Model) SubmitText(text string) (tea.Model, tea.Cmd) {
	if m.models != nil || m.launcher || m.section != "" || m.pendingBundleAction != nil {
		return m, nil
	}
	m.input = text
	if m.running {
		return m.submitRunningInput()
	}
	return m.submitComposerInput()
}
