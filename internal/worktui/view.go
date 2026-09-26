package worktui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/rattles"
)

func (m Model) View() string {
	width := m.width
	if width <= 0 {
		width = 80
	} else if width < 20 {
		width = 20
	}
	height := m.height
	if height <= 0 {
		height = 24
	} else if height < 8 {
		height = 8
	}
	if m.models != nil {
		return m.models.View()
	}
	accent, dim, selectedStyle := workStyles(m.theme)
	if m.launcher {
		if m.launcherMode == "status-line" {
			return m.renderViewport(m.renderStatusLineEditor(width, height, accent, dim, selectedStyle), width, height)
		}
		return m.renderViewport(m.renderPalette(width, height, accent, dim, selectedStyle), width, height)
	}
	if m.section != "" {
		return m.renderViewport(m.renderSection(width, height, accent, dim, selectedStyle), width, height)
	}
	if m.home {
		return m.renderViewport(m.renderHome(width, height, accent), width, height)
	}
	var view strings.Builder
	view.WriteString(accent.Render(gatorWordmark))
	if m.title != "" {
		view.WriteString(dim.Render("  " + m.title))
	}
	if m.status != "" {
		view.WriteString(dim.Render(" · " + m.status))
	}
	view.WriteString("\n\n")
	var transcript strings.Builder
	messageStyle := lipgloss.NewStyle().Width(max(20, width-4))
	for _, item := range m.messages {
		if item.role == "Deliverables" || item.bundle != nil {
			contents := item.text
			if item.bundle != nil {
				contents = formatBundleSummary(*item.bundle)
			}
			card := lipgloss.NewStyle().
				Width(max(16, width-8)).
				Padding(0, 1).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("238")).
				Render(accent.Render("Verified deliverables") + "\n" + contents)
			transcript.WriteString(card)
		} else {
			transcript.WriteString(messageStyle.Render(accent.Render(item.role+":") + " " + item.text))
		}
		transcript.WriteString("\n\n")
	}
	if m.pendingBundleAction != nil {
		confirmation := accent.Render("Review transfer") + "\n" + m.pendingBundleAction.summary +
			"\n\n" + dim.Render("enter/y confirm · esc/n cancel")
		transcript.WriteString(lipgloss.NewStyle().
			Width(max(16, width-8)).
			Padding(0, 1).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("214")).
			Render(confirmation))
		transcript.WriteString("\n\n")
	}
	footer := ""
	if m.statusLineEnabled {
		footer = wrapStatusLine(m.statusLineItems(), width)
	}
	composer := m.renderComposer(width, !m.running)
	composerRows := strings.Count(composer, "\n") + 1
	// The composer is always anchored to the bottom of the viewport. History
	// fills the available space above it instead of moving the input around.
	composerTop := max(2, height-composerRows)
	workingRows := 0
	if m.running {
		workingRows = 2
	}
	lines := strings.Split(strings.TrimSuffix(transcript.String(), "\n"), "\n")
	headerRows := strings.Count(view.String(), "\n")
	footerRows := 0
	if footer != "" {
		footerRows = strings.Count(footer, "\n") + 1
	}
	footerTop := max(headerRows, composerTop-footerRows)
	aboveComposer := max(0, footerTop-headerRows-workingRows)
	available := aboveComposer
	maxScroll := max(0, len(lines)-available)
	scroll := min(m.scroll, maxScroll)
	end := len(lines) - scroll
	start := max(0, end-available)
	visible := lines[start:end]
	for len(visible) > 0 && visible[0] == "" {
		visible = visible[1:]
	}
	if len(visible) > 0 {
		view.WriteString(strings.Join(visible, "\n") + "\n")
	}
	if m.running {
		working := rattles.BrailleDots.Frame(m.loadingFrame) + " Working…"
		if elapsed := m.workElapsed(); elapsed != "" {
			working += " " + elapsed + " · esc to interrupt"
		}
		view.WriteString(accent.Render(working) + "\n\n")
	}
	if currentRow := strings.Count(view.String(), "\n"); currentRow < footerTop {
		view.WriteString(strings.Repeat("\n", footerTop-currentRow))
	}
	if footer != "" {
		view.WriteString(dim.Render(footer) + "\n")
	}
	view.WriteString(composer)
	return m.renderViewport(view.String(), width, height)
}

