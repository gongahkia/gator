package workapp

import (
	"fmt"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/uisnapshot"
)

func (m Model) bridgePanel() (result *uisnapshot.Panel) {
	defer func() {
		if result != nil {
			if result.BackLabel == "" || m.controlBusy {
				result.BackLabel = m.bridgeBackLabel(*result)
			}
		}
	}()
	if m.models != nil {
		panel := m.models.UISnapshot()
		return &panel
	}
	if m.launcher {
		panel := m.withPanelHelp(m.bridgeLauncherPanel())
		return &panel
	}
	if m.section != "" {
		panel := m.bridgeSectionPanel()
		panel = m.withPanelHelp(panel)
		return &panel
	}
	if m.pendingBundleAction != nil {
		return &uisnapshot.Panel{
			Kind: "confirmation", Title: "Review transfer", Subtitle: "Confirm this change before Gator writes to your files.",
			Blocks: []uisnapshot.Block{{Lines: strings.Split(m.pendingBundleAction.summary, "\n")}},
			Footer: "Enter or Y confirm · Esc or N cancel",
			Hints:  []uisnapshot.Hint{{Shortcut: "Enter", Action: "confirm"}, {Shortcut: "Esc", Action: "cancel"}},
		}
	}
	return nil
}

func (m Model) bridgeBackLabel(panel uisnapshot.Panel) string {
	if m.productHelp {
		return "Close shortcuts"
	}
	if m.models != nil {
		if panel.Kind == "confirmation" {
			return "Cancel model action"
		}
		if panel.Title != "Models" {
			return "Back to model setup"
		}
	}
	if m.controlBusy {
		return "Stop operation"
	}
	if m.controlForm != nil && m.section == "controls" {
		if m.controlReview && len(m.controlForm.Fields) > 0 {
			return "Edit review details"
		}
		return "Cancel changes"
	}
	if m.section == "overview" || m.controlFamily == "overview" && m.section == "controls" && len(m.controlRootArgs) > 0 {
		return "Back to overview"
	}
	if m.section == "inbox" && m.inboxDetail != "" {
		return "Back to Inbox"
	}
	if m.section == "history" && m.historyDetail != nil {
		return "Back to task runs"
	}
	if m.section == "learnings" {
		if m.learningForm != nil {
			return "Cancel guidance changes"
		}
		if m.learningDetail != nil {
			return "Back to saved guidance"
		}
	}
	if m.section == "context" && m.contextDetail != nil {
		if m.contextEditing {
			return "Cancel correction"
		}
		if m.contextConfirm {
			return "Cancel forgetting"
		}
		if m.contextWorkID != "" {
			return "Back to context and sources"
		}
		return "Back to saved knowledge"
	}
	if m.section == "followup" {
		if m.followupForm != nil {
			if m.followupForm.Review {
				return "Edit agreement"
			}
			return "Cancel check setup"
		}
		if m.checkDetail != nil {
			return "Back to follow-up checks"
		}
	}
	return "Back to conversation"
}

