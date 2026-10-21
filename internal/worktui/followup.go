package worktui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/followup"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
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
type followupSaved struct {
	Goal goal.Record
	Job  jobs.Definition
	Err  error
}
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
	if !definition.GoalWatch {
		m.sectionNotice = "This schedule uses advanced settings. Edit it with /jobs edit " + definition.ID + " or create a reviewed project check with N."
		return
	}
	draft, err := followup.New(definition.SourcePath, definition.GoalID)
	if err != nil {
		m.sectionNotice = err.Error()
		return
	}
	draft.Enabled = definition.Enabled
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
	if key.String() == "h" && m.config.Controls != nil {
		args := []string{}
		if m.checkDetail != nil {
			args = append(args, m.checkDetail.ID)
		}
		return m, m.openControls("recovery", args)
	}
	if m.checkDetail != nil {
		switch key.String() {
		case "e":
			m.editFollowup()
		case "p":
			if m.checkDetail.State == "goal paused" {
				m.sectionNotice = m.checkDetail.NextAction
				return m, nil
			}
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
			save, draft := m.config.SaveFollowup, f.Draft
			m.followupSaving, m.sectionNotice = true, "Saving your reviewed agreement…"
			return m, func() tea.Msg { record, definition, err := save(draft); return followupSaved{record, definition, err} }
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

func (m Model) finishFollowup(value followupSaved) (tea.Model, tea.Cmd) {
	m.followupSaving = false
	if value.Err != nil {
		m.sectionNotice = value.Err.Error()
		return m, nil
	}
	record, definition := value.Goal, value.Job
	if m.goalID != record.ID {
		m.messages, m.revision, m.snapshot, m.lastWorkID = nil, "", "", ""
	}
	if m.config.SelectGoal != nil {
		state, err := m.config.SelectGoal(record.ID)
		if err != nil {
			m.sectionNotice = "Saved check; could not select its goal: " + err.Error()
			return m, m.refreshChecksCommand()
		}
		m.conversation, m.source, m.title = state.ConversationID, state.SourcePath, state.Title
	} else {
		m.source, m.title, m.conversation = record.SourcePath, record.Title, record.ConversationID
	}
	m.goalID = record.ID
	m.followupForm, m.checkDetail = nil, nil
	m.sectionNotice = "Saved " + definition.Name + ". Review background health below; a stopped supervisor cannot run checks."
	m.refreshAttention()
	return m, m.refreshChecksCommand()
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
	p := uisnapshot.Panel{Kind: "followup", Title: "Follow-up checks", Selected: m.selected, Notice: m.sectionNotice}
	if f := m.followupForm; f != nil {
		if f.Review {
			p.Kind, p.Title = "confirmation", "Review ongoing check"
			p.Subtitle = f.Draft.Title
			agreement := uisnapshot.Block{Title: "Your agreement"}
			for _, line := range f.Draft.ReviewLines() {
				if label, value, ok := strings.Cut(line, ": "); ok {
					agreement.Fields = append(agreement.Fields, uisnapshot.Field{Label: label, Value: value})
				} else {
					agreement.Lines = append(agreement.Lines, line)
				}
			}
			p.Blocks = []uisnapshot.Block{agreement}
			p.Hints = []uisnapshot.Hint{{Shortcut: "Enter", Action: "save check"}, {Shortcut: "E", Action: "edit"}, {Shortcut: "Esc", Action: "back"}}
			return p
		}
		d := f.Draft
		p.Kind, p.Title, p.Selected = "followup-form", "Set up a follow-up check", f.Field
		p.Subtitle = "Choose the outcome, sources, timing, and permitted actions. Nothing runs until you review and save."
		selectedSource := "Local files only"
		if m.config.ConnectorChoices != nil {
			choices := m.config.ConnectorChoices()
			if len(choices) > 0 {
				selectedSource = choices[m.selected%len(choices)] + " · selected: " + valueOrNone(strings.Join(d.ConnectorIDs, ", "))
			}
		}
		p.Rows = []uisnapshot.Row{{Label: "Title", Detail: d.Title}, {Label: "Desired outcome", Detail: d.Outcome}, {Label: "Local workspace", Detail: d.SourcePath}, {Label: "Connected sources · ←/→ choose, Space toggle", Detail: selectedSource}, {Label: "Mail selection · exact Gmail query", Detail: valueOrNone(d.GmailQuery)}, {Label: "Calendar window · ←/→ change", Detail: strconv.Itoa(d.CalendarDays) + " days (0 disables)"}, {Label: "When Gator works · ←/→ change", Detail: fmt.Sprintf("Every %d minutes", d.IntervalMinutes)}, {Label: "Notify me when", Detail: d.AlertWhen}, {Label: "What Gator may do · ←/→ change", Detail: string(d.Mode) + "; sending requires foreground approval"}, {Label: "Per-try budget · ←/→ change", Detail: fmt.Sprintf("%d requests · %d tokens · %ds", d.MaxRequests, d.MaxTokens, d.MaxSeconds)}, {Label: "Daily Work checks · ←/→ change", Detail: fmt.Sprintf("Up to %d per UTC day; then review with Enter", d.MaxChecksPerDay)}}
		p.Hints = []uisnapshot.Hint{{Shortcut: "Tab", Action: "next field"}, {Shortcut: "Ctrl+R", Action: "review"}, {Shortcut: "Ctrl+U", Action: "clear field"}, {Shortcut: "Esc", Action: "cancel"}}
		for index, value := range map[int]string{0: d.Title, 1: d.Outcome, 2: d.SourcePath, 4: d.GmailQuery, 7: d.AlertWhen} {
			p.Rows[index].Input = bridgeInput(value, 4096, index == 1 || index == 7)
		}
		p.Rows[0].Input.Limit = 100
		p.Rows[2].Input.Kind = "directory"
		p.Rows[4].Input.Limit = 1024
		p.Rows[7].Input.Limit = 2048
		p.Rows[3].Label = "Connected sources"
		selected, _ := json.Marshal(d.ConnectorIDs)
		p.Rows[3].Input = &uisnapshot.Input{Kind: "multi", Value: string(selected), Limit: 4096, Max: 4,
			Help: "Local files remain included. Choose up to four configured sources; this does not grant new permissions."}
		var connectors []string
		if m.config.ConnectorChoices != nil {
			connectors = m.config.ConnectorChoices()
		}
		for _, id := range connectors {
			p.Rows[3].Input.Options = append(p.Rows[3].Input.Options, uisnapshot.Option{Value: id, Label: id})
		}
		for _, id := range d.ConnectorIDs {
			if !sliceContains(connectors, id) {
				p.Rows[3].Input.Options = append(p.Rows[3].Input.Options, uisnapshot.Option{Value: id, Label: id + " (unavailable)"})
			}
		}
		p.Rows[5].Label, p.Rows[5].Input = "Calendar window (days)", bridgeNumberInput(int64(d.CalendarDays), 0, 30)
		p.Rows[5].Input.Help = "Zero excludes calendar events. Mail and calendar filters require a connected source."
		p.Rows[6].Label, p.Rows[6].Input = "Check frequency", bridgeSelect(strconv.Itoa(d.IntervalMinutes),
			uisnapshot.Option{Value: "5", Label: "Every 5 minutes"}, uisnapshot.Option{Value: "15", Label: "Every 15 minutes"},
			uisnapshot.Option{Value: "30", Label: "Every 30 minutes"}, uisnapshot.Option{Value: "60", Label: "Every hour"})
		p.Rows[8].Label, p.Rows[8].Input = "What Gator may do", bridgeSelect(string(d.Mode),
			uisnapshot.Option{Value: "inspect", Label: "Read and recommend"}, uisnapshot.Option{Value: "draft", Label: "Read and prepare drafts for review"})
		p.Rows[8].Input.Help = "Sending and publishing always require separate foreground approval."
		p.Rows[9].Label = "Limits for each attempt"
		p.Rows[9].Input = &uisnapshot.Input{Kind: "budget", Limit: 128, Value: fmt.Sprintf("%d,%d,%d", d.MaxRequests, d.MaxTokens, d.MaxSeconds),
			Parts: []uisnapshot.NumberPart{{Label: "Provider requests", Min: 1, Max: 256}, {Label: "Tokens", Min: 1000, Max: 1000000}, {Label: "Time (seconds)", Min: 30, Max: 1800}},
			Help:  "Up to three transient attempts per check. Usage and shared limits also apply."}
		p.Rows[10].Label, p.Rows[10].Input = "Checks per UTC day", bridgeNumberInput(int64(d.MaxChecksPerDay), 1, 288)
		for index := range p.Rows {
			switch {
			case index < 2:
				p.Rows[index].Group = "Outcome"
			case index < 6:
				p.Rows[index].Group = "Sources"
			case index < 8:
				p.Rows[index].Group = "Timing and notifications"
			case index == 8:
				p.Rows[index].Group = "Permissions"
			default:
				p.Rows[index].Group = "Usage limits"
			}
		}
		return p
	}
	if c := m.checkDetail; c != nil {
		p.Title, p.Subtitle = "Follow-up details", c.Title
		last, next := "Not checked yet", "Not scheduled"
		if !c.LastSuccess.IsZero() {
			last = c.LastSuccess.Local().Format("2 Jan, 15:04")
		}
		if !c.NextDue.IsZero() {
			next = c.NextDue.Local().Format("2 Jan, 15:04")
		}
		p.Summary = []uisnapshot.Field{{Label: "Status", Value: c.State, Tone: checkTone(c.State)}, {Label: "Last successful check", Value: last}, {Label: "Next scheduled check", Value: next}}
		permission := "Read and recommend"
		if c.Definition.Mode == action.Draft {
			permission = "Read and prepare drafts"
		}
		p.Blocks = []uisnapshot.Block{
			{Title: "Outcome", Role: "content", Lines: []string{c.Outcome}},
			{Title: "Next step", Lines: []string{c.NextAction, c.Issue}},
			{Title: "Permissions", Fields: []uisnapshot.Field{{Label: "Allowed actions", Value: permission}, {Label: "Sending or publishing", Value: "Separate foreground approval required"}}},
			{Title: "Timing and control", Collapsible: true, Lines: []string{checkFrequency(*c), "Checks depend on this computer and the supervisor running.", "Pausing stops future checks; a current check may finish.", "Agreement edits apply to future checks."}},
			{Title: "Sources and permissions", Collapsible: true, Lines: checkSourceLines(*c)},
			{Title: "Usage and limits", Collapsible: true, Lines: checkLimitLines(*c)},
		}
		pause := "resume check"
		if c.Enabled {
			pause = "pause check"
		}
		p.Hints = []uisnapshot.Hint{{Shortcut: "P", Action: pause}, {Shortcut: "X", Action: "check now", Group: "more"}, {Shortcut: "H", Action: "recovery and offline history", Group: "more"}, {Shortcut: "R", Action: "refresh"}, {Shortcut: "Esc", Action: "back"}}
		if c.Definition.GoalWatch {
			p.Hints = append(p.Hints, uisnapshot.Hint{Shortcut: "E", Action: "edit agreement"})
		}
		if !m.config.SupervisorRunning {
			p.Hints = append(p.Hints, uisnapshot.Hint{Shortcut: "S", Action: "start supervisor", Group: "more"})
		}
		return p
	}
	for _, c := range m.checks {
		at := ""
		if !c.LastAttempt.IsZero() {
			at = c.LastAttempt.Format(time.RFC3339)
		}
		p.Rows = append(p.Rows, uisnapshot.Row{Label: c.Title, Detail: c.HealthLine(), State: c.State, Tone: checkTone(c.State), Source: c.Definition.SourcePath, UpdatedAt: at})
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

func checkFrequency(c workhome.Check) string {
	if c.Definition.PollMinutes > 0 {
		return fmt.Sprintf("Every %d minutes while the host and supervisor run", c.Definition.PollMinutes)
	}
	return c.Definition.Schedule + " · " + c.Definition.Timezone
}
func checkSourceLines(c workhome.Check) []string {
	lines := []string{c.Definition.SourcePath, "Notify when: " + valueOrNone(c.Definition.AlertWhen), "May " + string(c.Definition.Mode) + "; external actions forbidden unattended"}
	for _, source := range c.Definition.WatchSources {
		line := source.ConnectorID + " / " + source.Operation
		if source.Query != "" {
			line += " · query: " + source.Query
		}
		if source.Days > 0 {
			line += fmt.Sprintf(" · next %d days", source.Days)
		}
		lines = append(lines, line)
	}
	return lines
}
func checkLimitLines(c workhome.Check) []string {
	requests, seconds := c.Definition.Limits.ModelRequests, c.Definition.Limits.WallSeconds
	if requests == 0 {
		requests = 256
	}
	if seconds == 0 {
		seconds = 1800
	}
	tokens := "not capped"
	if c.Definition.Limits.Tokens > 0 {
		tokens = fmt.Sprint(c.Definition.Limits.Tokens)
	}
	daily := "No daily Work-check cap"
	if c.Definition.MaxChecksPerDay > 0 {
		daily = fmt.Sprintf("Daily cap: %d Work checks (UTC), including failed starts", c.Definition.MaxChecksPerDay)
	}
	return []string{fmt.Sprintf("Today (UTC): %d Work checks · %d reported execution requests · %d reported tokens", c.ChecksToday, c.ModelRequests, c.Tokens), fmt.Sprintf("Per try: %d requests · tokens %s · %ds; up to 3 transient tries per check", requests, tokens, seconds), daily, "Cost estimate unavailable. Provider reporting may be incomplete; retrieval and preliminary routing can add separate charges."}
}
