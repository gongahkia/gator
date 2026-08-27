package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gongahkia/gator/internal/journal"
	gatorrun "github.com/gongahkia/gator/internal/run"
)

func (m Model) preflightView() string {
	label := "Before starting"
	ready := "Ready to create an isolated worktree."
	readiness := "Run readiness"
	if m.delegateRuntime != "" {
		label = "Before running " + delegatedRuntimeLabel(m.delegateRuntime)
		ready = "Ready to start " + delegatedRuntimeLabel(m.delegateRuntime) + " in an isolated worktree."
		readiness = "Harness readiness"
	}
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
	width := m.composerInputWidth()
	m.verification.SetWidth(width)
	m.provider.Width = width
	m.model.Width = width
	m.terminalInput.Width = width
	m.runOptions.maxSteps.Width = width
	m.runOptions.baseRef.Width = width
	m.runOptions.setup.SetWidth(width)
	m.runOptions.scopes.SetWidth(width)
	m.runOptions.scouts.SetWidth(width)
	m.runOptions.allowed.SetWidth(width)
	m.runOptions.prefixes.SetWidth(width)

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
	m.syncVimLineNumbers()
	m.resizeConversation()
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
	case terminalScreen:
		view = m.attachedTerminalView()
	case reviewScreen:
		view = m.reviewView()
	case transcriptScreen:
		view = m.transcriptView()
	case helpScreen:
		view = m.helpView()
	case recentScreen:
		view = m.recentRunsView()
	case threadScreen:
		view = m.threadTreeView()
	case effortScreen:
		view = m.effortView()
	case extensionUIScreen:
		view = m.extensionUIView()
	case localModelsScreen:
		view = m.localModelsView()
	case managementScreen:
		view = m.managementView()
	case doctorScreen:
		view = m.doctorView()
	case runOptionsScreen:
		view = m.runOptionsView()
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

func (m Model) localModelsView() string {
	sections := []string{m.header("models")}
	sections = append(sections, m.inline(dimStyle.Render("Effort: "+m.effort.label()+" · intent-level turn budget. Ctrl+S or /effort changes it; this screen controls exact models and logins.")))
	if m.localModels.manager == nil {
		sections = append(sections, m.panel(errorStyle.Render("Model management was not configured for this TUI session.")), m.noticeView(), m.footer("esc return"))
		return strings.Join(sections, "\n")
	}

	if m.localModels.section == cloudModelSection {
		sections = append(sections, m.cloudModelsView())
		sections = append(sections, m.inline(dimStyle.Render(m.localModelsSummary())))
	} else {
		sections = append(sections, m.localRuntimeView(), m.localCatalogView())
		if m.localModels.dependencyHelp {
			sections = append(sections, m.localDependencyHelpView())
		}
		sections = append(sections, m.inline(dimStyle.Render("Cloud provider readiness is available under the Cloud section.")))
	}

	if m.localModels.action != localModelIdle {
		detail := m.localModelActionLabel()
		if m.localModels.action == localModelPulling {
			detail = m.localModelProgressLabel()
		}
		sections = append(sections, m.fieldView("Working", "The TUI remains responsive; Esc or Ctrl+C requests cancellation.", m.localModels.spinner.View()+" "+detail))
	}

	if m.localModels.renaming != nil {
		sections = append(sections, m.fieldView("Rename display label", "This changes only Gator's local label; the provider model ID remains unchanged.", m.localModels.renaming.input.View()))
	}
	if setup := m.cloudModelSetupView(); setup != "" {
		sections = append(sections, setup)
	}
	if setup := m.customProviderSetupView(); setup != "" {
		sections = append(sections, setup)
	}
	if m.oauthLogin != nil && strings.TrimSpace(m.commandOutput) != "" {
		sections = append(sections, m.fieldView("Cloud sign-in", "Complete the provider login in your browser. This TUI stays open while Gator waits for its callback.", m.commandOutput))
	}
	if m.localModels.confirmation != localModelNoConfirmation {
		sections = append(sections, m.modelCatalogConfirmationView())
	}

	if m.localModels.cloudSetup != nil {
		keys := []string{"tab/up/down next field", "enter save", "esc cancel"}
		if m.localModels.cloudSetup.canUseBearerToken() {
			keys = append(keys, "a auth type")
		}
		sections = append(sections, m.noticeView(), m.footer(append(keys, "f1 shortcuts")...))
	} else if m.localModels.customSetup != nil && m.localModels.confirmation != localModelConfirmSaveCustom {
		sections = append(sections, m.noticeView(), m.footer("tab next field", "enter review", "esc cancel", "f1 shortcuts"))
	} else if m.localModels.renaming != nil {
		sections = append(sections, m.noticeView(), m.footer("enter save", "esc cancel", "f1 shortcuts"))
	} else if m.oauthLogin != nil {
		sections = append(sections, m.noticeView(), m.footer("ctrl+c cancel sign-in", "f1 shortcuts"))
	} else if m.localModels.confirmation == localModelConfirmStart {
		sections = append(sections, m.noticeView(), m.footer("enter/y start with Gator", "n/esc start myself", "f1 shortcuts"))
	} else if m.localModels.confirmation == localModelConfirmInstall {
		sections = append(sections, m.noticeView(), m.footer("enter/y installation help", "n/esc install myself", "f1 shortcuts"))
	} else if m.localModels.confirmation == localModelConfirmPull {
		sections = append(sections, m.noticeView(), m.footer("enter/y download", "esc/n cancel", "f1 shortcuts"))
	} else if m.localModels.confirmation == localModelConfirmRemove || m.localModels.confirmation == localModelConfirmRemoveCredential || m.localModels.confirmation == localModelConfirmRemoveCustom || m.localModels.confirmation == localModelConfirmSaveCustom || m.localModels.confirmation == localModelConfirmApplyDiscovery {
		sections = append(sections, m.noticeView(), m.footer("enter/y confirm", "esc/n cancel", "f1 shortcuts"))
	} else if m.localModels.action != localModelIdle {
		sections = append(sections, m.noticeView(), m.footer("esc/ctrl+c cancel", "f1 shortcuts"))
	} else if m.localModels.dependencyHelp {
		sections = append(sections, m.noticeView(), m.footer("i/esc close help", "f1 shortcuts"))
	} else if m.localModels.section == cloudModelSection {
		sections = append(sections, m.noticeView(), m.footer("up/down choose", "u/enter use", "c configure", "n new provider", "g discover", "d remove credential", "x remove provider", "l sign in", "e rename", "tab local", "r refresh", "esc composer", "f1 shortcuts"))
	} else {
		sections = append(sections, m.noticeView(), m.footer("up/down choose", "p pull", "u/enter use", "x remove", "e rename", "s start Ollama", "i install help", "tab cloud", "r refresh", "esc composer", "f1 shortcuts"))
	}
	return strings.Join(sections, "\n")
}

func (m Model) modelCatalogConfirmationView() string {
	switch m.localModels.confirmation {
	case localModelConfirmStart:
		return m.fieldView("Start Ollama?", "Gator can start 'ollama serve' as a child of this TUI and stops it when Gator exits. You can instead start it yourself.", "Enter/y  Start with Gator (default)\nn/esc  I'll start it myself")
	case localModelConfirmInstall:
		return m.fieldView("Install Ollama?", "Ollama is not installed. Gator can show the official source and platform advice; it never runs a system installer or package manager.", "Enter/y  Open installation help (default)\nn/esc  I'll install it myself")
	case localModelConfirmPull:
		if selected, found := m.selectedLocalModel(); found {
			return m.fieldView("Confirm download", "Model weights and upstream terms remain governed by the linked source.", selected.Name+" · approximately "+selected.Download+"\n"+selected.SourceURL)
		}
	case localModelConfirmRemove:
		if selected, found := m.selectedLocalModel(); found {
			return m.fieldView("Confirm removal", "This deletes local model data from the selected Ollama runtime.", selected.Name+" · "+selected.OllamaModel)
		}
	case localModelConfirmRemoveCredential:
		if cloud, found := m.selectedCloudModel(); found {
			status := m.storedCredential(cloud.provider)
			detail := cloud.provider + " · stored " + status.Kind + "\nOnly Gator's private credential file is changed. Environment variables, AWS/ADC, one-run keys, and vendor CLI logins remain."
			if cloud.provider == "claude" {
				detail = "Claude Code · stored Anthropic API key\nThis removes the Gator-owned Anthropic key. ANTHROPIC_API_KEY in the environment is not unset."
			}
			return m.fieldView("Remove stored Gator credential?", "This does not revoke the upstream key and does not log you out of vendor CLIs.", detail)
		}
	case localModelConfirmRemoveCustom:
		if cloud, found := m.selectedCloudModel(); found {
			return m.fieldView("Remove custom provider?", "This deletes endpoint metadata from Gator config.json. The named API key environment variable is not changed or unset.", cloud.provider)
		}
	case localModelConfirmSaveCustom:
		if m.localModels.pendingCustom != nil {
			return m.fieldView("Save custom provider?", "Review the destination before writing non-secret metadata. No API key is stored.", customProviderReviewText(*m.localModels.pendingCustom))
		}
	case localModelConfirmApplyDiscovery:
		if m.localModels.discovery != nil {
			preview := *m.localModels.discovery
			limit := min(12, len(preview.Models))
			return m.fieldView("Replace configured models?", "These IDs came from an untrusted /models response. Confirming overwrites the configured catalog.", preview.ID+" default "+preview.DefaultModel+"\n"+strings.Join(preview.Models[:limit], "\n"))
		}
	}
	return ""
}

func (m Model) cloudModelsView() string {
	entries := m.cloudModels()
	if len(entries) == 0 {
		return m.fieldView("Cloud models", "No direct cloud providers are available in this build.", dimStyle.Render("No cloud providers found."))
	}
	start, end := m.visibleRange(len(entries), m.localModels.cloudIndex, m.cloudModelLimit())
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		entry := entries[index]
		prefix := "  "
		if index == m.localModels.cloudIndex {
			prefix = "> "
		}
		status := dimStyle.Render(entry.status)
		if strings.Contains(entry.status, "signed in") || strings.Contains(entry.status, "configured") || strings.Contains(entry.status, "stored") || strings.Contains(entry.status, "API key set") {
			status = okStyle.Render(entry.status)
		}
		lines = append(lines, prefix+keyStyle.Render(compact(entry.name, max(16, m.panelTextWidth()-4)))+"\n    "+status)
	}
	return m.fieldView("Cloud models", "Readiness is credential/configuration state only. c configures a built-in provider or edits a custom endpoint; n creates a custom provider; d removes a stored Gator credential; g previews /models. Gator does not claim a complete remote catalog.", strings.Join(lines, "\n"))
}

