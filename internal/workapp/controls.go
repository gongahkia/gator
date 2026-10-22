package workapp

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
	Kind, Help   string
	Secret       bool
}
type ControlAction struct {
	Key, Label, Action, Description string
	Detail, Command, GoalID, WorkID string
	Family                          string
	External                        []string
	Args                            []string
	Fields                          []ControlField
	Review                          bool
	ConnectorID                     string
	ConnectorReview                 string
}
type SourceMove struct{ From, To string }

type ControlScreen struct {
	Location             string
	SourceMoves          []SourceMove
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
	m.controlReturnHome = m.home && len(m.messages) == 0
	m.section, m.home, m.launcher = "controls", false, false
	m.controlFamily = family
	m.controlRootArgs = append([]string(nil), args...)
	m.controlScreen = ControlScreen{}
	m.controlForm = nil
	m.controlReview = false
	m.controlResultNotice = ""
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
	m.controlBackground = false
	call, family := m.config.Controls, m.controlFamily
	return func() tea.Msg {
		screen, err := call(ctx, family, action, args)
		return controlDone{generation, screen, err}
	}
}

func (m *Model) refreshControlBackground() tea.Cmd {
	cmd := m.controlCommand("status", m.controlRootArgs)
	m.controlBusy, m.controlBackground = false, true
	return cmd
}

func (m *Model) cancelControlBackground() {
	if !m.controlBackground {
		return
	}
	if m.controlCancel != nil {
		m.controlCancel()
		m.controlCancel = nil
	}
	m.controlGeneration++
	m.controlBackground = false
}

