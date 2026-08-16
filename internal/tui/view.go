package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

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
	return m.chatView(false)
}

func (m Model) chatView(running bool) string {
	mode := "new thread"
	if m.resumeStatePath != "" {
		mode = "thread " + compact(m.threadID, 12)
	}
	provider := strings.TrimSpace(m.provider.Value())
	if provider == "" {
		provider = "provider not selected"
	}
	model := strings.TrimSpace(m.model.Value())
	if model == "" {
		model = "provider default"
	}
	sections := []string{
		m.header(mode + " · " + m.runMode.String()),
		m.inline(dimStyle.Render(compact(provider+" · "+model+" · "+m.vimModeLabel(), m.inlineWidth()))),
	}

	if running {
		sections = append(sections, m.activityView(), m.chatHistoryView())
		if verification := m.verificationStatusView(); verification != "" {
			sections = append(sections, verification)
		}
		if m.vimCommand != "" {
			sections = append(sections, m.vimCommandView())
		} else {
			if palette := m.commandPaletteView(); palette != "" {
				sections = append(sections, palette)
			}
			if references := m.contextReferencesView(); references != "" {
				sections = append(sections, references)
			}
			if completions := m.contextCompletionView(); completions != "" {
				sections = append(sections, completions)
			}
		}
		label := "Steer this run"
		if m.vim != vimOff {
			label += " · " + m.vimModeLabel()
		}
		sections = append(sections, labelStyle.Render(label), m.panel(m.task.View()))
		if preview := m.queuePreviewView(); preview != "" {
			sections = append(sections, preview)
		}
		if m.commandOutput != "" {
			sections = append(sections, labelStyle.Render("Local status"), m.panel(compact(m.commandOutput, max(16, m.panelTextWidth()*3))))
		}
		sections = append(sections, m.noticeView(), m.runningFooter())
		return strings.Join(sections, "\n")
	}

	sections = append(sections, m.chatHistoryView())
	if result := m.latestRunView(); result != "" {
		sections = append(sections, result)
	}
	if verification := m.verificationStatusView(); verification != "" {
		sections = append(sections, verification)
	}

	if m.vimCommand != "" {
		sections = append(sections, m.vimCommandView())
	} else if configuration := m.chatConfigurationView(); configuration != "" {
		sections = append(sections, configuration)
	} else {
		if palette := m.commandPaletteView(); palette != "" {
			sections = append(sections, palette)
		}
		if references := m.contextReferencesView(); references != "" {
			sections = append(sections, references)
		}
		if completions := m.contextCompletionView(); completions != "" {
			sections = append(sections, completions)
		}
		label := "You"
		if m.vim != vimOff {
			label += " · " + m.vimModeLabel()
		}
		sections = append(sections, labelStyle.Render(label), m.panel(m.task.View()))
	}
	if m.commandOutput != "" {
		sections = append(sections, labelStyle.Render("Local status"), m.panel(compact(m.commandOutput, max(16, m.panelTextWidth()*3))))
	}
	if warning := m.draftWarningView(); warning != "" {
		sections = append(sections, warning)
	}
	sections = append(sections, m.noticeView())
	if m.vimCommand != "" {
		sections = append(sections, m.footer("enter run Vim command", "esc cancel", "f1 shortcuts", "ctrl+c quit"))
	} else if m.focus == taskField {
		switch m.vim {
		case vimNormal:
			sections = append(sections, m.footer("i/a edit", "enter send", "? commands", "ctrl+r send", "f1 shortcuts", "ctrl+c quit"))
		case vimInsert:
			sections = append(sections, m.footer("esc normal", "enter newline", "ctrl+r send", "f1 shortcuts", "ctrl+c quit"))
		default:
			sections = append(sections, m.footer("? commands", "ctrl+o threads", "pgup/pgdn browse", "enter send", "ctrl+r send", "f1 shortcuts", "ctrl+c quit"))
		}
	} else {
		sections = append(sections, m.footer("tab change field", "ctrl+r send", "f1 shortcuts", "ctrl+c quit"))
	}
	return strings.Join(sections, "\n")
}