// renderViewport occupies every terminal cell so each render clears the prior
// frame, but deliberately leaves the background unset. This lets a terminal
// emulator's configured background and transparency show through consistently.
func (m Model) renderViewport(contents string, width, height int) string {
	return lipgloss.NewStyle().Width(width).Height(height).Render(contents)
}

func (m Model) renderHome(width, height int, accent lipgloss.Style) string {
	title := accent.Copy().Bold(true).Render(gatorWordmark)
	composer := m.renderComposer(width, true)
	composerRows := strings.Count(composer, "\n") + 1
	composerTop := max(1, height-composerRows)
	var view strings.Builder
	view.WriteString(title + "\n")
	if currentRow := strings.Count(view.String(), "\n"); currentRow < composerTop {
		view.WriteString(strings.Repeat("\n", composerTop-currentRow))
	}
	view.WriteString(lipgloss.Place(width, composerRows, lipgloss.Center, lipgloss.Top, composer))
	return view.String()
}

func (m Model) renderPalette(width, height int, accent, dim, selectedStyle lipgloss.Style) string {
	panelWidth := min(76, max(16, width-4))
	titleWidth := min(28, max(8, panelWidth/2-1))
	subtitleWidth := max(0, panelWidth-titleWidth-3)
	visible := m.filteredEntries()
	selected := min(m.selected, max(0, len(visible)-1))
	maximum := max(1, height-9)
	start := 0
	if selected >= maximum {
		start = selected - maximum + 1
	}
	end := min(len(visible), start+maximum)
	var panel strings.Builder
	title := "Commands"
	if m.launcherMode == "conversations" {
		title = "Conversations"
	} else if m.launcherMode == "source" {
		title = "Source"
	} else if m.launcherMode == "copy" {
		title = "Copy"
	}
	panel.WriteString(accent.Render(title) + "\n")
	query := m.paletteQuery
	if query == "" {
		query = dim.Render("type to filter")
	}
	panel.WriteString("Search  " + query + accent.Render("█") + "\n\n")
	if len(visible) == 0 {
		empty := "No matching commands."
		if m.launcherMode == "conversations" && m.paletteQuery == "" {
			empty = "No retained conversations yet."
		} else if m.launcherMode != "commands" {
			empty = "No matching items."
		}
		panel.WriteString(dim.Render(empty) + "\n")
	}
	rowStyle := lipgloss.NewStyle().Width(panelWidth)
	for index := start; index < end; index++ {
		item := visible[index]
		title := truncate(item.title, titleWidth)
		subtitle := truncate(item.subtitle, subtitleWidth)
		line := rowStyle.Render(fmt.Sprintf("  %-*s %s", titleWidth, title, dim.Render(subtitle)))
		if index == selected {
			line = selectedStyle.Width(panelWidth).Render("› " + fmt.Sprintf("%-*s %s", titleWidth, title, subtitle))
		}
		panel.WriteString(line + "\n")
	}
	panel.WriteString("\n" + dim.Render("type filter  ·  ↑/↓ choose  ·  enter run  ·  esc close"))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel.String())
}

func (m Model) renderSection(width, height int, accent, dim, selectedStyle lipgloss.Style) string {
	switch m.section {
	case "history":
		return m.renderHistorySection(width, height, accent, dim, selectedStyle)
	case "learnings":
		return m.renderLearningsSection(width, height, accent, dim, selectedStyle)
	}
	var view strings.Builder
	title := "Inbox"
	if m.section == "jobs" {
		title = "Scheduled jobs"
	}
	view.WriteString(accent.Render(gatorWordmark) + dim.Render("  "+title) + "\n\n")
	messageStyle := lipgloss.NewStyle().Width(max(20, width-4))
	if m.section == "inbox" {
		if len(m.config.Inbox) == 0 {
			view.WriteString(dim.Render("No recent results.") + "\n")
		}
		for _, item := range m.config.Inbox {
			view.WriteString(messageStyle.Render(accent.Render(item.Status+":") + " " + item.Title + "\n" + item.Summary))
			view.WriteString("\n\n")
		}
	} else {
		if len(m.config.Jobs) == 0 {
			view.WriteString(dim.Render("No scheduled jobs. Create one with `/jobs add …`.") + "\n")
		}
		for _, job := range m.config.Jobs {
			state := "disabled"
			if job.Enabled {
				state = "enabled"
			}
			view.WriteString(messageStyle.Render(accent.Render(state+":") + " " + job.Name + "\n" + job.Schedule + " · " + job.Timezone))
			view.WriteString("\n\n")
		}
	}
	footer := wrapStatusLine([]string{"esc back", "ctrl+i workspace", "ctrl+p commands", "ctrl+x conversations", "ctrl+b inbox", "ctrl+j jobs"}, width)
	view.WriteString("\n" + dim.Render(footer))
	return view.String()
}