func (m Model) bridgeLauncherPanel() uisnapshot.Panel {
	if m.launcherMode == "status-line" {
		panel := uisnapshot.Panel{Kind: "status-line", Title: "Terminal status line", Selected: m.selected,
			Footer: "V show or hide · Space toggle · ←/→ reorder · R defaults · Enter save · Esc cancel"}
		panel.Toggle = &uisnapshot.Toggle{Label: "Show status line", Checked: m.statusLineDraftEnabled}
		panel.Hints = []uisnapshot.Hint{{Shortcut: "V", Action: "show/hide"}, {Shortcut: "Space", Action: "toggle"}, {Shortcut: "←/→", Action: "reorder"}, {Shortcut: "R", Action: "defaults"}, {Shortcut: "Enter", Action: "save"}, {Shortcut: "Esc", Action: "cancel"}}
		if m.statusLineDraftEnabled {
			panel.Subtitle = "Status line shown. Choose items; selected order is preserved."
		} else {
			panel.Subtitle = "Status line hidden. Choose items; selected order is preserved."
		}
		for _, option := range statusLineOptions {
			position := selectedStatusLineIndex(m.statusLineDraft, option.id)
			state := "off"
			if position >= 0 {
				state = fmt.Sprintf("%d", position+1)
			}
			panel.Rows = append(panel.Rows, uisnapshot.Row{Label: option.label, Detail: option.description, State: state, Checked: position >= 0, Position: position + 1})
		}
		if preview := m.statusLineDraftValues(); m.statusLineDraftEnabled && len(preview) > 0 {
			panel.Blocks = []uisnapshot.Block{{Title: "Preview", Lines: preview}}
		}
		return panel
	}
	panel := uisnapshot.Panel{Kind: "picker", Title: "Commands", Selected: m.selected,
		Query: m.paletteQuery, Footer: "Type to filter · ↑/↓ choose · Enter select · Esc close"}
	panel.Hints = []uisnapshot.Hint{{Shortcut: "↑/↓", Action: "choose"}, {Shortcut: "Enter", Action: "select"}, {Shortcut: "Esc", Action: "close"}}
	switch m.launcherMode {
	case "conversations":
		panel.Title = "Conversations"
	case "goals":
		panel.Title = "Ongoing goals"
	case "source":
		panel.Title = "Workspace"
	case "copy":
		panel.Title = "Copy"
	}
	for _, item := range m.filteredEntries() {
		row := uisnapshot.Row{Label: item.title, Detail: item.subtitle}
		if item.kind == "conversation" {
			row.Source = item.source
			for _, conversation := range m.config.Conversations {
				if conversation.ID == item.id {
					row.UpdatedAt = conversation.UpdatedAt.Format(time.RFC3339)
				}
			}
		} else if item.kind == "goal" {
			for _, goal := range m.config.Goals {
				if goal.ID == item.id {
					row.Source = goal.SourcePath
				}
			}
			row.Metadata = []uisnapshot.Field{{Label: "Follow-up", Value: strings.TrimSuffix(item.subtitle, " · /followup new")}}
		} else if m.launcherMode == "source" && item.command == "/source" {
			row.Source = m.source
		}
		panel.Rows = append(panel.Rows, row)
	}
	return panel
}

func (m Model) bridgeSectionPanel() uisnapshot.Panel {
	panel := uisnapshot.Panel{Kind: m.section, Selected: m.selected, Notice: m.sectionNotice, Footer: "Esc back"}
	switch m.section {
	case "controls":
		return m.controlsPanel()
	case "followup":
		return m.followupPanel()
	case "memory":
		return m.recallPanel()
	case "context":
		return m.contextPanel()
	case "appearance":
		return m.appearancePanel()
	case "effort":
		panel.Title = "Choose effort"
		panel.Subtitle = "How much time Gator may spend on a request."
		panel.Footer = "↑/↓ choose · Enter set effort · Esc back"
		panel.Hints = []uisnapshot.Hint{{Shortcut: "↑/↓", Action: "choose"}, {Shortcut: "Enter", Action: "set"}, {Shortcut: "Esc", Action: "back"}}
		for index, name := range []string{"Low", "Standard", "High"} {
			state := ""
			if index == effortIndex(m.options.MaxSteps) {
				state = "current"
			}
			panel.Rows = append(panel.Rows, uisnapshot.Row{Label: name, Detail: []string{"Quick requests · up to 12 steps", "Everyday work · up to 24 steps", "Complex work · up to 48 steps"}[index], State: state})
		}
	case "inbox":
		if m.inboxDetail != "" {
			return m.inboxDetailPanel()
		}
		panel.Title = "Inbox"
		panel.Hints = []uisnapshot.Hint{{Shortcut: "Esc", Action: "back"}, {Shortcut: "Enter", Action: "open"}, {Shortcut: "R", Action: "refresh"}}
		for _, item := range m.config.Inbox {
			state := "unread"
			if !item.ReadAt.IsZero() {
				state = "read"
			}
			if item.Urgent {
				state += " · urgent"
			}
			if item.Rating != "" {
				state += " · " + item.Rating
			}
			tone := "muted"
			if item.Urgent {
				tone = "warning"
			} else if item.ReadAt.IsZero() {
				tone = "success"
			}
			panel.Rows = append(panel.Rows, uisnapshot.Row{Label: safeInboxText(item.Title),
				Detail: safeInboxText(item.Summary), State: safeInboxText(item.Status) + " · " + item.ID + " · " + state, StateLabel: state, Tone: tone, UpdatedAt: item.CreatedAt.Format(time.RFC3339), Metadata: []uisnapshot.Field{{Label: "Result status", Value: safeInboxText(item.Status)}}})
		}
		if len(panel.Rows) == 0 {
			panel.Notice = "No recent results."
		}
	case "jobs":
		panel.Title = "Scheduled jobs"
		panel.Footer = "Esc back · /jobs add to create a recurring check"
		panel.Hints = []uisnapshot.Hint{{Shortcut: "Esc", Action: "back"}, {Shortcut: "/jobs add", Action: "create"}}
		for _, job := range m.config.Jobs {
			state := "disabled"
			if job.Enabled {
				state = "enabled"
			}
			tone := "muted"
			if job.Enabled {
				tone = "success"
			}
			panel.Rows = append(panel.Rows, uisnapshot.Row{Label: job.Name, Detail: job.Schedule + " · " + job.Timezone, State: state, Tone: tone})
		}
		if len(panel.Rows) == 0 {
			panel.Notice = "No scheduled jobs. Create one with /jobs add."
		}
	case "history":
		return m.bridgeHistoryPanel()
	case "learnings":
		return m.bridgeLearningsPanel()
	}
	return panel
}

