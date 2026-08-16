package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/attachment"
	modelprovider "github.com/gongahkia/gator/internal/model"
	gatorrun "github.com/gongahkia/gator/internal/run"
)

func (m Model) commandPaletteVisible() bool {
	return len(m.matchingCommands()) > 0
}

func (m Model) matchingCommands() []slashCommand {
	if m.focus != taskField {
		return nil
	}
	return matchingSlashCommands(m.task.Value())
}

func (m *Model) normalizeCommandSelection() {
	matches := m.matchingCommands()
	if len(matches) == 0 {
		m.commandIndex = 0
		return
	}
	if m.commandIndex >= len(matches) {
		m.commandIndex = len(matches) - 1
	}
}

func (m *Model) moveCommandSelection(delta int) {
	matches := m.matchingCommands()
	if len(matches) == 0 {
		m.commandIndex = 0
		return
	}
	m.commandIndex = (m.commandIndex + delta + len(matches)) % len(matches)
}

// completeSelectedCommand only fills the message composer. Enter remains the
// explicit action that runs a local slash command.
func (m *Model) completeSelectedCommand() {
	matches := m.matchingCommands()
	if len(matches) == 0 {
		return
	}
	m.task.SetValue(matches[m.commandIndex].name)
	m.commandIndex = 0
	m.persistDraft()
	m.refreshPreflight()
}

func (m Model) executeSelectedCommand() (tea.Model, tea.Cmd) {
	matches := m.matchingCommands()
	if len(matches) == 0 {
		return m, nil
	}
	defer m.persistDraft()
	defer m.refreshPreflight()
	command := matches[m.commandIndex]
	m.task.Reset()
	m.commandIndex = 0
	switch command.name {
	case "/clear":
		m.commandOutput = ""
		m.notice = notice{text: "Task cleared.", kind: noticeInfo}
	case "/help":
		m.commandOutput = commandHelp()
		m.notice = notice{text: "Commands operate locally and never start a run by themselves.", kind: noticeInfo}
	case "/plan":
		m.runMode = gatorrun.PlanMode
		m.notice = notice{text: "Plan mode is read-only: it can inspect the worktree but cannot edit files or run commands.", kind: noticeInfo}
	case "/execute":
		m.runMode = gatorrun.ExecuteMode
		m.notice = notice{text: "Execute mode will use the configured verifier policy after making changes.", kind: noticeInfo}
	case "/new":
		m.returnToComposer()
		return m, m.focusField()
	case "/model":
		m.commandOutput = ""
		m.focus = modelField
		m.notice = notice{text: "Choose a recommended model or type a model ID, then Tab back to the task.", kind: noticeInfo}
		return m, m.focusField()
	case "/provider":
		m.commandOutput = ""
		m.focus = providerField
		m.notice = notice{text: "Choose a provider or type to filter it, then Tab back to the task.", kind: noticeInfo}
		return m, m.focusField()
	case "/permissions":
		m.commandOutput = m.permissionsStatus()
		if isExternalProvider(m.provider.Value()) {
			m.notice = notice{text: "This provider delegates tool permissions to its vendor CLI; Gator verifies the final worktree.", kind: noticeInfo}
		} else {
			m.notice = notice{text: "Verifier commands are the only commands the agent may run.", kind: noticeInfo}
		}
	case "/quit":
		return m, tea.Quit
	case "/review":
		if m.outcome == nil || m.outcome.Worktree.Path == "" {
			m.notice = notice{text: "No completed or retained run is available to review yet.", kind: noticeError}
			return m, nil
		}
		m.screen = reviewScreen
		return m, nil
	case "/recent", "/threads":
		return m.openRecentRuns()
	case "/status":
		m.commandOutput = m.sessionStatus()
		m.notice = notice{text: "Current configuration shown below.", kind: noticeInfo}
	case "/verify":
		m.commandOutput = ""
		m.focus = verificationField
		m.notice = notice{text: "Edit the allowed verification commands, one argv per line.", kind: noticeInfo}
		return m, m.focusField()
	case "/worktree":
		m.commandOutput = "Every new Gator run creates a detached worktree beside this repository. The agent can edit only that worktree; your active checkout stays unchanged."
		m.notice = notice{text: "Worktree isolation is always on for new runs.", kind: noticeInfo}
	}
	return m, nil
}

