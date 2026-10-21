package worktui

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workhome"
)

// BridgeState is a display-only projection for the terminal and desktop.
// The Go model remains the sole owner of commands, runs, approvals and history.
type BridgeState struct {
	Mode                 string             `json:"mode"`
	Panel                *uisnapshot.Panel  `json:"panel,omitempty"`
	Messages             []BridgeMessage    `json:"messages"`
	Commands             []BridgeCommand    `json:"commands"`
	Actions              []workhome.Action  `json:"actions"`
	Title                string             `json:"title"`
	Source               string             `json:"source"`
	ConversationID       string             `json:"conversation_id,omitempty"`
	GoalID               string             `json:"goal_id,omitempty"`
	Provider             string             `json:"provider,omitempty"`
	Model                string             `json:"model,omitempty"`
	Theme                string             `json:"theme"`
	Status               string             `json:"status,omitempty"`
	StatusLine           []string           `json:"status_line"`
	StatusLineEnabled    bool               `json:"status_line_enabled"`
	Input                string             `json:"input,omitempty"`
	Running              bool               `json:"running"`
	Cancelling           bool               `json:"cancelling"`
	Elapsed              string             `json:"elapsed,omitempty"`
	LoadingFrame         int                `json:"loading_frame"`
	Queued               int                `json:"queued"`
	Queue                []BridgeQueuedRun  `json:"queue"`
	QueueResult          *BridgeQueueResult `json:"queue_result,omitempty"`
	GuidanceID           string             `json:"guidance_id,omitempty"`
	Home                 bool               `json:"home"`
	FirstRun             bool               `json:"first_run"`
	SupervisorNeedsStart bool               `json:"supervisor_needs_start"`
	Checks               []BridgeCheck      `json:"checks,omitempty"`
	Overview             *workhome.Overview `json:"overview,omitempty"`
	OverviewIssue        string             `json:"overview_issue,omitempty"`
	External             *BridgeExternal    `json:"external,omitempty"`
	PendingApproval      *BridgeApproval    `json:"pending_approval,omitempty"`
	Notice               string             `json:"notice,omitempty"`
	Destination          string             `json:"destination,omitempty"`
}

type BridgeApproval struct {
	ID int `json:"id"`
}

// BridgeExternal asks the frontend to give the terminal to a process. The
// command is constructed by the same Go paths used by the native TUI.
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
	Role   string             `json:"role"`
	Text   string             `json:"text"`
	Bundle *BundleSummary     `json:"bundle,omitempty"`
	Result *uisnapshot.Result `json:"result,omitempty"`
}

type BridgeCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Advanced    bool   `json:"advanced"`
	NeedsInput  bool   `json:"needs_input"`
}
type BridgeCheck struct {
	Title  string `json:"title"`
	Health string `json:"health"`
	State  string `json:"state"`
	Tone   string `json:"tone"`
}

func (m Model) BridgeState() BridgeState {
	state := BridgeState{
		Mode:              "chat",
		Messages:          make([]BridgeMessage, 0, len(m.messages)),
		Commands:          make([]BridgeCommand, 0),
		Actions:           m.config.Attention.Next(),
		Title:             m.title,
		Source:            m.source,
		ConversationID:    m.conversation,
		GoalID:            m.goalID,
		Provider:          m.modelStatus.Provider,
		Model:             m.modelStatus.Model,
		Theme:             m.theme,
		Status:            m.status,
		StatusLine:        m.statusLineItems(),
		StatusLineEnabled: m.statusLineEnabled,
		Input:             m.input,
		Running:           m.running,
		Cancelling:        m.cancelling,
		Elapsed:           m.workElapsed(),
		LoadingFrame:      m.loadingFrame,
		Queued:            len(m.queue),
		Queue:             make([]BridgeQueuedRun, 0, len(m.queue)),
		QueueResult:       m.bridgeQueueResult,
		Home:              m.home,
		FirstRun:          m.firstRun,
		External:          m.bridgeExternal,
		Notice:            m.bridgeNotice,
	}
	for _, item := range m.queue {
		state.Queue = append(state.Queue, BridgeQueuedRun{ID: bridgeQueuedRunID(item), Text: item.prompt})
	}
	state.GuidanceID = m.bridgeGuidanceID()
	if !m.overview.At.IsZero() {
		copy := m.overview
		state.Overview = &copy
	}
	state.OverviewIssue = m.overviewIssue
	for _, check := range m.checks {
		state.Checks = append(state.Checks, BridgeCheck{Title: singleLine(check.Title), Health: check.HealthLine(), State: check.State, Tone: checkTone(check.State)})
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
		state.Messages = append(state.Messages, BridgeMessage{Role: item.role, Text: item.text, Bundle: cloneBundlePointer(item.bundle), Result: item.result})
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
		state.PendingApproval = &BridgeApproval{ID: m.interaction.ID}
	}
	return state
}

func bridgePanelID(panel uisnapshot.Panel) string {
	panel.ID = ""
	data, _ := json.Marshal(panel)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// RespondBridgeApproval binds a desktop decision to the exact live request.
// A retained transcript card can never approve a later request.
func (m Model) RespondBridgeApproval(id int, approved bool) (tea.Model, tea.Cmd) {
	m.bridgeNotice = ""
	if m.interaction == nil || m.interaction.ID != id {
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
	case "/chat", "/knowledge", "/new", "/overview", "/goals", "/conversations", "/inbox", "/followup", "/history", "/memory", "/context", "/usage", "/data", "/recovery", "/model", "/settings", "/help":
	default:
		known := false
		for _, entry := range m.availableCommandEntries() {
			known = known || (entry.command == command && entry.kind == "command")
		}
		if !known {
			return m, nil
		}
	}
	if (m.running && command != "/chat") || m.controlBusy || m.followupSaving || m.recallBusy || m.bridgeExternal != nil {
		m.status = "Finish or stop the current operation before switching views."
		m.bridgeNotice = m.status
		return m, nil
	}
	if m.models != nil {
		m.models.Close()
		m.models = nil
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
// the terminal. The callback paths are shared with the native TUI.
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

// SubmitText routes a complete frontend request through the same Work TUI
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