func (m Model) bridgeHistoryPanel() uisnapshot.Panel {
	panel := uisnapshot.Panel{Kind: "history", Title: "History", Subtitle: historyFilterName(m.historyFilter),
		Selected: m.selected, Notice: m.sectionNotice, Footer: "↑/↓ select · Enter details · F filter · R refresh · Esc back"}
	panel.Hints = []uisnapshot.Hint{{Shortcut: "↑/↓", Action: "select"}, {Shortcut: "Enter", Action: "details"}, {Shortcut: "F", Action: "filter"}, {Shortcut: "Esc", Action: "back"}}
	if detail := m.historyDetail; detail != nil {
		panel.Title = "History detail"
		panel.Subtitle = detail.Record.ObjectiveSummary
		panel.Rows = nil
		panel.Summary = []uisnapshot.Field{{Label: "Task status", Value: detail.Record.DisplayStatus()}, {Label: "File verification", Value: valueOrNone(string(detail.Record.VerificationStatus))}, {Label: "Delivery", Value: detail.Delivery}}
		panel.Blocks = []uisnapshot.Block{
			{Title: "Run details", Collapsible: true, Lines: []string{historyStatusLine(detail.Record, detail.Project, detail.Delivery), "Source: " + valueOrNone(detail.Project), "Snapshot: " + valueOrNone(detail.Record.SnapshotID), "Verification: " + valueOrNone(string(detail.Record.VerificationStatus)) + " · artifact " + valueOrNone(detail.Record.ArtifactStatus)}},
			{Title: "Artifacts", Lines: historyArtifactLines(detail.Artifacts, detail.ArtifactIssue)},
			{Title: "Delivery", Lines: formatHistoryDeliveries(detail.Deliveries)},
			{Title: "Feedback", Collapsible: true, Lines: historyFeedbackLines(detail.Observations)},
			{Title: "Derived learnings", Collapsible: true, Lines: historyLearningLines(detail.Learnings)},
		}
		if decision := detail.Record.Decision; decision != nil {
			lines := []string{"Purpose: " + decision.Intent + " · " + string(decision.Mode), "Chosen by: " + decision.Origin, "Connected sources: " + valueOrNone(strings.Join(decision.Connectors, ", ")), "Recall: " + valueOrNone(decision.RecallStatus)}
			if decision.GoalID != "" {
				lines = append(lines, "Goal: "+decision.GoalID)
			}
			if len(decision.ContextRefs) > 0 {
				lines = append(lines, "Saved context supplied: "+strings.Join(decision.ContextRefs, ", "))
			}
			if decision.Reason != "" {
				lines = append(lines, decision.Reason)
			}
			panel.Blocks = append(panel.Blocks, uisnapshot.Block{Title: "Context and sources", Collapsible: true, Lines: append([]string{"Run: " + detail.Record.ID}, lines...)})
		}
		if detail.Review != "" {
			panel.Blocks = append(panel.Blocks, uisnapshot.Block{Title: "Review", Lines: []string{detail.Review}})
		}
		controls := []string{"Esc back", "R review"}
		if m.historyRetryAvailable() {
			controls = append(controls, "T retry delivery")
		}
		if detail.Record.ConversationID != "" {
			controls = append(controls, "C open conversation")
		}
		panel.Footer = strings.Join(controls, " · ")
		panel.Hints = []uisnapshot.Hint{{Shortcut: "Esc", Action: "back"}, {Shortcut: "R", Action: "review"}}
		if m.historyRetryAvailable() {
			panel.Hints = append(panel.Hints, uisnapshot.Hint{Shortcut: "T", Action: "retry"})
		}
		if detail.Record.ConversationID != "" {
			panel.Hints = append(panel.Hints, uisnapshot.Hint{Shortcut: "C", Action: "conversation"})
		}
		return panel
	}
	panel.Filter = &uisnapshot.Filter{Value: historyFilterName(m.historyFilter), Options: []uisnapshot.Option{{Value: "all", Label: "All task runs"}, {Value: "running", Label: "Running"}, {Value: "completed", Label: "Completed"}, {Value: "failed", Label: "Failed"}}}
	for _, item := range m.historyItems {
		at := item.Record.FinishedAt
		if at.IsZero() {
			at = item.Record.StartedAt
		}
		tone := "muted"
		if item.Record.Status == "failed" {
			tone = "error"
		}
		panel.Rows = append(panel.Rows, uisnapshot.Row{Label: item.Record.ObjectiveSummary, Detail: historyStatusLine(item.Record, item.Project, item.Delivery),
			Source: item.Project, UpdatedAt: at.Format(time.RFC3339), State: item.Record.DisplayStatus(), Tone: tone,
			Metadata: []uisnapshot.Field{{Label: "Mode", Value: string(item.Record.Mode)}}})
	}
	if len(panel.Rows) == 0 && panel.Notice == "" {
		panel.Notice = "No retained Work matches this view yet."
	}
	return panel
}