func (m Model) commandPaletteView() string {
	matches := m.matchingCommands()
	if len(matches) == 0 {
		if strings.HasPrefix(strings.TrimSpace(m.task.Value()), "/") {
			return errorStyle.Render("No Gator command matches this input.")
		}
		return ""
	}
	start, end := m.visibleRange(len(matches), m.commandIndex, m.popupLimit())
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		command := matches[index]
		prefix := "  "
		if index == m.commandIndex {
			prefix = "> "
		}
		line := prefix + keyStyle.Render(command.name)
		if !m.compactLayout() {
			line += "  " + dimStyle.Render(command.description)
		}
		lines = append(lines, line)
	}
	return labelStyle.Render("Commands") + "\n" + m.panel(strings.Join(lines, "\n")) + "\n" + m.inline(dimStyle.Render("up/down choose · tab complete · enter run command · esc dismiss"))
}

type dropdownOption struct {
	value       string
	label       string
	description string
	custom      bool
}

func providerDropdownOptions() []dropdownOption {
	descriptions := map[modelprovider.Provider]string{
		modelprovider.OpenAI:           "OpenAI Responses API",
		modelprovider.AzureOpenAI:      "Azure OpenAI Chat Completions",
		modelprovider.Anthropic:        "Anthropic Messages API",
		modelprovider.Gemini:           "Gemini GenerateContent API",
		modelprovider.Mistral:          "Mistral Chat Completions",
		modelprovider.XAI:              "xAI Chat Completions",
		modelprovider.Groq:             "Groq Chat Completions",
		modelprovider.OpenRouter:       "OpenRouter Chat Completions",
		modelprovider.Together:         "Together AI Chat Completions",
		modelprovider.Fireworks:        "Fireworks Chat Completions",
		modelprovider.DeepSeek:         "DeepSeek Chat Completions",
		modelprovider.OpenAICompatible: "custom Chat Completions endpoint",
		modelprovider.Codex:            "local Codex CLI subscription",
		modelprovider.Claude:           "local Claude Code subscription",
		modelprovider.Copilot:          "local GitHub Copilot CLI subscription",
		modelprovider.Cursor:           "local Cursor Agent CLI subscription",
	}
	options := make([]dropdownOption, 0, len(descriptions))
	for _, name := range modelprovider.Names() {
		provider, err := modelprovider.ParseProvider(name)
		if err != nil {
			continue
		}
		options = append(options, dropdownOption{value: name, description: descriptions[provider]})
	}
	return options
}

// modelDropdownOptions deliberately offers only model IDs Gator can recommend
// without guessing an arbitrary provider catalog. The text field remains
// editable for deployments, aliases, previews, and account-specific models.
func modelDropdownOptions(providerName string) []dropdownOption {
	provider, err := modelprovider.ParseProvider(providerName)
	if err != nil {
		return nil
	}
	customDescription := "type a model ID supported by this provider"
	if provider == modelprovider.AzureOpenAI {
		customDescription = "type the Azure deployment name"
	}
	if modelprovider.IsHarness(provider) {
		return []dropdownOption{
			{value: "", label: "provider default", description: "use the default configured in the vendor CLI"},
			{label: "custom model ID", description: "type a model selector supported by the vendor CLI", custom: true},
		}
	}
	options := []dropdownOption{{label: "custom model ID", description: customDescription, custom: true}}
	if defaultModel := modelprovider.DefaultModel(provider); defaultModel != "" {
		options = append([]dropdownOption{{value: defaultModel, label: defaultModel, description: "Gator recommended default"}}, options...)
	}
	return options
}

func matchingDropdownOptions(options []dropdownOption, query string) []dropdownOption {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return options
	}
	for _, option := range options {
		if option.custom {
			continue
		}
		if strings.EqualFold(option.value, query) {
			return options
		}
	}
	var matches []dropdownOption
	for _, option := range options {
		if option.custom {
			continue
		}
		if strings.HasPrefix(strings.ToLower(option.value), query) {
			matches = append(matches, option)
		}
	}
	for _, option := range options {
		if option.custom {
			matches = append(matches, option)
		}
	}
	return matches
}

