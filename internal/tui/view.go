package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	gatorrun "github.com/gongahkia/gator/internal/run"
)

func (m Model) preflightView() string {
	label := "Before starting"
	ready := "Ready to create an isolated worktree."
	readiness := "Run readiness"
	if m.runMode == gatorrun.PlanMode {
		label = "Before planning"
		ready = "Ready to inspect an isolated worktree without edits."
		readiness = "Plan readiness"
	}
	if m.resumeStatePath != "" {
		label = "Before continuing"
		ready = "Ready to continue the retained worktree."
		if m.runMode == gatorrun.PlanMode {
			label = "Before planning"
			ready = "Ready to inspect the retained worktree without edits."
		}
	}
	if len(m.preflight) == 0 {
		return labelStyle.Render(readiness) + "\n" + okStyle.Render(ready)
	}
	lines := make([]string, 0, len(m.preflight))
	for _, issue := range m.preflight {
		lines = append(lines, "• "+issue)
	}
	return labelStyle.Render(label) + "\n" + m.panel(errorStyle.Render(strings.Join(lines, "\n")))
}

func (m Model) draftWarningView() string {
	if m.draftErr == nil {
		return ""
	}
	return labelStyle.Render("Draft recovery") + "\n" + m.inline(errorStyle.Render(compact("The current draft could not be saved: "+m.draftErr.Error(), m.inlineWidth())))
}

func (m *Model) focusField() tea.Cmd {
	m.task.Blur()
	m.verification.Blur()
	m.provider.Blur()
	m.model.Blur()
	m.normalizeDropdownSelection()
	switch m.focus {
	case taskField:
		return m.task.Focus()
	case verificationField:
		return m.verification.Focus()
	case providerField:
		return m.provider.Focus()
	case modelField:
		return m.model.Focus()
	default:
		return nil
	}
}

func (m *Model) resizeInputs() {
	width := max(1, m.width-8)
	m.task.SetWidth(width)
	m.verification.SetWidth(width)
	m.provider.Width = width
	m.model.Width = width

	switch {
	case m.height < 20:
		m.task.SetHeight(2)
		m.verification.SetHeight(1)
	case m.height < 34:
		m.task.SetHeight(3)
		m.verification.SetHeight(1)
	case m.height < 48:
		m.task.SetHeight(5)
		m.verification.SetHeight(2)
	default:
		m.task.SetHeight(7)
		m.verification.SetHeight(3)
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return "Starting Gator..."
	}
	var view string
	switch m.screen {
	case composeScreen:
		view = m.composeView()
	case attachmentConfirmScreen:
		view = m.attachmentConfirmView()
	case runningScreen:
		view = m.runningView()
	case reviewScreen:
		view = m.reviewView()
	case transcriptScreen:
		view = m.transcriptView()
	case helpScreen:
		view = m.helpView()
	case recentScreen:
		view = m.recentRunsView()
	default:
		return ""
	}
	return m.fitToTerminal(view)
}

func (m Model) attachmentConfirmView() string {
	provider := strings.TrimSpace(m.provider.Value())
	if provider == "" {
		provider = "the selected provider"
	}
	lines := make([]string, 0, len(m.attachmentPreview))
	for _, attachment := range m.attachmentPreview {
		lines = append(lines, "• "+attachment.name+" · "+attachment.mediaType+" · "+formatAttachmentSize(attachment.bytes))
	}
	if len(lines) == 0 {
		lines = append(lines, "No attachment bytes are pending.")
	}
	sections := []string{
		m.header("confirm attachment transmission"),
		m.fieldView("Files to send", "These exact bytes will be sent with this task.", strings.Join(lines, "\n")),
		m.fieldView("Provider boundary", "The selected provider receives the files under its own data-handling and retention policy. Review that policy before sending.", provider),
		m.fieldView("Safety and continuation", "Attachment contents are untrusted data: instruction-like text inside them can influence a model despite Gator's safeguards. Gator does not retain raw attachment bytes in the continuation session; re-add @ files to send them in a later turn.", "PDF byte limits do not cap provider page or token cost."),
		m.noticeView(),
		m.footer("enter/y send", "esc/n cancel", "f1 shortcuts"),
	}
	return strings.Join(sections, "\n")
}

func formatAttachmentSize(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	return fmt.Sprintf("%.1f KiB", float64(bytes)/1024)
}

