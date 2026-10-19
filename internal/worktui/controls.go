package worktui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workhome"
)

// Control screens reuse the terminal's existing picker, form and review panel.
// Host callbacks own validation and state mutations; the bridge receives only
// the display projection, never action arguments or secret form values.
type ControlField struct {
	Label, Value string
	Secret       bool
}
type ControlAction struct {
	Key, Label, Action, Description string
	Args                            []string
	Fields                          []ControlField
	Review                          bool
	ConnectorID                     string
	ConnectorReview                 string
}
type ControlScreen struct {
	ResetConversation    bool
	ClearGoal            bool
	RemovedConversations []string
	Panel                uisnapshot.Panel
	Actions              []ControlAction
}
type controlDone struct {
	Generation uint64
	Screen     ControlScreen
	Err        error
}

func (m *Model) openControls(family string, args []string) tea.Cmd {
	m.section, m.home, m.launcher = "controls", false, false
	m.controlFamily = family
	m.controlRootArgs = append([]string(nil), args...)
	m.controlScreen = ControlScreen{}
	m.controlForm = nil
	m.controlReview = false
	m.sectionNotice = ""
	return m.controlCommand("status", args)
}
func (m *Model) controlCommand(action string, args []string) tea.Cmd {
	if m.config.Controls == nil {
		return nil
	}
	if m.controlCancel != nil {
		m.controlCancel()
	}
	m.controlGeneration++
	generation := m.controlGeneration
	ctx, cancel := context.WithCancel(context.Background())
	m.controlCancel = cancel
	m.controlBusy = true
	call, family := m.config.Controls, m.controlFamily
	return func() tea.Msg {
		screen, err := call(ctx, family, action, args)
		return controlDone{generation, screen, err}
	}
}
func (m Model) updateControls(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.running && m.controlFamily == "data" && key.String() != "esc" {
		m.sectionNotice = "Finish or stop the current Work before changing private data."
		return m, nil
	}
	if m.controlBusy {
		if key.String() == "esc" && m.controlCancel != nil {
			m.controlCancel()
			m.sectionNotice = "Stopping…"
		}
		return m, nil
	}
	if f := m.controlForm; f != nil {
		if key.String() == "esc" {
			if m.controlReview && len(f.Fields) > 0 {
				m.controlReview = false
			} else {
				m.controlForm = nil
			}
			return m, nil
		}
		if m.controlReview {
			if key.String() != "enter" {
				return m, nil
			}
			args := append([]string(nil), f.Args...)
			for _, field := range f.Fields {
				args = append(args, field.Value)
			}
			if f.ConnectorID != "" {
				m.controlForm = nil
				m.controlReview = false
				arguments := []string{"login", f.ConnectorID}
				if f.ConnectorReview != "" {
					arguments = append(arguments, "--review-digest", f.ConnectorReview)
				}
				if err := m.startConnectorCommand("reconnect", f.ConnectorID, arguments); err != nil {
					m.sectionNotice = err.Error()
				}
				return m, nil
			}
			m.controlForm = nil
			m.controlReview = false
			return m, m.controlCommand(f.Action, args)
		}
		switch key.String() {
		case "ctrl+r":
			m.controlReview = true
		case "tab", "down":
			m.controlField = (m.controlField + 1) % max(1, len(f.Fields))
		case "shift+tab", "up":
			m.controlField = (m.controlField + max(1, len(f.Fields)) - 1) % max(1, len(f.Fields))
		default:
			if len(f.Fields) > 0 {
				editFormText(&f.Fields[m.controlField].Value, key, 8192)
			}
		}
		return m, nil
	}
	if key.String() == "esc" {
		if m.controlScreen.Panel.Kind == "confirmation" {
			return m, m.controlCommand("status", m.controlRootArgs)
		}
		m.section = ""
		return m, nil
	}
	if key.String() == "up" {
		m.selected = max(0, m.selected-1)
		return m, nil
	}
	if key.String() == "down" {
		m.selected = min(max(0, len(m.controlScreen.Actions)-1), m.selected+1)
		return m, nil
	}
	for i, a := range m.controlScreen.Actions {
		if key.String() != strings.ToLower(a.Key) && !(key.String() == "enter" && i == m.selected) {
			continue
		}
		if len(a.Fields) > 0 || a.Review || a.ConnectorID != "" {
			copy := a
			copy.Fields = append([]ControlField(nil), a.Fields...)
			m.controlForm = &copy
			m.controlField = 0
			m.controlReview = len(a.Fields) == 0
			return m, nil
		}
		if a.Action == "select" && m.controlFamily == "recovery" {
			m.controlRootArgs = append([]string(nil), a.Args...)
		}
		return m, m.controlCommand(a.Action, a.Args)
	}
	return m, nil
}
func (m Model) controlsPanel() uisnapshot.Panel {
	p := m.controlScreen.Panel
	p.Selected = m.selected
	p.Notice = m.sectionNotice
	if p.Title == "" {
		p.Title = "Loading " + m.controlFamily
		p.Kind = "controls"
	}
	if f := m.controlForm; f != nil {
		p = uisnapshot.Panel{Title: f.Label, Kind: "settings-form", Selected: m.controlField, Notice: m.sectionNotice, Subtitle: f.Description}
		if m.controlReview {
			p.Kind = "confirmation"
			lines := []string{f.Description}
			for _, field := range f.Fields {
				value := field.Value
				if field.Secret {
					value = "New credential (hidden)"
				}
				lines = append(lines, field.Label+": "+value)
			}
			p.Blocks = []uisnapshot.Block{{Title: "Review", Lines: lines}}
			p.Hints = []uisnapshot.Hint{{Shortcut: "Enter", Action: "confirm"}, {Shortcut: "Esc", Action: "edit or cancel"}}
		} else {
			for _, field := range f.Fields {
				value := field.Value
				if field.Secret {
					value = strings.Repeat("•", len([]rune(value)))
				}
				if value == "" {
					value = " "
				}
				p.Rows = append(p.Rows, uisnapshot.Row{Label: field.Label, Detail: value})
			}
			p.Hints = []uisnapshot.Hint{{Shortcut: "Tab", Action: "next field"}, {Shortcut: "Ctrl+R", Action: "review"}, {Shortcut: "Ctrl+U", Action: "clear"}, {Shortcut: "Esc", Action: "cancel"}}
		}
		return p
	}
	for _, a := range m.controlScreen.Actions {
		p.Rows = append(p.Rows, uisnapshot.Row{Label: a.Label})
		p.Hints = append(p.Hints, uisnapshot.Hint{Shortcut: a.Key, Action: a.Label})
	}
	p.Hints = append(p.Hints, uisnapshot.Hint{Shortcut: "Esc", Action: "back"})
	if m.controlBusy {
		p.Notice = "Working… Esc stops the operation"
	}
	return p
}

