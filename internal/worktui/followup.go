package worktui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/followup"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workhome"
)

type followupForm struct {
	Draft  followup.Draft
	Field  int
	Review bool
}
type checksRefreshed struct {
	Checks  []workhome.Check
	Running bool
	Err     error
}
type checksTick struct{}
type checkActionDone struct {
	Text string
	Err  error
}

func (m Model) refreshChecksCommand() tea.Cmd {
	if m.config.ReadChecks == nil {
		return nil
	}
	read := m.config.ReadChecks
	return func() tea.Msg { checks, running, err := read(); return checksRefreshed{checks, running, err} }
}

func (m *Model) openFollowup() tea.Cmd {
	m.section, m.launcher, m.home = "followup", false, false
	m.checkDetail, m.followupForm, m.sectionNotice = nil, nil, ""
	m.selected = 0
	return m.refreshChecksCommand()
}

func (m *Model) newFollowup() {
	draft, err := followup.New(m.source, m.goalID)
	if err != nil {
		m.sectionNotice = err.Error()
		return
	}
	if m.goalID != "" {
		draft.Title = m.title
	}
	m.followupForm = &followupForm{Draft: draft}
	m.section, m.sectionNotice = "followup", ""
}

func (m *Model) editFollowup() {
	if m.checkDetail == nil {
		return
	}
	definition := m.checkDetail.Definition
	draft, err := followup.New(definition.SourcePath, definition.GoalID)
	if err != nil {
		m.sectionNotice = err.Error()
		return
	}
	draft.JobID, draft.Title, draft.Outcome = definition.ID, definition.Name, definition.Objective
	draft.ExpectedDigest, _ = definition.PolicyDigest()
	draft.ConnectorIDs, draft.AlertWhen, draft.Mode = append([]string(nil), definition.ConnectorIDs...), definition.AlertWhen, definition.Mode
	if draft.AlertWhen == "" {
		draft.AlertWhen = "Material changes and due steps requiring review"
	}
	if definition.PollMinutes != 0 {
		draft.IntervalMinutes = definition.PollMinutes
	}
	if definition.Limits.ModelRequests > 0 {
		draft.MaxRequests = definition.Limits.ModelRequests
	}
	if definition.Limits.WallSeconds > 0 {
		draft.MaxSeconds = definition.Limits.WallSeconds
	}
	if definition.Limits.Tokens > 0 {
		draft.MaxTokens = definition.Limits.Tokens
	}
	if definition.MaxChecksPerDay > 0 {
		draft.MaxChecksPerDay = definition.MaxChecksPerDay
	}
	for _, source := range definition.WatchSources {
		if source.Operation == "gmail_list" {
			draft.GmailQuery = source.Query
		}
		if source.Operation == "calendar_events" {
			draft.CalendarDays = source.Days
		}
	}
	m.followupForm, m.sectionNotice = &followupForm{Draft: draft}, ""
}

func (m Model) updateFollowupKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.followupForm != nil {
		return m.updateFollowupForm(key)
	}
	if key.String() == "esc" {
		if m.checkDetail != nil {
			m.checkDetail = nil
		} else {
			m.section = ""
		}
		return m, nil
	}
	if key.String() == "n" {
		m.newFollowup()
		return m, nil
	}
	if key.String() == "s" && m.config.StartSupervisor != nil {
		start := m.config.StartSupervisor
		m.sectionNotice = "Starting the local supervisor…"
		return m, func() tea.Msg {
			err := start()
			return checkActionDone{"Local supervisor started. Checks run while this computer is on.", err}
		}
	}
	if key.String() == "r" {
		return m, m.refreshChecksCommand()
	}
	if m.checkDetail != nil {
		switch key.String() {
		case "e":
			m.editFollowup()
		case "p":
			if m.config.JobAction == nil {
				return m, nil
			}
			action := "enable"
			if m.checkDetail.Enabled {
				action = "disable"
			}
			id, change := m.checkDetail.ID, m.config.JobAction
			return m, func() tea.Msg { text, err := change([]string{action, id}); return checkActionDone{text, err} }
		case "c":
			if m.checkDetail.GoalID != "" {
				return m.runLocalCommand("/goals")
			}
		case "x":
			if m.config.JobAction == nil {
				return m, nil
			}
			id, run := m.checkDetail.ID, m.config.JobAction
			m.sectionNotice = "Running the selected check…"
			return m, func() tea.Msg { text, err := run([]string{"run", id}); return checkActionDone{text, err} }
		}
		return m, nil
	}
	switch key.String() {
	case "up":
		if m.selected > 0 {
			m.selected--
		}
	case "down":
		if m.selected+1 < len(m.checks) {
			m.selected++
		}
	case "enter":
		if m.selected < len(m.checks) {
			check := m.checks[m.selected]
			m.checkDetail = &check
		}
	}
	return m, nil
}