func (m Model) composeView() string {
	mode := "new isolated thread · " + m.runMode.String()
	if m.resumeStatePath != "" {
		mode = "thread " + compact(m.threadID, 12) + " · " + m.runMode.String()
	}
	if m.compactComposer() || (m.height < 52 && (m.commandPaletteVisible() || m.contextCompletionVisible() || m.dropdownVisible())) {
		return m.compactComposeView(mode)
	}
	verificationHint := "One allowed argv command per line. Each must pass before Gator accepts completion."
	if m.runMode == gatorrun.PlanMode {
		verificationHint = "Plan mode cannot run commands. This verifier policy will apply after switching to Execute."
	}
	providerHint := "Choose from the dropdown or type to filter providers."
	modelHint := "Choose a recommendation or type any model ID supported by the provider."
	if m.resumeStatePath != "" {
		verificationHint = "Inherited from the retained run to preserve its command policy."
		providerHint = "Inherited from the retained run to preserve provider continuity."
		modelHint = "Inherited from the retained run to preserve conversation continuity."
	}
	sections := []string{
		m.header(mode),
		m.fieldView("Task", "Explain the desired behavior and any constraints.", m.task.View()),
	}
	if palette := m.commandPaletteView(); palette != "" {
		sections = append(sections, palette, m.noticeView(), m.footer("up/down choose", "enter select", "esc dismiss", "f1 shortcuts", "ctrl+c quit"))
		return strings.Join(sections, "\n")
	}
	if references := m.contextReferencesView(); references != "" {
		sections = append(sections, references)
	}
	if completions := m.contextCompletionView(); completions != "" {
		sections = append(sections, completions)
	}
	providerSection := m.fieldView("Provider", providerHint, m.provider.View())
	modelSection := m.fieldView("Model", modelHint, m.model.View())
	if dropdown := m.dropdownView(); dropdown != "" {
		if m.focus == providerField {
			providerSection += "\n" + dropdown
		} else {
			modelSection += "\n" + dropdown
		}
	}
	sections = append(sections,
		m.fieldView("Verification", verificationHint, m.verification.View()),
		providerSection,
		modelSection,
	)
	if readiness := m.preflightView(); readiness != "" {
		sections = append(sections, readiness)
	}
	if warning := m.draftWarningView(); warning != "" {
		sections = append(sections, warning)
	}
	if m.commandOutput != "" {
		sections = append(sections, labelStyle.Render("Command result"), m.panel(m.commandOutput))
	}
	sections = append(sections,
		m.noticeView(),
		m.footer("? commands", "ctrl+o threads", "tab switch field", "ctrl+r start "+m.runMode.String(), "f1 shortcuts", "ctrl+c quit"),
	)
	return strings.Join(sections, "\n")
}

func (m Model) compactComposeView(mode string) string {
	sections := []string{
		m.header(mode),
		m.fieldView("Task", "", m.task.View()),
	}
	if palette := m.commandPaletteView(); palette != "" {
		sections = append(sections, palette, m.noticeView(), m.footer("up/down choose", "enter select", "esc dismiss", "f1 shortcuts", "ctrl+c quit"))
		return strings.Join(sections, "\n")
	}
	if completions := m.contextCompletionView(); completions != "" {
		sections = append(sections, completions, m.noticeView(), m.footer("up/down choose", "enter insert", "esc dismiss", "f1 shortcuts", "ctrl+c quit"))
		return strings.Join(sections, "\n")
	}

	if m.focus != taskField {
		label, value := "", ""
		switch m.focus {
		case verificationField:
			label, value = "Verification", m.verification.View()
		case providerField:
			label, value = "Provider", m.provider.View()
		case modelField:
			label, value = "Model", m.model.View()
		}
		sections = append(sections, m.fieldView(label, "", value))
		if dropdown := m.dropdownView(); dropdown != "" {
			sections = append(sections, dropdown)
		}
	}

	sections = append(sections, m.composerSummary(), m.compactPreflightView())
	if m.commandOutput != "" {
		sections = append(sections, labelStyle.Render("Command result"), m.panel(compact(m.commandOutput, max(16, m.panelTextWidth()*3))))
	}
	if warning := m.draftWarningView(); warning != "" {
		sections = append(sections, warning)
	}
	sections = append(sections,
		m.noticeView(),
		m.footer("? commands", "ctrl+o threads", "tab switch field", "ctrl+r start "+m.runMode.String(), "f1 shortcuts", "ctrl+c quit"),
	)
	return strings.Join(sections, "\n")
}