func (m *Model) clearRemovedData(screen ControlScreen) {
	reset := screen.ResetConversation
	for _, id := range screen.RemovedConversations {
		if id == m.conversation {
			reset = true
		}
	}
	if !reset {
		return
	}
	m.messages = nil
	m.conversation = ""
	m.revision = ""
	m.snapshot = ""
	m.lastWorkID = ""
	m.lastOutput = ""
	m.lastBundle = BundleSummary{}
	m.queue = nil
	m.options.ExcludedKnowledgeIDs = nil
	m.options.PreviousArtifacts = nil
	m.config.Conversations = nil
	m.config.Goals = nil
	m.config.Jobs = nil
	m.config.Inbox = nil
	m.config.Attention = workhome.Attention{}
	m.checks = nil
	m.checkDetail = nil
	m.followupForm = nil
	m.entries = nil
	m.tasks = map[string]string{}
	m.contextState = ContextState{}
	m.contextDetail = nil
	m.contextEdit = ""
	m.contextWorkID = ""
	m.contextEditing = false
	m.contextConfirm = false
	if screen.ClearGoal {
		m.goalID = ""
		if m.config.ClearGoal != nil {
			m.config.ClearGoal()
		}
		if m.config.ResetPrivateState != nil {
			fresh, err := m.config.ResetPrivateState()
			if err != nil {
				m.sectionNotice = "Data removed; choose a workspace with /source: " + err.Error()
			} else {
				m.source, m.title = fresh.SourcePath, fresh.Title
			}
		}
	}
}