func preservedControlSelection(old, next ControlScreen, selected int) int {
	if old.Location != next.Location || selected < 0 || selected >= len(old.Actions) {
		return 0
	}
	a := old.Actions[selected]
	for i, b := range next.Actions {
		if a.Action == b.Action && strings.Join(a.Args, "\x00") == strings.Join(b.Args, "\x00") && (len(a.Args) > 0 || a.Key == b.Key) {
			return i
		}
	}
	return min(selected, max(0, len(next.Actions)-1))
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
			if len(f.External) > 0 {
				cmd := m.startControlExternal(f.External)
				return m, cmd
			}
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
		if m.controlFamily == "overview" && len(m.controlRootArgs) > 0 {
			m.controlRootArgs = m.controlRootArgs[:len(m.controlRootArgs)-1]
			cmd := m.controlCommand("status", m.controlRootArgs)
			return m, cmd
		}
		if m.controlScreen.Panel.Kind == "confirmation" {
			return m, m.controlCommand("status", m.controlRootArgs)
		}
		m.section = ""
		m.cancelControlBackground()
		if m.controlReturnHome && len(m.messages) == 0 {
			m.home = true
		}
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
		m.cancelControlBackground()
		m.controlResultNotice = ""
		if len(a.Fields) > 0 || a.Review || a.ConnectorID != "" {
			copy := a
			copy.Fields = append([]ControlField(nil), a.Fields...)
			m.controlForm = &copy
			m.controlField = 0
			m.controlReview = len(a.Fields) == 0
			return m, nil
		}
		if a.Family != "" {
			m.controlFamily = a.Family
			m.controlRootArgs = nil
			cmd := m.controlCommand(a.Action, a.Args)
			return m, cmd
		}
		if a.GoalID != "" {
			return m.openOverviewGoal(a.GoalID, a.Command)
		}
		if a.WorkID != "" {
			return m.openOverviewWork(a.WorkID)
		}
		if a.Command != "" {
			return m.runLocalCommand(a.Command)
		}
		if (a.Action == "select" && m.controlFamily == "recovery") || ((a.Action == "section" || a.Action == "item" || a.Action == "status") && m.controlFamily == "overview") {
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
	if p.Notice == "" {
		p.Notice = m.controlResultNotice
	}
	if p.Title == "" {
		p.Title = "Loading " + m.controlFamily
		p.Kind = "controls"
	}
	if f := m.controlForm; f != nil {
		p = uisnapshot.Panel{Title: f.Label, Kind: "settings-form", Selected: m.controlField, Notice: m.sectionNotice, Subtitle: f.Description}
		if m.controlReview {
			p.Kind = "confirmation"
			fields := []uisnapshot.Field{}
			for _, field := range f.Fields {
				value := field.Value
				if field.Secret {
					value = "New credential (hidden)"
				}
				fields = append(fields, uisnapshot.Field{Label: field.Label, Value: value})
			}
			if len(fields) > 0 {
				p.Blocks = []uisnapshot.Block{{Title: "Review details", Fields: fields}}
			}
			back := "cancel"
			if len(f.Fields) > 0 {
				back = "edit"
			}
			p.Hints = []uisnapshot.Hint{{Shortcut: "Enter", Action: controlReviewLabel(f.Action, f.Label)}, {Shortcut: "Esc", Action: back}}
		} else {
			for _, field := range f.Fields {
				value := field.Value
				if field.Secret {
					value = strings.Repeat("•", len([]rune(value)))
				}
				if value == "" {
					value = " "
				}
				input := bridgeInput(field.Value, 8192, false)
				input.Kind, input.Help = field.Kind, field.Help
				if field.Kind == "mappings" {
					input.Value = bridgeMappingsJSON(field.Value)
				}
				if field.Secret {
					input.Secret, input.Value = true, ""
				}
				p.Rows = append(p.Rows, uisnapshot.Row{Label: field.Label, Detail: value, Input: input})
			}
			p.Hints = []uisnapshot.Hint{{Shortcut: "Tab", Action: "next field"}, {Shortcut: "Ctrl+R", Action: "review"}, {Shortcut: "Ctrl+U", Action: "clear"}, {Shortcut: "Esc", Action: "cancel"}}
		}
		return p
	}
	for _, a := range m.controlScreen.Actions {
		row := uisnapshot.Row{Label: a.Label, Detail: a.Detail}
		if a.Action == "seen" || a.Action == "start" || a.Action == "retry" {
			row.Affordance = "action"
		}
		p.Rows = append(p.Rows, row)
		hint := uisnapshot.Hint{Shortcut: a.Key, Action: a.Label}
		if p.Kind == "confirmation" && a.Action == "apply" {
			hint.Tone = "error"
		}
		p.Hints = append(p.Hints, hint)
	}
	p.Hints = append(p.Hints, uisnapshot.Hint{Shortcut: "Esc", Action: "back"})
	if m.controlBusy {
		p.Notice = "Working… Esc stops the operation"
	}
	return p
}

func controlReviewLabel(action, fallback string) string {
	labels := map[string]string{"limits": "Save limits", "retention": "Save retention", "backup": "Export backup", "verify": "Verify backup",
		"restore-plan": "Preview restore", "relink-plan": "Preview source changes", "install": "Enable start at login", "remove": "Disable start at login",
		"token": "Replace credentials", "finish-restore-review": "Mark setup reviewed"}
	if label := labels[action]; label != "" {
		return label
	}
	return fallback
}

func (m *Model) clearRemovedData(screen ControlScreen) {
	for _, move := range screen.SourceMoves {
		m.source = moveSourcePath(m.source, move.From, move.To)
		m.primarySource = moveSourcePath(m.primarySource, move.From, move.To)
		m.options.RefreshSource = true
		m.snapshot = ""
		m.contextState = ContextState{}
		m.config.Goals = nil
	}

	reset := screen.ResetConversation
	for _, id := range screen.RemovedConversations {
		if id == m.conversation {
			reset = true
		}
	}
	if !reset {
		return
	}
	m.overviewGeneration++
	m.overview = workhome.Overview{}
	m.overviewBusy = false
	m.messages = nil
	m.conversation = ""
	if m.primary {
		m.primaryID = ""
	}
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
	m.inboxDetail = ""
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
				m.primarySource = fresh.SourcePath
			}
		}
	}
}