func (m Model) updateFollowupForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.followupForm
	if f.Review {
		switch key.String() {
		case "esc", "e":
			f.Review = false
		case "enter":
			if m.config.SaveFollowup == nil {
				m.sectionNotice = "Follow-up setup unavailable."
				return m, nil
			}
			record, definition, err := m.config.SaveFollowup(f.Draft)
			if err != nil {
				m.sectionNotice = err.Error()
				return m, nil
			}
			if m.goalID != record.ID {
				m.messages, m.revision, m.snapshot, m.lastWorkID = nil, "", "", ""
			}
			if m.config.SelectGoal != nil {
				state, selectErr := m.config.SelectGoal(record.ID)
				if selectErr != nil {
					m.sectionNotice = "Saved check; could not select its goal: " + selectErr.Error()
					return m, m.refreshChecksCommand()
				}
				m.conversation, m.source, m.title = state.ConversationID, state.SourcePath, state.Title
			}
			m.goalID = record.ID
			m.followupForm, m.checkDetail = nil, nil
			m.sectionNotice = "Saved " + definition.Name + ". Review background health below; a stopped supervisor cannot run checks."
			m.refreshAttention()
			return m, m.refreshChecksCommand()
		}
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.followupForm, m.sectionNotice = nil, ""
		return m, nil
	case "tab", "down":
		f.Field = (f.Field + 1) % 11
		return m, nil
	case "shift+tab", "up":
		f.Field = (f.Field + 10) % 11
		return m, nil
	case "ctrl+r":
		if err := f.Draft.Validate(); err != nil {
			m.sectionNotice = err.Error()
		} else {
			f.Review, m.sectionNotice = true, ""
		}
		return m, nil
	case "enter":
		if f.Field == 10 {
			if err := f.Draft.Validate(); err != nil {
				m.sectionNotice = err.Error()
			} else {
				f.Review, m.sectionNotice = true, ""
			}
		} else {
			f.Field++
		}
		return m, nil
	}
	if f.Field == 6 || f.Field == 8 || f.Field == 9 || f.Field == 10 {
		if key.String() != "left" && key.String() != "right" && key.String() != " " {
			return m, nil
		}
		switch f.Field {
		case 6:
			f.Draft.IntervalMinutes = cycleInt(f.Draft.IntervalMinutes, []int{5, 15, 30, 60})
		case 8:
			if f.Draft.Mode == action.Draft {
				f.Draft.Mode = action.Inspect
			} else {
				f.Draft.Mode = action.Draft
			}
		case 9:
			f.Draft.MaxRequests = cycleInt(f.Draft.MaxRequests, []int{16, 32, 64})
			f.Draft.MaxSeconds = f.Draft.MaxRequests * 300 / 32
			f.Draft.MaxTokens = int64(f.Draft.MaxRequests) * 100000 / 32
		case 10:
			f.Draft.MaxChecksPerDay = cycleInt(f.Draft.MaxChecksPerDay, []int{6, 24, 96})
		}
		return m, nil
	}
	var text *string
	switch f.Field {
	case 0:
		text = &f.Draft.Title
	case 1:
		text = &f.Draft.Outcome
	case 2:
		text = &f.Draft.SourcePath
	case 4:
		text = &f.Draft.GmailQuery
	case 7:
		text = &f.Draft.AlertWhen
	}
	if f.Field == 3 {
		// A focused picker cycles configured sources; Space toggles membership.
		choices := []string{}
		if m.config.ConnectorChoices != nil {
			choices = m.config.ConnectorChoices()
		}
		if len(choices) == 0 {
			m.sectionNotice = "No connected sources. Local files remain selected; configure /connector to add mail or calendar."
			return m, nil
		}
		if key.String() == "left" {
			m.selected = (m.selected + len(choices) - 1) % len(choices)
		}
		if key.String() == "right" {
			m.selected = (m.selected + 1) % len(choices)
		}
		if key.String() == " " {
			id := choices[m.selected%len(choices)]
			if sliceContains(f.Draft.ConnectorIDs, id) {
				f.Draft.ConnectorIDs = removeString(f.Draft.ConnectorIDs, id)
			} else {
				f.Draft.ConnectorIDs = append(f.Draft.ConnectorIDs, id)
			}
		}
		return m, nil
	}
	if f.Field == 5 {
		if key.String() == "left" || key.String() == "right" || key.String() == " " {
			f.Draft.CalendarDays = cycleInt(f.Draft.CalendarDays, []int{0, 7, 14, 30})
		}
		return m, nil
	}
	if text != nil {
		editFormText(text, key, 4096)
	}
	return m, nil
}