func (m Model) composerSummary() string {
	verification := "no commands"
	if m.runMode == gatorrun.PlanMode {
		verification = "deferred in plan"
	}
	if commands, err := parseVerification(m.verification.Value()); err == nil && len(commands) > 0 {
		verification = fmt.Sprintf("%d command(s)", len(commands))
	} else if strings.TrimSpace(m.verification.Value()) != "" {
		verification = "needs attention"
	}
	provider := strings.TrimSpace(m.provider.Value())
	if provider == "" {
		provider = "not selected"
	}
	model := strings.TrimSpace(m.model.Value())
	if model == "" {
		model = "provider default"
	}
	text := "verify: " + verification + " · provider: " + provider + " · model: " + model
	return m.inline(dimStyle.Render(compact(text, m.inlineWidth())))
}

func (m Model) compactPreflightView() string {
	if len(m.preflight) == 0 {
		ready := "Ready to create an isolated worktree."
		if m.runMode == gatorrun.PlanMode {
			ready = "Ready to inspect an isolated worktree without edits."
		}
		return m.inline(okStyle.Render(ready))
	}
	label := "Before starting: "
	if m.resumeStatePath != "" {
		label = "Before continuing: "
	}
	if m.runMode == gatorrun.PlanMode {
		label = "Before planning: "
	}
	return m.inline(errorStyle.Render(compact(label+m.preflight[0], m.inlineWidth())))
}

func (m Model) helpView() string {
	if m.compactLayout() {
		lines := []string{
			"F1  close this help",
			"Ctrl+R  start the selected mode",
			"Ctrl+O  choose a retained thread",
			"Tab / Shift+Tab  move between fields",
			"?  open commands; @  reference a path",
			"Ctrl+C  quit",
		}
		if m.constrainedLayout() {
			lines = []string{
				"F1  close this help",
				"Ctrl+R  start the selected mode",
				"Tab  move between fields",
				"Ctrl+C  quit",
			}
		}
		return strings.Join([]string{
			m.header("keyboard shortcuts"),
			m.panel(strings.Join(lines, "\n")),
			m.footer("esc close help"),
		}, "\n")
	}
	sections := []string{
		m.header("keyboard shortcuts"),
		labelStyle.Render("Composer") + "\n" + m.panel(strings.Join([]string{
			"F1  show or close this help",
			"Ctrl+R  start the selected mode",
			"Ctrl+O  choose a retained thread",
			"Tab / Shift+Tab  move between fields",
			"?  open the / command menu from an empty task",
			"@  begin a repository-path reference",
			"Ctrl+Space (Ctrl+@)  reopen @ path suggestions",
			"Arrows + Enter or Tab  choose an open suggestion",
			"Ctrl+C  quit",
		}, "\n")),
		labelStyle.Render("Running") + "\n" + m.panel("F1  show this help\nCtrl+C  request cancellation and retain the worktree"),
		labelStyle.Render("Review") + "\n" + m.panel("F1  show this help\nc  continue the retained thread\nd  refresh the diff\ne  show patch export/apply commands\nn or Esc  start a new task\nq or Ctrl+C  quit"),
		m.footer("esc close help"),
	}
	return strings.Join(sections, "\n")
}

func (m Model) recentRunsView() string {
	start, end := m.visibleRange(len(m.recentThreads), m.recentIndex, m.recentRunLimit())
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		thread := m.recentThreads[index]
		prefix := "  "
		if index == m.recentIndex {
			prefix = "> "
		}
		modelName := thread.Model
		if modelName == "" {
			modelName = "provider default"
		}
		availability := okStyle.Render("path found")
		if !thread.Available {
			availability = errorStyle.Render("worktree missing")
		}
		limit := 96
		if m.compactLayout() {
			limit = max(12, m.panelTextWidth()-22)
		}
		turns := fmt.Sprintf("%d turn", thread.TurnCount)
		if thread.TurnCount != 1 {
			turns += "s"
		}
		lines = append(lines, prefix+keyStyle.Render(thread.Provider+" · "+modelName)+"  "+availability+"\n    "+dimStyle.Render(thread.UpdatedAt.Local().Format("Jan 2 15:04"))+"  "+dimStyle.Render(turns)+"  "+compact(thread.Task, limit))
	}
	sections := []string{
		m.header("recent conversation threads"),
		m.panel(strings.Join(lines, "\n\n")),
	}
	if !m.compactLayout() {
		sections = append(sections, dimStyle.Render("Thread state stays private; Gator validates a selected worktree before continuation."))
	}
	sections = append(sections, m.noticeView(), m.footer("up/down choose", "enter continue", "r refresh", "esc back", "f1 shortcuts"))
	return strings.Join(sections, "\n")
}