func (m Model) localRuntimeView() string {
	runtime := ""
	if m.localModels.catalog.Executable == "" {
		runtime = "Ollama executable: not found\nPress i for installation help, or open https://ollama.com/download."
	} else {
		runtime = "Ollama executable: " + m.localModels.catalog.Executable
	}
	if m.localModels.catalog.RuntimeURL != "" {
		runtime += "\nRuntime: " + m.localModels.catalog.RuntimeURL
	}
	if m.localModels.catalog.HostSummary != "" {
		runtime += "\nHost: " + m.localModels.catalog.HostSummary
	}
	for _, advice := range m.localModels.catalog.HostAdvice {
		runtime += "\n" + dimStyle.Render(compact(advice, m.panelTextWidth()))
	}
	if m.localModels.catalog.RuntimeError != "" {
		runtime += "\n" + errorStyle.Render("Unavailable: "+compact(m.localModels.catalog.RuntimeError, m.panelTextWidth()))
		if m.ollamaMissing() {
			runtime += "\n" + dimStyle.Render("Install Ollama, then press r to refresh.")
		} else {
			runtime += "\n" + dimStyle.Render("Start it with 'ollama serve' or 'gator local serve', then press r.")
		}
	} else if m.localModels.catalog.RuntimeVersion != "" {
		runtime += "\n" + okStyle.Render("Connected · Ollama "+m.localModels.catalog.RuntimeVersion)
	}
	return m.fieldView("Local runtime", "Gator only manages a loopback Ollama runtime; use a custom provider for remote endpoints.", runtime)
}