func (m Model) dropdownOptions() []dropdownOption {
	switch m.focus {
	case providerField:
		return matchingDropdownOptions(providerDropdownOptions(), m.provider.Value())
	case modelField:
		return matchingDropdownOptions(modelDropdownOptions(m.provider.Value()), m.model.Value())
	default:
		return nil
	}
}

func (m Model) dropdownVisible() bool {
	return m.resumeStatePath == "" && (m.focus == providerField || m.focus == modelField) && len(m.dropdownOptions()) > 0
}

func (m *Model) normalizeDropdownSelection() {
	options := m.dropdownOptions()
	if len(options) == 0 {
		m.dropdownIndex = 0
		return
	}
	value := ""
	switch m.focus {
	case providerField:
		value = m.provider.Value()
	case modelField:
		value = m.model.Value()
	}
	for index, option := range options {
		if strings.EqualFold(option.value, strings.TrimSpace(value)) {
			m.dropdownIndex = index
			return
		}
	}
	if m.dropdownIndex >= len(options) {
		m.dropdownIndex = len(options) - 1
	}
}

func (m *Model) moveDropdownSelection(delta int) {
	options := m.dropdownOptions()
	if len(options) == 0 {
		m.dropdownIndex = 0
		return
	}
	m.dropdownIndex = (m.dropdownIndex + delta + len(options)) % len(options)
}

func (m *Model) applySelectedDropdown() {
	options := m.dropdownOptions()
	if len(options) == 0 {
		return
	}
	selected := options[m.dropdownIndex]
	switch m.focus {
	case providerField:
		m.provider.SetValue(selected.value)
		provider, err := modelprovider.ParseProvider(selected.value)
		if err == nil {
			m.model.SetValue(modelprovider.DefaultModel(provider))
		}
		m.notice = notice{text: "Provider selected: " + selected.value, kind: noticeInfo}
	case modelField:
		if selected.custom {
			if strings.TrimSpace(m.model.Value()) == "" {
				m.notice = notice{text: "Enter a model ID supported by the selected provider.", kind: noticeInfo}
			} else {
				m.notice = notice{text: "Custom model retained: " + strings.TrimSpace(m.model.Value()), kind: noticeInfo}
			}
			m.normalizeDropdownSelection()
			m.persistDraft()
			m.refreshPreflight()
			return
		}
		m.model.SetValue(selected.value)
		if selected.value == "" {
			m.notice = notice{text: "The provider CLI will choose its configured model.", kind: noticeInfo}
		} else {
			m.notice = notice{text: "Model selected: " + selected.value, kind: noticeInfo}
		}
	}
	m.normalizeDropdownSelection()
	m.persistDraft()
	m.refreshPreflight()
}

func (m Model) dropdownView() string {
	if !m.dropdownVisible() {
		return ""
	}
	options := m.dropdownOptions()
	start, end := m.visibleRange(len(options), m.dropdownIndex, m.popupLimit())
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		option := options[index]
		prefix := "  "
		if index == m.dropdownIndex {
			prefix = "> "
		}
		label := option.label
		if label == "" {
			label = option.value
		}
		if label == "" {
			label = "provider default"
		}
		line := prefix + keyStyle.Render(label)
		if !m.compactLayout() {
			line += "  " + dimStyle.Render(option.description)
		}
		lines = append(lines, line)
	}
	title := "Provider choices"
	if m.focus == modelField {
		title = "Recommended models"
	}
	return labelStyle.Render(title) + "\n" + m.panel(strings.Join(lines, "\n")) + "\n" + m.inline(dimStyle.Render("type to filter · up/down choose · enter apply · tab apply and continue"))
}

func (m *Model) normalizeContextSelection() {
	if _, active := activeContextCompletion(m.task.Value()); !active {
		m.contextIndex = 0
		return
	}
	if !m.contextLoaded {
		m.contextPaths, m.contextErr = contextCompletionCandidates(m.config.RepositoryPath)
		m.contextLoaded = true
	}
	matches := m.contextCompletions()
	if len(matches) == 0 {
		m.contextIndex = 0
		return
	}
	if m.contextIndex >= len(matches) {
		m.contextIndex = len(matches) - 1
	}
}