func (m Model) runningView() string {
	status := "Gator is working in an isolated worktree."
	if m.runMode == gatorrun.PlanMode {
		status = "Gator is inspecting the isolated worktree in enforced read-only Plan mode."
	}
	if m.cancelling {
		status = "Gator is stopping; the retained worktree will remain reviewable."
	}
	entries := m.events
	maxEntries := max(1, m.height-7)
	if len(entries) > maxEntries {
		entries = entries[len(entries)-maxEntries:]
	}
	lines := make([]string, 0, len(entries)+1)
	if len(entries) == 0 {
		lines = append(lines, dimStyle.Render("waiting for the first agent event..."))
	}
	for _, entry := range entries {
		lines = append(lines, entry.text)
		if entry.detail != "" {
			lines = append(lines, "    "+compact(entry.detail, max(24, m.panelTextWidth()-4)))
		}
	}
	sections := []string{
		m.header("live " + m.runMode.String()),
		m.inline(dimStyle.Render(compact(status, m.inlineWidth()))),
		m.panel(strings.Join(lines, "\n")),
		m.noticeView(),
		m.footer("ctrl+c stop after current operation", "f1 shortcuts"),
	}
	return strings.Join(sections, "\n")
}

func (m Model) reviewView() string {
	title := "review"
	if m.runMode == gatorrun.PlanMode {
		title = "plan review"
	}
	sections := []string{m.header(title)}
	if m.outcome != nil {
		if m.compactLayout() {
			sections = append(sections, m.inline(dimStyle.Render(compact("worktree: "+m.outcome.Worktree.Path, m.inlineWidth()))))
		} else {
			sections = append(sections, labelStyle.Render("Retained worktree"), m.inline(m.outcome.Worktree.Path))
			sections = append(sections, labelStyle.Render("Run record"), m.inline(m.outcome.StatePath))
		}
	}
	if m.runErr != nil {
		sections = append(sections, m.inline(errorStyle.Render(compact("Run result: "+m.runErr.Error(), m.inlineWidth()))))
	} else if m.outcome != nil && strings.TrimSpace(m.outcome.Result.FinalText) != "" {
		result := m.outcome.Result.FinalText
		if m.compactLayout() {
			result = compact(result, max(16, m.panelTextWidth()*2))
		}
		sections = append(sections, m.panel(result))
	}
	sections = append(sections, labelStyle.Render("Current diff"), m.diffView(), m.noticeView())
	continueLabel := "c continue thread"
	if m.runMode == gatorrun.PlanMode {
		continueLabel = "c continue plan"
	}
	if m.outcome == nil || m.outcome.StatePath == "" {
		continueLabel = ""
	}
	sections = append(sections, m.footer("d refresh diff", "t transcript", "e patch handoff", continueLabel, "n new task", "f1 shortcuts", "q quit"))
	return strings.Join(sections, "\n")
}

func (m Model) transcriptView() string {
	if len(m.events) == 0 {
		return m.header("run transcript") + "\n" + m.panel(dimStyle.Render("No live events were recorded for this run.")) + "\n" + m.footer("esc return")
	}
	start, end := m.visibleRange(len(m.events), m.transcriptIndex, max(1, m.height/4))
	sections := []string{m.header("run transcript")}
	for index := start; index < end; index++ {
		entry := m.events[index]
		prefix := "  "
		if index == m.transcriptIndex {
			prefix = "> "
		}
		content := prefix + entry.text
		if entry.detail != "" {
			content += "\n" + entry.detail
		}
		sections = append(sections, m.panel(content))
	}
	sections = append(sections, m.footer("up/down browse", "t/esc return", "f1 shortcuts"))
	return strings.Join(sections, "\n")
}