func (m Model) runningFooter() string {
	if m.vimCommand != "" {
		return m.footer("enter run Vim command", "esc cancel", "ctrl+c stop", "f1 shortcuts")
	}
	switch m.vim {
	case vimNormal:
		return m.footer("i/a edit", "enter steer", "tab queue", "pgup/pgdn browse", "ctrl+c stop", "f1 shortcuts")
	case vimInsert:
		return m.footer("esc normal", "enter newline", "ctrl+r steer", "tab queue", "ctrl+c stop", "f1 shortcuts")
	default:
		return m.footer("enter steer", "tab queue", "pgup/pgdn browse", "ctrl+c stop", "f1 shortcuts")
	}
}

func (m Model) activityView() string {
	phase := m.activityPhaseLabel()
	detail := m.activity.detail
	if detail == "" {
		detail = "creating an isolated worktree"
	}
	if m.cancelling {
		phase = "stopping"
		detail = "waiting for the current operation to stop; the worktree will remain reviewable"
	}
	activity := "● " + phase
	if duration := m.activityDuration(time.Now()); duration != "" {
		activity += " · " + duration
	}
	providerMode := "native"
	steering := "Enter steers at a model or tool boundary"
	if m.execution != nil && !m.execution.steeringSupported {
		providerMode = "delegated CLI"
		steering = "Tab queues follow-up work; active steering is unavailable"
	}
	mode := m.runMode.String()
	if m.runMode == gatorrun.PlanMode {
		mode += " read-only"
	}
	turn := m.activity.turn
	if turn < 1 {
		turn = 1
	}
	facts := fmt.Sprintf("%s · %s · isolated worktree · turn %d/%d", mode, providerMode, turn, m.config.MaxSteps)
	if age := m.lastActivityAge(time.Now()); age != "" {
		facts += " · last activity " + age
	}
	return labelStyle.Render("Activity") + "\n" + m.panel(
		keyStyle.Render(compact(activity, m.panelTextWidth()))+"\n"+
			dimStyle.Render(compact(detail, m.panelTextWidth()))+"\n"+
			dimStyle.Render(compact(facts+" · "+steering, m.panelTextWidth())),
	)
}

func (m Model) verificationStatusView() string {
	if len(m.verificationStatus) == 0 {
		return ""
	}
	limit := min(3, len(m.verificationStatus))
	lines := make([]string, 0, limit+1)
	for index := 0; index < limit; index++ {
		status := m.verificationStatus[index]
		icon := "○"
		switch status.phase {
		case verificationRunning:
			icon = "●"
		case verificationPassed:
			icon = "✓"
		case verificationFailed:
			icon = "✗"
		}
		line := icon + " " + strings.Join(status.argv, " ") + " · " + verificationPhaseLabel(status.phase)
		if status.phase == verificationRunning && !status.startedAt.IsZero() {
			line += " · " + time.Since(status.startedAt).Round(time.Second).String()
		}
		if status.phase == verificationFailed && status.detail != "" {
			line += " · " + status.detail
		}
		style := dimStyle
		if status.phase == verificationPassed {
			style = okStyle
		} else if status.phase == verificationFailed {
			style = errorStyle
		}
		lines = append(lines, style.Render(compact(line, m.panelTextWidth())))
	}
	if len(m.verificationStatus) > limit {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("+%d additional verification command(s)", len(m.verificationStatus)-limit)))
	}
	return labelStyle.Render("Verification") + "\n" + m.panel(strings.Join(lines, "\n"))
}

func (m Model) queuePreviewView() string {
	if len(m.queue) == 0 {
		return ""
	}
	limit := min(2, len(m.queue))
	lines := make([]string, 0, limit+1)
	for index := 0; index < limit; index++ {
		item := m.queue[index]
		lines = append(lines, fmt.Sprintf("%d. %s", index+1, compact(item.text, max(16, m.panelTextWidth()-4))))
	}
	if len(m.queue) > limit {
		lines = append(lines, fmt.Sprintf("+%d more · /queue manages pending work", len(m.queue)-limit))
	} else {
		lines = append(lines, "/queue manages pending work")
	}
	return labelStyle.Render("Queued next · "+m.queueSummary()) + "\n" + m.panel(strings.Join(lines, "\n"))
}