func (m Model) renderHistorySection(width, height int, accent, dim, selectedStyle lipgloss.Style) string {
	if m.historyDetail != nil {
		return m.renderHistoryDetail(width, accent, dim)
	}
	var view strings.Builder
	view.WriteString(accent.Render(gatorWordmark) + dim.Render("  History · "+historyFilterName(m.historyFilter)) + "\n\n")
	if m.sectionNotice != "" {
		view.WriteString(dim.Render(m.sectionNotice) + "\n\n")
	}
	if len(m.historyItems) == 0 && m.sectionNotice == "" {
		view.WriteString(dim.Render("No retained Work matches this view yet.") + "\n")
	}
	maximum := max(1, (height-7)/2)
	start, end := sectionWindow(len(m.historyItems), m.selected, maximum)
	rowStyle := lipgloss.NewStyle().Width(max(20, width-4))
	for index := start; index < end; index++ {
		item := m.historyItems[index]
		row := accent.Render(truncate(item.Record.ObjectiveSummary, max(16, width-8))) + "\n" + dim.Render(historyStatusLine(item.Record, item.Project, item.Delivery))
		if index == m.selected {
			view.WriteString(selectedStyle.Width(max(20, width-4)).Render("› "+row) + "\n")
		} else {
			view.WriteString(rowStyle.Render("  "+row) + "\n")
		}
	}
	view.WriteString("\n" + dim.Render("↑/↓ select · enter details · f status filter · r refresh · esc back"))
	return view.String()
}

func (m Model) renderHistoryDetail(width int, accent, dim lipgloss.Style) string {
	detail := m.historyDetail
	var view strings.Builder
	view.WriteString(accent.Render(gatorWordmark) + dim.Render("  History detail") + "\n\n")
	view.WriteString(accent.Render(detail.Record.ObjectiveSummary) + "\n")
	view.WriteString(dim.Render(historyStatusLine(detail.Record, detail.Project, detail.Delivery)) + "\n\n")
	view.WriteString("Source: " + valueOrNone(detail.Project) + "\n")
	view.WriteString("Snapshot: " + valueOrNone(detail.Record.SnapshotID) + "\n")
	view.WriteString("Verification: " + valueOrNone(string(detail.Record.VerificationStatus)) + " · artifact " + valueOrNone(detail.Record.ArtifactStatus) + "\n")
	writeSectionItems(&view, "Artifacts", historyArtifactLines(detail.Artifacts, detail.ArtifactIssue), dim)
	writeSectionItems(&view, "Delivery", formatHistoryDeliveries(detail.Deliveries), dim)
	writeSectionItems(&view, "Feedback", historyFeedbackLines(detail.Observations), dim)
	writeSectionItems(&view, "Derived learnings", historyLearningLines(detail.Learnings), dim)
	if detail.Review != "" {
		writeSectionItems(&view, "Review", []string{detail.Review}, dim)
	}
	if m.sectionNotice != "" {
		view.WriteString("\n" + dim.Render(m.sectionNotice) + "\n")
	}
	controls := "esc back · r review retained output"
	if m.historyRetryAvailable() {
		controls += " · t retry delivery"
	}
	if detail.Record.ConversationID != "" {
		controls += " · c open conversation"
	}
	view.WriteString("\n" + dim.Render(controls))
	return view.String()
}

func historyArtifactLines(items []ArtifactSummary, issue string) []string {
	lines := make([]string, 0, min(3, len(items))+1)
	for _, item := range items[:min(3, len(items))] {
		mark := "✓"
		if !item.Valid {
			mark = "!"
		}
		lines = append(lines, fmt.Sprintf("%s %s · %s · %d bytes", mark, item.Path, item.MediaType, item.Bytes))
	}
	if len(items) > 3 {
		lines = append(lines, fmt.Sprintf("+%d more artifacts", len(items)-3))
	}
	if issue != "" {
		lines = append(lines, issue)
	}
	return lines
}

