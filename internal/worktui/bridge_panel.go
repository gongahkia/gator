package worktui

import (
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/uisnapshot"
)

func (m Model) bridgePanel() *uisnapshot.Panel {
	if m.models != nil {
		panel := m.models.UISnapshot()
		return &panel
	}
	if m.launcher {
		panel := m.bridgeLauncherPanel()
		return &panel
	}
	if m.section != "" {
		panel := m.bridgeSectionPanel()
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

func (m Model) bridgeLauncherPanel() uisnapshot.Panel {
	if m.launcherMode == "status-line" {
		panel := uisnapshot.Panel{Kind: "status-line", Title: "Configure status line", Selected: m.selected,
			Footer: "V show or hide · Space toggle · ←/→ reorder · R defaults · Enter save · Esc cancel"}
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
			panel.Rows = append(panel.Rows, uisnapshot.Row{Label: option.label, Detail: option.description, State: state})
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
		panel.Rows = append(panel.Rows, uisnapshot.Row{Label: item.title, Detail: item.subtitle})
	}
	return panel
}

func (m Model) bridgeSectionPanel() uisnapshot.Panel {
	panel := uisnapshot.Panel{Kind: m.section, Selected: m.selected, Notice: m.sectionNotice, Footer: "Esc back"}
	switch m.section {
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
		panel.Title = "Inbox"
		panel.Footer = "Esc back · /inbox rate ID useful|noise|wrong · /inbox read ID"
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
				Detail: safeInboxText(item.Summary), State: safeInboxText(item.Status) + " · " + item.ID + " · " + state, Tone: tone})
		}
		if len(panel.Rows) == 0 {
			panel.Notice = "No recent results."
		}
	case "jobs":
		panel.Title = "Scheduled jobs"
		panel.Footer = "Esc back · /jobs add to create a recurring check"
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
		panel.Blocks = []uisnapshot.Block{
			{Title: "Work", Lines: []string{historyStatusLine(detail.Record, detail.Project, detail.Delivery), "Source: " + valueOrNone(detail.Project), "Snapshot: " + valueOrNone(detail.Record.SnapshotID), "Verification: " + valueOrNone(string(detail.Record.VerificationStatus)) + " · artifact " + valueOrNone(detail.Record.ArtifactStatus)}},
			{Title: "Artifacts", Lines: historyArtifactLines(detail.Artifacts, detail.ArtifactIssue)},
			{Title: "Delivery", Lines: formatHistoryDeliveries(detail.Deliveries)},
			{Title: "Feedback", Lines: historyFeedbackLines(detail.Observations)},
			{Title: "Derived learnings", Lines: historyLearningLines(detail.Learnings)},
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
	for _, item := range m.historyItems {
		panel.Rows = append(panel.Rows, uisnapshot.Row{Label: item.Record.ObjectiveSummary,
			Detail: historyStatusLine(item.Record, item.Project, item.Delivery)})
	}
	if len(panel.Rows) == 0 && panel.Notice == "" {
		panel.Notice = "No retained Work matches this view yet."
	}
	return panel
}

func (m Model) bridgeLearningsPanel() uisnapshot.Panel {
	panel := uisnapshot.Panel{Kind: "learnings", Title: "Learnings", Subtitle: learningFilterName(m.learningFilter),
		Selected: m.selected, Notice: m.sectionNotice, Footer: "↑/↓ select · Enter details · N add · F filter · R refresh · Esc back"}
	panel.Hints = []uisnapshot.Hint{{Shortcut: "↑/↓", Action: "select"}, {Shortcut: "Enter", Action: "details"}, {Shortcut: "N", Action: "add"}, {Shortcut: "Esc", Action: "back"}}
	if form := m.learningForm; form != nil {
		panel.Kind = "learning-form"
		panel.Title = "Add explicit learning"
		if form.EditingID != "" {
			panel.Title = "Edit explicit learning"
		}
		panel.Rows = []uisnapshot.Row{
			{Label: "Type", Detail: string(form.Type)},
			{Label: "Scope", Detail: learningScopeText(form.Scope)},
			{Label: "Key", Detail: valueOrNone(form.Key)},
			{Label: "Text", Detail: valueOrNone(form.Content)},
		}
		panel.Selected = form.Field
		panel.Footer = "Tab next field · Enter continue or save · Esc cancel"
		panel.Hints = []uisnapshot.Hint{{Shortcut: "Tab", Action: "next field"}, {Shortcut: "Enter", Action: "save"}, {Shortcut: "Esc", Action: "cancel"}}
		return panel
	}
	if record := m.learningDetail; record != nil {
		panel.Title = "Learning detail"
		panel.Subtitle = singleLine(record.Content)
		panel.Blocks = []uisnapshot.Block{
			{Title: "Status", Lines: []string{string(record.Status) + " · " + string(record.Origin)}},
			{Title: "Type and scope", Lines: []string{string(record.Type) + " · " + learningScopeText(record.Scope)}},
			{Title: "Key", Lines: []string{record.Key}},
			{Title: "Created", Lines: []string{record.CreatedAt.Local().Format("2006-01-02 15:04")}},
			{Title: "Derived from Work", Lines: compactLines(record.Provenance.WorkIDs, 3)},
			{Title: "Evidence", Lines: compactLines(record.Provenance.EvidenceRefs, 3)},
		}
		if record.Origin == learning.Inferred {
			panel.Blocks = append(panel.Blocks, uisnapshot.Block{Title: "Confidence", Lines: []string{fmt.Sprint(record.Confidence)}})
		}
		if !record.Provenance.UserConfirmedAt.IsZero() {
			panel.Blocks = append(panel.Blocks, uisnapshot.Block{Title: "Confirmed", Lines: []string{record.Provenance.UserConfirmedAt.Local().Format("2006-01-02 15:04")}})
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
	for _, item := range m.learningItems {
		detail := string(item.Status) + " · " + string(item.Type) + " · " + learningScopeShort(item.Scope)
		if item.Origin == learning.Inferred {
			detail += fmt.Sprintf(" · confidence %d", item.Confidence)
		}
		panel.Rows = append(panel.Rows, uisnapshot.Row{Label: singleLine(item.Content), Detail: detail})
	}
	if len(panel.Rows) == 0 && panel.Notice == "" {
		panel.Notice = "No learnings match this view. Press N to add a preference."
	}
	return panel
}