func (m Model) localDependencyHelpView() string {
	missing := m.localMissingDependencies()
	if len(missing) == 0 {
		return ""
	}
	lines := make([]string, 0, len(missing)*3)
	for _, dependency := range missing {
		role := "optional"
		if dependency.Required {
			role = "required"
		}
		lines = append(lines, dependency.Name+" · "+role+" for "+dependency.Purpose)
		if dependency.HelpURL != "" {
			lines = append(lines, "  Official source: "+dependency.HelpURL)
		}
		for _, instruction := range dependency.Instructions {
			lines = append(lines, "  "+instruction)
		}
	}
	return m.fieldView("Installation help", "Gator detected these prerequisites as missing. It provides checked-in guidance but does not execute system installers or package managers.", strings.Join(lines, "\n"))
}

func (m Model) localCatalogView() string {
	if len(m.localModels.catalog.Models) == 0 {
		return m.fieldView("Reviewed local models", "Only reviewed Ollama tags can be downloaded through Gator.", dimStyle.Render("Loading the reviewed catalog…"))
	}
	start, end := m.visibleRange(len(m.localModels.catalog.Models), m.localModels.selected, m.localModelLimit())
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		model := m.localModels.catalog.Models[index]
		prefix := "  "
		if index == m.localModels.selected {
			prefix = "> "
		}
		state := dimStyle.Render("available")
		if model.BlockedReason != "" {
			state = errorStyle.Render("disabled")
		} else if model.Installed {
			state = okStyle.Render("installed")
		}
		line := prefix + keyStyle.Render(model.Name) + " · " + state + " · " + dimStyle.Render(model.Download+" · "+model.Context)
		detail := model.Summary + " · " + model.OllamaModel
		if model.BlockedReason != "" {
			detail = "blocked: " + model.BlockedReason
		}
		if model.Requirement != "" {
			if model.BlockedReason != "" {
				detail = model.Requirement + " · " + detail
			} else {
				detail += " · " + model.Requirement
			}
		}
		lines = append(lines, line+"\n    "+dimStyle.Render(compact(detail, max(16, m.panelTextWidth()-4))))
	}
	view := m.fieldView("Reviewed local models", "Only these reviewed Ollama tags are downloaded through Gator.", strings.Join(lines, "\n"))
	if selected, found := m.selectedLocalModel(); found && !m.compactLayout() && selected.ID != "" {
		view += "\n" + m.inline(dimStyle.Render("Selected source: "+selected.SourceURL))
	}
	return view
}