func historyFeedbackLines(items []learning.Observation) []string {
	lines := make([]string, 0, min(3, len(items)))
	for _, item := range items[:min(3, len(items))] {
		line := string(item.Signal)
		if item.Summary != "" {
			line += " · " + singleLine(item.Summary)
		}
		lines = append(lines, line)
	}
	if len(items) > 3 {
		lines = append(lines, fmt.Sprintf("+%d more observations", len(items)-3))
	}
	return lines
}

func historyLearningLines(items []learning.Record) []string {
	lines := make([]string, 0, min(3, len(items)))
	for _, item := range items[:min(3, len(items))] {
		lines = append(lines, string(item.Status)+" · "+singleLine(item.Content))
	}
	if len(items) > 3 {
		lines = append(lines, fmt.Sprintf("+%d more learnings", len(items)-3))
	}
	return lines
}

func (m Model) renderLearningsSection(width, height int, accent, dim, selectedStyle lipgloss.Style) string {
	if m.learningForm != nil {
		return m.renderLearningForm(width, accent, dim)
	}
	if m.learningDetail != nil {
		return m.renderLearningDetail(width, accent, dim)
	}
	var view strings.Builder
	view.WriteString(accent.Render(gatorWordmark) + dim.Render("  Learnings · "+learningFilterName(m.learningFilter)) + "\n\n")
	if m.sectionNotice != "" {
		view.WriteString(dim.Render(m.sectionNotice) + "\n\n")
	}
	if len(m.learningItems) == 0 && m.sectionNotice == "" {
		view.WriteString(dim.Render("No learnings match this view. Press n to add an explicit preference.") + "\n")
	}
	maximum := max(1, height-7)
	start, end := sectionWindow(len(m.learningItems), m.selected, maximum)
	rowStyle := lipgloss.NewStyle().Width(max(20, width-4))
	for index := start; index < end; index++ {
		item := m.learningItems[index]
		confidence := ""
		if item.Origin == learning.Inferred {
			confidence = fmt.Sprintf(" · confidence %d", item.Confidence)
		}
		row := truncate(singleLine(item.Content), max(16, width-30)) + "\n" + dim.Render(string(item.Status)+" · "+string(item.Type)+" · "+learningScopeShort(item.Scope)+" · "+string(item.Origin)+confidence)
		if index == m.selected {
			view.WriteString(selectedStyle.Width(max(20, width-4)).Render("› "+row) + "\n")
		} else {
			view.WriteString(rowStyle.Render("  "+row) + "\n")
		}
	}
	view.WriteString("\n" + dim.Render("↑/↓ select · enter details · n add · f status filter · r refresh · esc back"))
	return view.String()
}

func (m Model) renderLearningDetail(width int, accent, dim lipgloss.Style) string {
	record := m.learningDetail
	var view strings.Builder
	view.WriteString(accent.Render(gatorWordmark) + dim.Render("  Learning detail") + "\n\n")
	view.WriteString(accent.Render(singleLine(record.Content)) + "\n\n")
	view.WriteString("Status: " + string(record.Status) + " · " + string(record.Origin) + "\n")
	view.WriteString("Type: " + string(record.Type) + " · scope: " + learningScopeText(record.Scope) + "\n")
	view.WriteString("Key: " + record.Key + "\n")
	if record.Origin == learning.Inferred {
		view.WriteString(fmt.Sprintf("Confidence: %d\n", record.Confidence))
	}
	view.WriteString("Created: " + record.CreatedAt.Local().Format("2006-01-02 15:04") + "\n")
	if !record.Provenance.UserConfirmedAt.IsZero() {
		view.WriteString("Confirmed: " + record.Provenance.UserConfirmedAt.Local().Format("2006-01-02 15:04") + "\n")
	}
	writeSectionItems(&view, "Derived from Work", compactLines(record.Provenance.WorkIDs, 3), dim)
	writeSectionItems(&view, "Evidence", compactLines(record.Provenance.EvidenceRefs, 3), dim)
	if m.sectionNotice != "" {
		view.WriteString("\n" + dim.Render(m.sectionNotice) + "\n")
	}
	controls := []string{"esc back", "n add"}
	switch record.Status {
	case learning.Candidate:
		controls = append(controls, "a approve", "r reject")
	case learning.Active:
		controls = append(controls, "d disable")
	case learning.Disabled:
		controls = append(controls, "a enable")
	}
	if record.Origin == learning.UserAuthored {
		controls = append(controls, "e edit")
	}
	view.WriteString("\n" + dim.Render(strings.Join(controls, " · ")))
	return view.String()
}