func (m Model) contextCompletions() []string {
	active, ok := activeContextCompletion(m.task.Value())
	if !ok || m.contextClosed || m.contextErr != nil {
		return nil
	}
	return matchingContextCompletions(m.contextPaths, active.query)
}

func (m Model) contextCompletionVisible() bool {
	return m.focus == taskField && len(m.contextCompletions()) > 0
}

func (m *Model) moveContextSelection(delta int) {
	matches := m.contextCompletions()
	if len(matches) == 0 {
		m.contextIndex = 0
		return
	}
	m.contextIndex = (m.contextIndex + delta + len(matches)) % len(matches)
}

func (m *Model) applySelectedContextCompletion() {
	active, ok := activeContextCompletion(m.task.Value())
	matches := m.contextCompletions()
	if !ok || len(matches) == 0 {
		return
	}
	selected := matches[m.contextIndex]
	m.task.SetValue(m.task.Value()[:active.start] + contextToken(selected) + " ")
	m.contextIndex = 0
	m.contextClosed = false
	m.persistDraft()
	m.refreshPreflight()
}

func (m Model) contextCompletionView() string {
	matches := m.contextCompletions()
	if len(matches) == 0 {
		return ""
	}
	start, end := m.visibleRange(len(matches), m.contextIndex, m.popupLimit())
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		candidate := matches[index]
		prefix := "  "
		if index == m.contextIndex {
			prefix = "> "
		}
		lines = append(lines, prefix+keyStyle.Render("["+contextBadge(candidate)+"]")+"  "+"@"+compact(candidate, max(8, m.panelTextWidth()-8)))
	}
	return labelStyle.Render("Path suggestions") + "\n" + m.panel(strings.Join(lines, "\n")) + "\n" + m.inline(dimStyle.Render("type to filter · up/down choose · enter or tab insert · esc dismiss"))
}

func (m Model) contextReferencesView() string {
	references := extractContextReferences(m.task.Value())
	if len(references) == 0 {
		return ""
	}
	values := make([]string, 0, len(references))
	for _, reference := range references {
		label := "@" + reference
		if imageMediaType(reference) != "" {
			label = "[image] " + label
		} else if attachment.IsSupported(reference) {
			label = "[document] " + label
		}
		values = append(values, keyStyle.Render(label))
	}
	return labelStyle.Render("Context references") + "\n" + m.panel(strings.Join(values, "  ")) + "\n" + m.inline(dimStyle.Render("Images, PDFs, and supported documents attach to native-provider turns; other paths are inspected first."))
}

func (m Model) sessionStatus() string {
	verification, err := parseVerification(m.verification.Value())
	verificationText := "not configured"
	if err == nil {
		verificationText = formatVerification(verification)
	} else if m.runMode == gatorrun.ExecuteMode {
		verificationText = "invalid: " + err.Error()
	}
	return "repository: " + m.config.RepositoryPath + "\nmode: " + m.runMode.String() + "\nprovider: " + m.provider.Value() + "\nmodel: " + m.model.Value() + "\nmax steps: " + fmt.Sprint(m.config.MaxSteps) + "\nverification:\n" + verificationText
}

func (m Model) permissionsStatus() string {
	if m.runMode == gatorrun.PlanMode {
		return "mode: enforced Plan\nwrites: disabled\ncommands: disabled\nreads: repository paths and Git state only\nactive checkout: never edited by a normal run"
	}
	verification, err := parseVerification(m.verification.Value())
	commands := "invalid verifier configuration: " + err.Error()
	if err == nil {
		commands = formatVerification(verification)
	}
	if isExternalProvider(m.provider.Value()) {
		return "writes: isolated run worktree only\nprovider: delegated CLI with its own permission policy\nGator runs required verification after the CLI exits:\n" + commands + "\nactive checkout: never edited by a normal run"
	}
	return "writes: isolated run worktree only\nreads: repository paths only\ncommands allowed:\n" + commands + "\nactive checkout: never edited by a normal run"
}

func commandHelp() string {
	lines := make([]string, 0, len(slashCommands))
	for _, command := range slashCommands {
		lines = append(lines, command.name+" — "+command.description)
	}
	return strings.Join(lines, "\n")
}