func (m Model) localModelsSummary() string {
	installed := 0
	for _, model := range m.localModels.catalog.Models {
		if model.Installed {
			installed++
		}
	}
	if m.localModels.catalog.RuntimeError != "" {
		return "Local models: runtime unavailable · Tab opens the reviewed catalog and recovery guidance."
	}
	return fmt.Sprintf("Local models: %d/%d reviewed models installed · Tab opens pull, use, remove, and rename controls.", installed, len(m.localModels.catalog.Models))
}

func (m Model) cloudModelLimit() int {
	if m.height <= 0 {
		return 8
	}
	return max(1, min(10, (m.height-12)/2))
}

func (m Model) localModelLimit() int {
	if m.height <= 0 {
		return 4
	}
	return max(1, min(4, (m.height-15)/3))
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
	if m.drawerOpen && !m.drawerUsesSidePane() {
		return m.drawerView(m.width, m.height)
	}
	mode := "new thread"
	if m.resumeStatePath != "" {
		mode = "thread " + compact(m.threadID, 12)
	} else if m.forkStatePath != "" {
		mode = "new fork"
	}
	provider := strings.TrimSpace(m.provider.Value())
	if provider == "" {
		provider = "provider not selected"
	}
	if m.delegateRuntime != "" {
		provider += " · " + delegatedRuntimeLabel(m.delegateRuntime)
	}
	model := strings.TrimSpace(m.model.Value())
	if model == "" {
		model = "provider default"
	}
	main := m.conversationView(mode, provider, model, running)
	if !m.drawerUsesSidePane() {
		return main
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, main, m.controlCenterDivider(), m.drawerView(m.drawerWidth(), m.height))
}

