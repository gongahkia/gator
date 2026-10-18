package worktui

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workhome"
)

// BridgeState is a display-only projection for the React terminal frontend.
// The Go model remains the sole owner of commands, runs, approvals and history.
type BridgeState struct {
	Mode              string            `json:"mode"`
	Panel             *uisnapshot.Panel `json:"panel,omitempty"`
	Messages          []BridgeMessage   `json:"messages"`
	Commands          []BridgeCommand   `json:"commands"`
	Actions           []workhome.Action `json:"actions"`
	Title             string            `json:"title"`
	Source            string            `json:"source"`
	GoalID            string            `json:"goal_id,omitempty"`
	Provider          string            `json:"provider,omitempty"`
	Model             string            `json:"model,omitempty"`
	Status            string            `json:"status,omitempty"`
	StatusLine        []string          `json:"status_line"`
	StatusLineEnabled bool              `json:"status_line_enabled"`
	Input             string            `json:"input,omitempty"`
	Running           bool              `json:"running"`
	Cancelling        bool              `json:"cancelling"`
	Elapsed           string            `json:"elapsed,omitempty"`
	LoadingFrame      int               `json:"loading_frame"`
	Queued            int               `json:"queued"`
	Home              bool              `json:"home"`
	FirstRun          bool              `json:"first_run"`
	External          *BridgeExternal   `json:"external,omitempty"`
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
	return &BridgeExternal{Kind: kind, Program: command.Path, Args: append([]string(nil), command.Args[1:]...),
		Dir: command.Dir, Env: append([]string(nil), command.Env...), Action: action, ID: id}
}

type BridgeMessage struct {
	Role   string         `json:"role"`
	Text   string         `json:"text"`
	Bundle *BundleSummary `json:"bundle,omitempty"`
}

type BridgeCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Advanced    bool   `json:"advanced"`
	NeedsInput  bool   `json:"needs_input"`
}

func (m Model) BridgeState() BridgeState {
	state := BridgeState{
		Mode:              "chat",
		Messages:          make([]BridgeMessage, 0, len(m.messages)),
		Commands:          make([]BridgeCommand, 0),
		Actions:           m.config.Attention.Next(),
		Title:             m.title,
		Source:            m.source,
		GoalID:            m.goalID,
		Provider:          m.modelStatus.Provider,
		Model:             m.modelStatus.Model,
		Status:            m.status,
		StatusLine:        m.statusLineItems(),
		StatusLineEnabled: m.statusLineEnabled,
		Input:             m.input,
		Running:           m.running,
		Cancelling:        m.cancelling,
		Elapsed:           m.workElapsed(),
		LoadingFrame:      m.loadingFrame,
		Queued:            len(m.queue),
		Home:              m.home,
		FirstRun:          m.firstRun,
		External:          m.bridgeExternal,
	}
	for _, item := range m.messages {
		state.Messages = append(state.Messages, BridgeMessage{Role: item.role, Text: item.text, Bundle: cloneBundlePointer(item.bundle)})
	}
	for _, item := range m.availableCommandEntries() {
		state.Commands = append(state.Commands, BridgeCommand{
			Name: strings.TrimPrefix(item.command, "/"), Description: item.subtitle,
			Advanced: item.advanced, NeedsInput: item.kind == "command-input",
		})
	}
	if panel := m.bridgePanel(); panel != nil {
		state.Mode = "panel"
		state.Panel = panel
	}
	return state
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