func (m Model) latestRunView() string {
	if m.outcome == nil && m.runErr == nil {
		return ""
	}
	if m.runErr != nil {
		return labelStyle.Render("Latest run") + "\n" + m.panel(errorStyle.Render(compact("Stopped: "+m.runErr.Error(), m.panelTextWidth()))+"\n"+dimStyle.Render(compact(m.failureGuidance(), m.panelTextWidth())))
	}
	result := m.verificationSummary()
	if m.diffStats.ready {
		if m.diffStats.files == 0 {
			result += " · no diff visible"
		} else {
			result += fmt.Sprintf(" · %d file(s) · +%d −%d", m.diffStats.files, m.diffStats.additions, m.diffStats.deletions)
		}
	} else if m.outcome != nil && m.outcome.Worktree.Path != "" {
		result += " · loading diff summary"
	}
	return labelStyle.Render("Latest run") + "\n" + m.panel(okStyle.Render(compact("✓ Complete · "+result, m.panelTextWidth()))+"\n"+dimStyle.Render(compact("Use /review to inspect the retained worktree and patch handoff.", m.panelTextWidth())))
}

func (m Model) failureGuidance() string {
	if m.lastRunCancelled {
		return "Cancelled by you. The worktree is retained; use /review or send a focused follow-up when ready."
	}
	if m.runErr == nil {
		return "Use /review to inspect the retained worktree."
	}
	message := strings.ToLower(m.runErr.Error())
	switch {
	case strings.Contains(message, "verification"):
		return "Verification did not complete successfully. Inspect its transcript and output, then send a focused follow-up."
	case strings.Contains(message, "harness"), strings.Contains(message, "cli"):
		return "The delegated CLI stopped. Inspect the transcript, check that provider CLI setup is usable, then retry with a focused follow-up."
	case strings.Contains(message, "api"), strings.Contains(message, "credential"), strings.Contains(message, "authentication"):
		return "The model provider stopped before completion. Check provider configuration or credentials, then retry the task."
	default:
		return "Inspect the transcript and retained worktree with /review, then send a focused follow-up."
	}
}

func (m Model) vimCommandView() string {
	return labelStyle.Render("Vim command") + "\n" + m.panel(keyStyle.Render(m.vimCommand)+"\n"+dimStyle.Render(":w send · :wq send then exit after successful queued work"))
}

func (m Model) vimModeLabel() string {
	switch m.vim {
	case vimNormal:
		return "vim normal"
	case vimInsert:
		return "vim insert"
	default:
		return "chat input"
	}
}

func (m Model) chatHistoryView() string {
	if len(m.chat) == 0 {
		return m.panel(dimStyle.Render("No messages yet. Describe the work you want to do below."))
	}
	limit := m.chatEntryLimit()
	start, end := m.visibleRange(len(m.chat), m.chatIndex, limit)
	lines := make([]string, 0, (end-start)*2)
	for index := start; index < end; index++ {
		entry := m.chat[index]
		label := "Gator"
		style := labelStyle
		switch entry.author {
		case chatUser:
			label, style = "You", keyStyle
		case chatTool:
			label, style = "Tool", dimStyle
		case chatSystem:
			label, style = "System", dimStyle
		}
		lines = append(lines, style.Render(label)+"  "+compact(entry.text, max(16, m.panelTextWidth()*2)))
		if entry.detail != "" {
			lines = append(lines, "      "+dimStyle.Render(compact(entry.detail, max(16, m.panelTextWidth()))))
		}
	}
	return m.panel(strings.Join(lines, "\n"))
}