func (m Model) conversationView(mode, provider, model string, running bool) string {
	width := m.conversationWidth()
	rail := m.inline(headerStyle.Render(gatorWordmark)+dimStyle.Render("  "+mode+" · "+m.runMode.String())) + "\n" +
		m.inline(dimStyle.Render(compact(provider+" · "+model+" · "+m.effort.label()+" effort · "+m.vimModeLabel(), width)))
	transcript := m.transcript
	transcript.Width = m.transcriptViewportWidth()
	transcript.Height = m.transcriptHeight()
	transcript.SetContent(m.transcriptContent())
	if m.followTranscript {
		transcript.GotoBottom()
	}
	sections := []string{rail, dimStyle.Render(strings.Repeat("─", max(1, width))), m.transcriptViewportView(transcript)}
	if !m.followTranscript {
		hint := "PgDn/End returns to latest"
		if m.transcriptUnread {
			hint = "New activity below · " + hint
		}
		sections = append(sections, keyStyle.Render("↓ ")+dimStyle.Render(hint))
	}
	if m.vimCommand != "" {
		sections = append(sections, keyStyle.Render(m.vimCommand))
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
	if !running {
		if summary := m.extensionUISummary("composer"); summary != "" {
			sections = append(sections, summary)
		}
	}
	if running && m.pendingApproval != nil {
		sections = append(sections, dimStyle.Render(strings.Repeat("─", max(1, width))), m.commandApprovalView())
		if notice := m.noticeView(); notice != "" {
			sections = append(sections, notice)
		}
		sections = append(sections, m.runningFooter())
		return strings.Join(sections, "\n")
	}
	label := "You"
	if running {
		label = "Steer"
	}
	if m.vim != vimOff {
		label += " · " + m.vimModeLabel()
	}
	sections = append(sections, dimStyle.Render(strings.Repeat("─", max(1, width))), labelStyle.Render(label), m.composerInputView())
	if notice := m.noticeView(); notice != "" {
		sections = append(sections, notice)
	}
	if running {
		sections = append(sections, m.runningFooter())
	} else {
		sections = append(sections, m.composerFooter())
	}
	return strings.Join(sections, "\n")
}

func (m Model) transcriptViewportView(transcript viewport.Model) string {
	if !m.showsTranscriptScrollbar(transcript) {
		return transcript.View()
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, transcript.View(), " ", m.transcriptScrollbar(transcript))
}

func (m Model) showsTranscriptScrollbar(transcript viewport.Model) bool {
	return m.conversationWidth() >= minimumScrollbarWidth && transcript.Height > 0 && transcript.TotalLineCount() > transcript.Height
}

func (m Model) transcriptScrollbar(transcript viewport.Model) string {
	height := transcript.Height
	total := transcript.TotalLineCount()
	if height <= 0 || total <= height {
		return ""
	}
	thumbHeight := max(1, min(height, (height*height+total-1)/total))
	maxOffset := height - thumbHeight
	maxScroll := total - height
	thumbOffset := 0
	if maxScroll > 0 && maxOffset > 0 {
		thumbOffset = (transcript.YOffset*maxOffset + maxScroll/2) / maxScroll
	}
	lines := make([]string, height)
	for row := range lines {
		if row >= thumbOffset && row < thumbOffset+thumbHeight {
			lines[row] = keyStyle.Render("█")
		} else {
			lines[row] = dimStyle.Render("│")
		}
	}
	return strings.Join(lines, "\n")
}

// composerInputView avoids textarea's focused-placeholder cursor, which draws
// the cursor over the first placeholder character. The prompt remains visible
// while the input is empty without making it look like the user typed a letter.
func (m Model) composerInputView() string {
	if m.task.Value() != "" {
		return m.task.View()
	}

	prompt := strings.TrimSpace(m.task.Placeholder)
	if prompt == "" {
		prompt = "Message Gator..."
	}
	prefix := ""
	if m.vimNumberState != nil && m.vimNumberState.enabled {
		prefix = dimStyle.Render(m.vimNumberState.emptyPrompt())
	}
	line := prefix + dimStyle.Render(prompt)
	if m.task.Focused() {
		line = prefix + keyStyle.Render("›") + " " + dimStyle.Render(prompt)
	}
	return line + strings.Repeat("\n", max(0, m.task.Height()-1))
}

func (m Model) commandApprovalView() string {
	argv := "(unknown command)"
	if m.pendingApproval != nil && len(m.pendingApproval.argv) > 0 {
		argv = strings.Join(m.pendingApproval.argv, " ")
	}
	return m.fieldView("Approve worktree command", "cwd is the isolated worktree. The process follows this run's configured sandbox policy.", argv+"\n\ny/enter  allow once\na  always allow this argv for this thread\nn  deny")
}

func (m Model) runningFooter() string {
	if m.pendingApproval != nil {
		return m.footer("y/enter once", "a always", "n deny", "ctrl+c/ctrl+x stop", "ctrl+q quit")
	}
	if m.vimCommand != "" {
		return m.footer("enter run Vim command", "esc cancel", "ctrl+c clear", "ctrl+x stop", "ctrl+q quit", "f1 shortcuts")
	}
	switch m.vim {
	case vimNormal:
		return m.footer("i/a edit", ": Ex", "u undo", "ctrl+r redo", "enter steer", "tab queue", "ctrl+c clear", "ctrl+x stop", "ctrl+q quit")
	case vimInsert:
		return m.footer("esc normal", "enter newline", "ctrl+r steer", "tab queue", "ctrl+c clear", "ctrl+x stop", "ctrl+q quit", "f1 shortcuts")
	default:
		return m.footer("ctrl+b controls", "ctrl+t terminal", "enter steer", "tab queue", "pgup/pgdn/wheel browse", "ctrl+c clear", "ctrl+x stop", "ctrl+q quit", "f1 shortcuts")
	}
}

func (m Model) composerFooter() string {
	if m.vimCommand != "" {
		return m.footer("enter execute command", "esc cancel", "ctrl+c clear", "ctrl+q quit", "f1 shortcuts")
	}
	switch m.vim {
	case vimNormal:
		return m.footer("i/a edit", ": Ex", "u undo", "ctrl+r redo", "enter send", "ctrl+c clear", "ctrl+q quit", "? commands", "f1 shortcuts")
	case vimInsert:
		return m.footer("esc normal", "enter newline", "ctrl+r send", "ctrl+b controls", "ctrl+c clear", "ctrl+q quit", "? commands", "f1 shortcuts")
	default:
		return m.footer("ctrl+s effort", "/model models", "ctrl+b controls", "ctrl+o threads", "pgup/pgdn/wheel browse", "end latest", "enter send", "ctrl+c clear", "ctrl+q quit", "? commands")
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
	steering := "Enter steers at a model or tool boundary"
	mode := m.runMode.String()
	if m.runMode == gatorrun.PlanMode {
		mode += " read-only"
	}
	turn := m.activity.turn
	if turn < 1 {
		turn = 1
	}
	facts := fmt.Sprintf("%s · %s effort · Gator-owned loop · isolated worktree · turn %d/%d", mode, m.effort.label(), turn, m.effort.maxSteps(m.config.MaxSteps))
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
	case strings.Contains(message, "api"), strings.Contains(message, "credential"), strings.Contains(message, "authentication"):
		return "The model provider stopped before completion. Check provider configuration or credentials, then retry the task."
	default:
		return "Inspect the transcript and retained worktree with /review, then send a focused follow-up."
	}
}

func (m Model) vimCommandView() string {
	return labelStyle.Render("Vim command") + "\n" + m.panel(keyStyle.Render(m.vimCommand)+"\n"+dimStyle.Render(":w send · :wq/:x send then exit · :q! discard and exit · :set line numbers"))
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
			"/tree  view retained turn lineage",
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
			"/tree  view the retained linear turn lineage",
			"PgUp / PgDn  browse the conversation",
			"/run  edit advanced run options",
			"/doctor  inspect local diagnostics",
			"/agents  list narrowing profiles and prompt-only roles",
			"@  begin a repository-path reference",
			"Ctrl+Space (Ctrl+@)  reopen @ path suggestions",
			"Tab  complete a command or insert a path; Enter runs a command",
			"/vim  toggle Vim Normal/Insert message editing",
			"Vim Normal  counts; h/j/k/l, 0/^/$, w/b/e, gg/G; i/I/a/A/o/O; d/c/y + motions; p/P; u/Ctrl+R",
			"Vim numbers  hybrid absolute/relative by default; :set number, relativenumber, nonumber, or norelativenumber",
			"Vim Ex  :w send · :wq or :x send then exit · :q! discard and exit · :help list supported commands",
			"Ctrl+C  quit",
		}, "\n")),
		labelStyle.Render("Running") + "\n" + m.panel("Enter  steer at the next model/tool boundary\nTab  queue the next prompt or a slash command\nCtrl+T  attach to a model-started terminal task\nCommand approval  y/enter once · a always this argv · n deny\n/queue, /dequeue, /clear-queue  inspect or manage local queued work\n/tree  view retained prior turns while a continuation runs\nPgUp / PgDn  browse conversation\nCtrl+C  request cancellation and retain the worktree"),
		labelStyle.Render("Review") + "\n" + m.panel("F1  show this help\nup/down, j/k, PgUp/PgDn  scroll the diff\nf  toggle focused duplicate-block review / full patch\nc or Esc  return to conversation\nd  refresh the diff\nt  view this run's transcript\ny  view retained thread lineage\ne  show patch export/apply commands\nn  start a new task\nq or Ctrl+C  quit"),
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
	if m.reviewLoaded {
		return m.structuredReviewView()
	}
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
	sections = append(sections, m.footer("up/down scroll diff", "f focused/full", "d refresh diff", "t transcript", "y thread tree", "e patch handoff", continueLabel, "n new task", "f1 shortcuts", "q quit"))
	return strings.Join(sections, "\n")
}

