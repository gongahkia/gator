package worktui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/workhome"
)

// BridgeState is a display-only projection for the React terminal frontend.
// The Go model remains the sole owner of commands, runs, approvals and history.
type BridgeState struct {
	Mode         string            `json:"mode"`
	LegacyView   string            `json:"legacy_view,omitempty"`
	Messages     []BridgeMessage   `json:"messages"`
	Commands     []BridgeCommand   `json:"commands"`
	Actions      []workhome.Action `json:"actions"`
	Title        string            `json:"title"`
	Source       string            `json:"source"`
	GoalID       string            `json:"goal_id,omitempty"`
	Provider     string            `json:"provider,omitempty"`
	Model        string            `json:"model,omitempty"`
	Status       string            `json:"status,omitempty"`
	Input        string            `json:"input,omitempty"`
	Running      bool              `json:"running"`
	Cancelling   bool              `json:"cancelling"`
	Elapsed      string            `json:"elapsed,omitempty"`
	LoadingFrame int               `json:"loading_frame"`
	Queued       int               `json:"queued"`
	Home         bool              `json:"home"`
	FirstRun     bool              `json:"first_run"`
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
		Mode:         "chat",
		Messages:     make([]BridgeMessage, 0, len(m.messages)),
		Commands:     make([]BridgeCommand, 0),
		Actions:      m.config.Attention.Next(),
		Title:        m.title,
		Source:       m.source,
		GoalID:       m.goalID,
		Provider:     m.modelStatus.Provider,
		Model:        m.modelStatus.Model,
		Status:       m.status,
		Input:        m.input,
		Running:      m.running,
		Cancelling:   m.cancelling,
		Elapsed:      m.workElapsed(),
		LoadingFrame: m.loadingFrame,
		Queued:       len(m.queue),
		Home:         m.home,
		FirstRun:     m.firstRun,
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
	if m.models != nil || m.launcher || m.section != "" || m.pendingBundleAction != nil {
		state.Mode = "legacy"
		state.LegacyView = m.View()
	}
	return state
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