func (m Model) renderLearningForm(width int, accent, dim lipgloss.Style) string {
	form := m.learningForm
	labels := []string{
		"Type: " + string(form.Type) + "  (left/right)",
		"Scope: " + learningScopeText(form.Scope) + "  (left/right)",
		"Key: " + valueOrNone(form.Key),
		"Text: " + valueOrNone(form.Content),
	}
	if form.EditingID != "" {
		labels[0] = "Type: " + string(form.Type) + "  (retained)"
		labels[1] = "Scope: " + learningScopeText(form.Scope) + "  (retained)"
	}
	var view strings.Builder
	title := "Add explicit learning"
	if form.EditingID != "" {
		title = "Edit explicit learning"
	}
	view.WriteString(accent.Render(gatorWordmark) + dim.Render("  "+title) + "\n\n")
	for index, label := range labels {
		if index == form.Field {
			view.WriteString(accent.Render("› "+label) + "\n")
		} else {
			view.WriteString("  " + label + "\n")
		}
	}
	if m.sectionNotice != "" {
		view.WriteString("\n" + dim.Render(m.sectionNotice) + "\n")
	}
	view.WriteString("\n" + dim.Render("tab next field · enter continue/save · esc cancel"))
	return view.String()
}

func writeSectionItems(view *strings.Builder, title string, items []string, dim lipgloss.Style) {
	if len(items) == 0 {
		items = []string{"none"}
	}
	view.WriteString("\n" + title + ":\n")
	for _, item := range items {
		view.WriteString(dim.Render("  "+item) + "\n")
	}
}

func compactLines(items []string, limit int) []string {
	if len(items) == 0 {
		return nil
	}
	result := append([]string(nil), items[:min(limit, len(items))]...)
	if len(items) > limit {
		result = append(result, fmt.Sprintf("+%d more", len(items)-limit))
	}
	return result
}

func sectionWindow(total, selected, maximum int) (int, int) {
	if total == 0 {
		return 0, 0
	}
	maximum = max(1, maximum)
	selected = min(max(0, selected), total-1)
	start := max(0, selected-maximum/2)
	end := min(total, start+maximum)
	start = max(0, end-maximum)
	return start, end
}

func boundedSectionText(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 4000 {
		return value
	}
	return strings.TrimSpace(value[:3997]) + "…"
}

func (m Model) renderComposer(width int, focused bool) string {
	composerWidth := min(68, max(12, width-8))
	border := lipgloss.Color("238")
	cursor := lipgloss.Color("42")
	if m.theme == "contrast" {
		border, cursor = lipgloss.Color("250"), lipgloss.Color("46")
	} else if m.theme == "mono" {
		border, cursor = lipgloss.Color("245"), lipgloss.Color("255")
	}
	if focused {
		border = cursor
	}
	value := m.input
	if value == "" {
		value = lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("Ask Gator to work on something…")
	}
	if focused {
		value += lipgloss.NewStyle().Foreground(cursor).Render("█")
	}
	return lipgloss.NewStyle().Width(composerWidth).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(border).Render(value)
}

func normalizeTheme(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "contrast":
		return "contrast"
	case "mono":
		return "mono"
	default:
		return "gator"
	}
}

func workStyles(theme string) (lipgloss.Style, lipgloss.Style, lipgloss.Style) {
	switch normalizeTheme(theme) {
	case "contrast":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true),
			lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
			lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("226"))
	case "mono":
		return lipgloss.NewStyle().Bold(true), lipgloss.NewStyle().Faint(true), lipgloss.NewStyle().Reverse(true)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true),
			lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
			lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color("236"))
	}
}

func truncate(value string, maximum int) string {
	if maximum <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	if maximum == 1 {
		return "…"
	}
	return string(runes[:maximum-1]) + "…"
}