func (m Model) bridgeLearningsPanel() uisnapshot.Panel {
	panel := uisnapshot.Panel{Kind: "learnings", Title: "Saved guidance", Subtitle: learningFilterName(m.learningFilter),
		Selected: m.selected, Notice: m.sectionNotice, Footer: "↑/↓ select · Enter details · N add · F filter · R refresh · Esc back"}
	panel.Hints = []uisnapshot.Hint{{Shortcut: "↑/↓", Action: "select"}, {Shortcut: "Enter", Action: "details"}, {Shortcut: "N", Action: "add"}, {Shortcut: "Esc", Action: "back"}}
	if form := m.learningForm; form != nil {
		panel.Kind = "learning-form"
		panel.Title = "Add saved guidance"
		panel.Subtitle = "Preferences, facts, and procedures for future work."
		if form.EditingID != "" {
			panel.Title = "Edit saved guidance"
		}
		panel.Rows = []uisnapshot.Row{
			{Label: "Type", Detail: string(form.Type), Input: bridgeSelect(string(form.Type),
				uisnapshot.Option{Value: string(learning.Preference), Label: "Preference"},
				uisnapshot.Option{Value: string(learning.EnvironmentFact), Label: "Environment fact"},
				uisnapshot.Option{Value: string(learning.Procedure), Label: "Procedure"},
				uisnapshot.Option{Value: string(learning.FailurePrevention), Label: "Failure prevention"})},
			{Label: "Scope", Detail: learningScopeText(form.Scope), Input: bridgeSelect(string(form.Scope.Kind),
				uisnapshot.Option{Value: string(learning.Global), Label: "All workspaces"},
				uisnapshot.Option{Value: string(learning.Project), Label: "This workspace"})},
			{Label: "Key", Detail: valueOrNone(form.Key), Input: bridgeInput(form.Key, 4096, false)},
			{Label: "Text", Detail: valueOrNone(form.Content), Input: bridgeInput(form.Content, 4096, true)},
		}
		panel.Rows[0].Input.ReadOnly, panel.Rows[1].Input.ReadOnly = form.EditingID != "", form.EditingID != ""
		if form.EditingID != "" {
			panel.Rows[0].Input.Help = "Type and scope are retained when editing existing guidance."
		}
		panel.Selected = form.Field
		panel.Footer = "Tab next field · Enter continue or save · Esc cancel"
		panel.Hints = []uisnapshot.Hint{{Shortcut: "Tab", Action: "next field"}, {Shortcut: "Enter", Action: "save"}, {Shortcut: "Esc", Action: "cancel"}}
		return panel
	}
	if record := m.learningDetail; record != nil {
		panel.Title = "Guidance details"
		panel.Subtitle = "Saved guidance for future work"
		panel.Summary = []uisnapshot.Field{{Label: "Status", Value: string(record.Status)}, {Label: "Type", Value: strings.ReplaceAll(string(record.Type), "_", " ")}, {Label: "Scope", Value: learningScopeShort(record.Scope)}}
		panel.Blocks = []uisnapshot.Block{
			{Title: "Guidance", Role: "content", Lines: []string{record.Content}},
			{Title: "Record details", Collapsible: true, Fields: []uisnapshot.Field{{Label: "Origin", Value: string(record.Origin)}, {Label: "Scope", Value: learningScopeText(record.Scope)}, {Label: "Key", Value: record.Key}, {Label: "Created", Value: record.CreatedAt.Local().Format("2006-01-02 15:04")}}},
			{Title: "Derived from Work", Collapsible: true, Lines: compactLines(record.Provenance.WorkIDs, 3)},
			{Title: "Evidence", Collapsible: true, Lines: compactLines(record.Provenance.EvidenceRefs, 3)},
		}
		if record.Origin == learning.Inferred {
			panel.Blocks = append(panel.Blocks, uisnapshot.Block{Title: "Confidence", Collapsible: true, Lines: []string{fmt.Sprint(record.Confidence)}})
		}
		if !record.Provenance.UserConfirmedAt.IsZero() {
			panel.Blocks = append(panel.Blocks, uisnapshot.Block{Title: "Confirmed", Collapsible: true, Lines: []string{record.Provenance.UserConfirmedAt.Local().Format("2006-01-02 15:04")}})
		}
		controls := []string{"Esc back", "N add"}
		switch record.Status {
		case learning.Candidate:
			controls = append(controls, "A approve", "R reject")
		case learning.Active:
			controls = append(controls, "D disable")
		case learning.Disabled:
			controls = append(controls, "A enable")
		}
		if record.Origin == learning.UserAuthored {
			controls = append(controls, "E edit")
		}
		panel.Footer = strings.Join(controls, " · ")
		panel.Hints = []uisnapshot.Hint{{Shortcut: "Esc", Action: "back"}, {Shortcut: "N", Action: "add"}}
		switch record.Status {
		case learning.Candidate:
			panel.Hints = append(panel.Hints, uisnapshot.Hint{Shortcut: "A", Action: "approve"}, uisnapshot.Hint{Shortcut: "R", Action: "reject"})
		case learning.Active:
			panel.Hints = append(panel.Hints, uisnapshot.Hint{Shortcut: "D", Action: "disable"})
		case learning.Disabled:
			panel.Hints = append(panel.Hints, uisnapshot.Hint{Shortcut: "A", Action: "enable"})
		}
		if record.Origin == learning.UserAuthored {
			panel.Hints = append(panel.Hints, uisnapshot.Hint{Shortcut: "E", Action: "edit"})
		}
		return panel
	}
	panel.Filter = &uisnapshot.Filter{Value: learningFilterName(m.learningFilter), Options: []uisnapshot.Option{{Value: "all", Label: "All guidance"}, {Value: "active", Label: "Active"}, {Value: "candidate", Label: "Awaiting review"}, {Value: "disabled", Label: "Disabled"}, {Value: "rejected", Label: "Rejected"}}}
	for _, item := range m.learningItems {
		detail := string(item.Status) + " · " + string(item.Type) + " · " + learningScopeShort(item.Scope)
		if item.Origin == learning.Inferred {
			detail += fmt.Sprintf(" · confidence %d", item.Confidence)
		}
		panel.Rows = append(panel.Rows, uisnapshot.Row{Label: singleLine(item.Content), Detail: detail, State: string(item.Status), UpdatedAt: item.CreatedAt.Format(time.RFC3339),
			Metadata: []uisnapshot.Field{{Label: "Type", Value: strings.ReplaceAll(string(item.Type), "_", " ")}, {Label: "Scope", Value: learningScopeShort(item.Scope)}}})
	}
	if len(panel.Rows) == 0 && panel.Notice == "" {
		panel.Notice = "No learnings match this view. Press N to add a preference."
	}
	return panel
}

func (m Model) withPanelHelp(panel uisnapshot.Panel) uisnapshot.Panel {
	if !m.productHelp {
		return panel
	}
	lines := []string{}
	for _, hint := range panel.Hints {
		lines = append(lines, hint.Shortcut+" · "+hint.Action)
	}
	return uisnapshot.Panel{Kind: "help", Title: panel.Title + " · shortcuts", Blocks: []uisnapshot.Block{{Lines: lines}}, Hints: []uisnapshot.Hint{{Shortcut: "F1/Esc", Action: "back"}}}
}