func (m Model) threadTreeView() string {
	if len(m.threadTurns) == 0 {
		return m.header("thread tree") + "\n" + m.panel(dimStyle.Render("No retained turns are available.")) + "\n" + m.footer("esc return")
	}
	threadID := m.threadID
	if threadID == "" {
		threadID = m.threadTurns[len(m.threadTurns)-1].ThreadID
	}
	title := "thread tree"
	if threadID != "" {
		title += " · " + compact(threadID, 16)
	}
	limit := m.threadTreeLimit()
	start, end := m.visibleRange(len(m.threadTurns), m.threadIndex, limit)
	lines := make([]string, 0, (end-start)*2)
	for index := start; index < end; index++ {
		turn := m.threadTurns[index]
		prefix := "  "
		if index == m.threadIndex {
			prefix = "> "
		}
		branch := "├─"
		if index == len(m.threadTurns)-1 {
			branch = "└─"
		}
		modelName := turn.Model
		if modelName == "" {
			modelName = "provider default"
		}
		mode := turn.Mode
		if mode == "" {
			mode = "execute"
		}
		facts := fmt.Sprintf("%02d · %s · %s · %s", index+1, mode, turn.Provider, modelName)
		lines = append(lines, prefix+branch+" "+keyStyle.Render(compact(facts, m.panelTextWidth()-4)))
		status := threadTurnStatusView(turn.Status)
		timestamp := threadTurnTimestamp(turn)
		lines = append(lines, "    "+status+dimStyle.Render(" · "+timestamp+" · ")+dimStyle.Render(compact(turn.Task, max(12, m.panelTextWidth()-26))))
		for _, fork := range m.threadForks[turn.StatePath] {
			label := "fork " + compact(fork.ID, 12)
			if fork.Available {
				label += " · available"
			} else {
				label += " · worktree missing"
			}
			lines = append(lines, "    ↳ "+keyStyle.Render(label)+" · "+dimStyle.Render(compact(fork.Task, max(12, m.panelTextWidth()-28))))
		}
	}
	selected := m.threadTurns[min(max(0, m.threadIndex), len(m.threadTurns)-1)]
	sections := []string{
		m.header(title),
		m.inline(dimStyle.Render(compact("Select a turn and press f to fork it into a new isolated worktree.", m.inlineWidth()))),
		m.panel(strings.Join(lines, "\n")),
		labelStyle.Render(fmt.Sprintf("Selected turn %d", m.threadIndex+1)),
		m.panel(compact(selected.Task, max(16, m.panelTextWidth()*2))),
	}
	if strings.TrimSpace(selected.FinalText) != "" && !m.compactLayout() {
		sections = append(sections, labelStyle.Render("Result"), m.panel(compact(selected.FinalText, max(16, m.panelTextWidth()*3))))
	}
	footer := m.footer("up/down select", "f fork selected", "c clone current", "r refresh", "y/esc return", "f1 shortcuts")
	if m.compactLayout() {
		footer = m.footer("up/down select", "y/esc return", "f1 help")
	}
	sections = append(sections, m.noticeView(), footer)
	return strings.Join(sections, "\n")
}

func (m Model) threadTreeLimit() int {
	if m.height <= 0 {
		return 6
	}
	return max(1, (m.height-13)/2)
}

func threadTurnStatusView(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "retained"
	}
	switch status {
	case "completed":
		return okStyle.Render("✓ completed")
	case "failed":
		return errorStyle.Render("✗ failed")
	default:
		return dimStyle.Render("○ " + status)
	}
}

func threadTurnTimestamp(turn journal.ThreadTurn) string {
	at := turn.FinishedAt
	if at.IsZero() {
		at = turn.StartedAt
	}
	if at.IsZero() {
		return "time unavailable"
	}
	return at.Local().Format("Jan 2 15:04")
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