func (m Model) chatEntryLimit() int {
	if m.height <= 0 {
		return 6
	}
	reserved := m.task.Height() + 12
	if m.screen == runningScreen {
		reserved += 6
		if len(m.verificationStatus) > 0 {
			reserved += min(5, len(m.verificationStatus)+2)
		}
		if len(m.queue) > 0 {
			reserved += min(4, len(m.queue)+2)
		}
	}
	return max(1, (m.height-reserved)/3)
}

func (m Model) chatConfigurationView() string {
	if m.focus == taskField {
		return ""
	}
	label, hint, value := "", "", ""
	switch m.focus {
	case verificationField:
		label = "Verification policy"
		hint = "One allowed argv command per line. Use /execute only when every listed command is intentional."
		value = m.verification.View()
	case providerField:
		label = "Provider"
		hint = "Choose a provider or type to filter it."
		value = m.provider.View()
	case modelField:
		label = "Model"
		hint = "Choose a recommendation or enter a provider-supported model ID."
		value = m.model.View()
	}
	view := m.fieldView(label, hint, value)
	if dropdown := m.dropdownView(); dropdown != "" {
		view += "\n" + dropdown
	}
	return view
}

func (m Model) helpView() string {
	if m.compactLayout() {
		lines := []string{
			"F1  close this help",
			"Enter  send the current message",
			"Ctrl+O  choose a retained thread",
			"PgUp / PgDn  browse the conversation",
			"?  open commands; @  reference a path",
			"Ctrl+C  quit",
		}
		if m.constrainedLayout() {
			lines = []string{
				"F1  close this help",
				"Enter  send the current message",
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
		labelStyle.Render("Conversation") + "\n" + m.panel(strings.Join([]string{
			"F1  show or close this help",
			"Enter  send the current message",
			"Ctrl+R  send in any input mode",
			"Ctrl+O  choose a retained thread",
			"Recent picker: a toggles current repository / all repositories",
			"PgUp / PgDn  browse the conversation",
			"?  open the / command menu from an empty task",
			"@  begin a repository-path reference",
			"Ctrl+Space (Ctrl+@)  reopen @ path suggestions",
			"Tab  complete a command or insert a path; Enter runs a command",
			"/vim  toggle Vim Normal/Insert message editing",
			":w  send from Vim Normal; :wq  send then exit after successful work",
			"Ctrl+C  quit",
		}, "\n")),
		labelStyle.Render("Running") + "\n" + m.panel("Enter  steer a native run at its next model/tool boundary\nTab  queue the next prompt or a slash command\n/queue, /dequeue, /clear-queue  inspect or manage local queued work\nDelegated CLI providers cannot accept active steering; use Tab\nPgUp / PgDn  browse conversation\nCtrl+C  request cancellation and retain the worktree"),
		labelStyle.Render("Review") + "\n" + m.panel("F1  show this help\nc or Esc  return to conversation\nd  refresh the diff\ne  show patch export/apply commands\nn  start a new task\nq or Ctrl+C  quit"),
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
		repository := ""
		if m.recentAll {
			repository = filepath.Base(thread.Repository) + " · "
		}
		lines = append(lines, prefix+keyStyle.Render(repository+thread.Provider+" · "+modelName)+"  "+availability+"\n    "+dimStyle.Render(thread.UpdatedAt.Local().Format("Jan 2 15:04"))+"  "+dimStyle.Render(turns)+"  "+compact(thread.Task, limit))
	}
	title := "recent conversation threads"
	if m.recentAll {
		title = "recent conversation threads · all repositories"
	}
	sections := []string{
		m.header(title),
		m.panel(strings.Join(lines, "\n\n")),
	}
	if !m.compactLayout() {
		sections = append(sections, dimStyle.Render("Thread state stays private; Gator validates a selected worktree before continuation."))
	}
	sections = append(sections, m.noticeView(), m.footer("up/down choose", "enter continue", "a scope", "r refresh", "esc back", "f1 shortcuts"))
	return strings.Join(sections, "\n")
}

func (m Model) runningView() string {
	return m.chatView(true)
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