func cycleInt(current int, choices []int) int {
	for i, v := range choices {
		if v == current {
			return choices[(i+1)%len(choices)]
		}
	}
	return choices[0]
}
func editFormText(text *string, key tea.KeyMsg, limit int) {
	switch key.String() {
	case "ctrl+u":
		*text = ""
	case "backspace":
		r := []rune(*text)
		if len(r) > 0 {
			*text = string(r[:len(r)-1])
		}
	default:
		if (key.Type == tea.KeyRunes || key.String() == " ") && len(*text)+len(key.String()) <= limit {
			*text += key.String()
		}
	}
}

func (m Model) followupPanel() uisnapshot.Panel {
	p := uisnapshot.Panel{Kind: "followup", Title: "Project follow-up", Selected: m.selected, Notice: m.sectionNotice}
	if f := m.followupForm; f != nil {
		if f.Review {
			p.Kind, p.Title = "confirmation", "Review ongoing check"
			p.Blocks = []uisnapshot.Block{{Title: "Your agreement", Lines: f.Draft.ReviewLines()}}
			p.Hints = []uisnapshot.Hint{{Shortcut: "Enter", Action: "save check"}, {Shortcut: "E", Action: "edit"}, {Shortcut: "Esc", Action: "back"}}
			return p
		}
		d := f.Draft
		p.Kind, p.Title, p.Selected = "followup-form", "Set up project follow-up", f.Field
		p.Subtitle = "Choose the outcome, sources, timing, and permitted actions. Nothing runs until you review and save."
		selectedSource := "Local files only"
		if m.config.ConnectorChoices != nil {
			choices := m.config.ConnectorChoices()
			if len(choices) > 0 {
				selectedSource = choices[m.selected%len(choices)] + " · selected: " + valueOrNone(strings.Join(d.ConnectorIDs, ", "))
			}
		}
		p.Rows = []uisnapshot.Row{{Label: "Title", Detail: d.Title}, {Label: "Desired outcome", Detail: d.Outcome}, {Label: "Local workspace", Detail: d.SourcePath}, {Label: "Connected sources · ←/→ choose, Space toggle", Detail: selectedSource}, {Label: "Mail selection · exact Gmail query", Detail: valueOrNone(d.GmailQuery)}, {Label: "Calendar window · ←/→ change", Detail: strconv.Itoa(d.CalendarDays) + " days (0 disables)"}, {Label: "When Gator works · ←/→ change", Detail: fmt.Sprintf("Every %d minutes", d.IntervalMinutes)}, {Label: "Notify me when", Detail: d.AlertWhen}, {Label: "What Gator may do · ←/→ change", Detail: string(d.Mode) + "; sending requires foreground approval"}, {Label: "Per-try budget · ←/→ change", Detail: fmt.Sprintf("%d requests · %d tokens · %ds", d.MaxRequests, d.MaxTokens, d.MaxSeconds)}, {Label: "Daily model checks · ←/→ change", Detail: fmt.Sprintf("Up to %d per UTC day; then review with Enter", d.MaxChecksPerDay)}}
		p.Hints = []uisnapshot.Hint{{Shortcut: "Tab", Action: "next field"}, {Shortcut: "Ctrl+U", Action: "clear field"}, {Shortcut: "Ctrl+R", Action: "review"}, {Shortcut: "Esc", Action: "cancel"}}
		return p
	}
	if c := m.checkDetail; c != nil {
		p.Title, p.Subtitle = "Follow-up details", c.Title
		p.Blocks = []uisnapshot.Block{{Title: "Outcome", Lines: []string{c.Outcome}}, {Title: "Background health", Lines: []string{c.HealthLine(), c.Issue, "Next action: " + c.NextAction}}, {Title: "Sources and permissions", Lines: []string{c.Definition.SourcePath, "Connected: " + valueOrNone(strings.Join(c.Definition.ConnectorIDs, ", ")), "Notify when: " + valueOrNone(c.Definition.AlertWhen), "May " + string(c.Definition.Mode) + "; external actions forbidden unattended"}}, {Title: "Usage and limits", Lines: []string{fmt.Sprintf("Today: %d model checks · %d reported requests · %d reported tokens", c.ChecksToday, c.ModelRequests, c.Tokens), fmt.Sprintf("Per try: %d requests · %d tokens · %ds; up to 3 transient tries per check", c.Definition.Limits.ModelRequests, c.Definition.Limits.Tokens, c.Definition.Limits.WallSeconds), fmt.Sprintf("Daily cap: %d model checks (UTC)", c.Definition.MaxChecksPerDay), "Cost estimate unavailable. Reported usage may be incomplete when a provider omits it."}}}
		p.Hints = []uisnapshot.Hint{{Shortcut: "P", Action: "pause/resume"}, {Shortcut: "E", Action: "edit agreement"}, {Shortcut: "X", Action: "check now"}, {Shortcut: "S", Action: "start supervisor"}, {Shortcut: "R", Action: "refresh"}, {Shortcut: "Esc", Action: "back"}}
		return p
	}
	for _, c := range m.checks {
		p.Rows = append(p.Rows, uisnapshot.Row{Label: c.Title, Detail: c.HealthLine(), State: c.State, Tone: checkTone(c.State)})
	}
	if len(p.Rows) == 0 {
		p.Blocks = []uisnapshot.Block{{Title: "Start with one useful loop", Lines: []string{"Watch project changes, identify the next step, and prepare a response for review.", "Press N to choose sources and review the agreement."}}}
	}
	p.Hints = []uisnapshot.Hint{{Shortcut: "↑/↓", Action: "choose"}, {Shortcut: "Enter", Action: "details"}, {Shortcut: "N", Action: "new check"}, {Shortcut: "S", Action: "start supervisor"}, {Shortcut: "R", Action: "refresh"}, {Shortcut: "Esc", Action: "back"}}
	return p
}
func checkTone(state string) string {
	if state == "offline" || state == "couldn't check" {
		return "warning"
	}
	if state == "paused" {
		return "muted"
	}
	return "success"
}
func nextChecksTick() tea.Cmd {
	return tea.Tick(15*time.Second, func(time.Time) tea.Msg { return checksTick{} })
}
