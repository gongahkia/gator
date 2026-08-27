package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/attachment"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/instructions"
	"github.com/gongahkia/gator/internal/journal"
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
	matches := matchingSlashCommands(m.task.Value())
	query := strings.TrimSpace(m.task.Value())
	if !strings.HasPrefix(query, "/") || strings.Contains(query, "\n") {
		return matches
	}
	query = strings.TrimPrefix(query, "/")
	if fields := strings.Fields(query); len(fields) > 0 {
		query = fields[0]
	}
	for _, command := range m.extensionSlashCommands() {
		if strings.HasPrefix(strings.TrimPrefix(command.name, "/"), query) {
			matches = append(matches, command)
		}
	}
	return matches
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

func (m *Model) toggleVimMode() {
	if m.vim == vimOff {
		m.vim = vimNormal
		m.vimAbsoluteNumbers = true
		m.vimRelativeNumbers = true
		m.resetVimPending()
		m.syncVimLineNumbers()
		m.notice = notice{text: "Vim Normal mode enabled. Hybrid line numbers are on; use :set number or :set relativenumber to configure them.", kind: noticeInfo}
		return
	}
	m.vim = vimOff
	m.resetVimPending()
	m.syncVimLineNumbers()
	m.notice = notice{text: "Vim mode disabled. Enter now sends the message.", kind: noticeInfo}
}

func (m Model) executeSelectedCommand() (tea.Model, tea.Cmd) {
	matches := m.matchingCommands()
	if len(matches) == 0 {
		return m, nil
	}
	defer m.persistDraft()
	defer m.refreshPreflight()
	command := matches[m.commandIndex]
	raw := strings.TrimSpace(m.task.Value())
	remainder := strings.TrimSpace(strings.TrimPrefix(raw, command.name))
	arguments := strings.Fields(raw)
	m.task.Reset()
	m.commandIndex = 0
	switch command.name {
	default:
		if command.prompt != "" {
			m.task.SetValue(command.prompt)
			m.notice = notice{text: "Extension template loaded. Review it, then send it explicitly.", kind: noticeInfo}
			return m, m.focusField()
		}
		m.notice = notice{text: "Unknown local command.", kind: noticeError}
		return m, nil
	case "/clear":
		m.commandOutput = ""
		m.notice = notice{text: "Task cleared.", kind: noticeInfo}
	case "/clear-queue":
		removed := len(m.queue)
		m.queue = nil
		m.commandOutput = "No prompts queued."
		m.notice = notice{text: fmt.Sprintf("Removed %d queued instruction(s).", removed), kind: noticeInfo}
	case "/dequeue":
		if len(m.queue) == 0 {
			m.commandOutput = "No prompts queued."
			m.notice = notice{text: "There is no queued instruction to remove.", kind: noticeInfo}
			break
		}
		removed := m.queue[0]
		m.queue = m.queue[1:]
		m.commandOutput = m.queueStatus()
		m.notice = notice{text: "Removed next queued " + queuedInputLabel(removed) + ".", kind: noticeInfo}
	case "/help":
		m.commandOutput = commandHelp()
		m.notice = notice{text: "Commands operate locally and never start a run by themselves.", kind: noticeInfo}
	case "/clone":
		if m.resumeStatePath == "" {
			m.notice = notice{text: "Continue a retained thread before cloning it.", kind: noticeInfo}
			return m, nil
		}
		return m.beginFork(m.resumeStatePath)
	case "/compact":
		if m.resumeStatePath == "" && m.forkStatePath == "" {
			m.notice = notice{text: "Start, continue, or fork a retained thread before compacting its context.", kind: noticeInfo}
			return m, nil
		}
		m.forceCompaction = true
		m.commandOutput = "The next retained-thread run will summarize older context before it starts. The source transcript remains available in its run records."
		m.notice = notice{text: "Older retained context will be compacted before the next turn.", kind: noticeInfo}
	case "/copy":
		return m.copyLatestAgentOutput()
	case "/copyall":
		return m.copyConversationTranscript()
	case "/fork":
		if m.resumeStatePath == "" {
			m.notice = notice{text: "Continue a retained thread before choosing a turn to fork.", kind: noticeInfo}
			return m, nil
		}
		return m.openThreadTree(m.screen)
	case "/plan":
		m.runMode = gatorrun.PlanMode
		if m.delegateRuntime != "" {
			m.notice = notice{text: delegatedRuntimeLabel(m.delegateRuntime) + " does not support Gator plan mode. Use /execute before sending a task.", kind: noticeInfo}
		} else {
			m.notice = notice{text: "Plan mode is read-only: it can inspect the worktree but cannot edit files or run commands.", kind: noticeInfo}
		}
	case "/execute":
		m.runMode = gatorrun.ExecuteMode
		m.notice = notice{text: "Execute mode will use the configured verifier policy after making changes.", kind: noticeInfo}
	case "/effort":
		return m.openEffortPicker()
	case "/extensions":
		return m.openExtensionUI(m.screen)
	case "/new":
		m.returnToComposer()
		return m, m.focusField()
	case "/model":
		m.commandOutput = ""
		m.notice = notice{text: "Choose and manage cloud or local models. Provider sign-in is available from the Cloud section.", kind: noticeInfo}
		return m.openModelCatalog()
	case "/manage":
		m.commandOutput = ""
		m.notice = notice{text: "Inspect local state before changing trust or retained artifacts.", kind: noticeInfo}
		return m.openManagement()
	case "/doctor":
		m.commandOutput = ""
		return m.openDoctor()
	case "/run":
		m.commandOutput = ""
		return m.openRunOptions()
	case "/agents":
		m.commandOutput = m.agentsStatus()
		m.notice = notice{text: "Profiles can only narrow policy. Roles remain prompt-only specializations.", kind: noticeInfo}
	case "/permissions":
		m.commandOutput = m.permissionsStatus()
		m.notice = notice{text: "Verifier, exact allow-command, and literal prefixes may run without a prompt; everything else waits.", kind: noticeInfo}
	case "/quit":
		return m, tea.Quit
	case "/queue":
		m.commandOutput = m.queueStatus()
		m.notice = notice{text: m.queueSummary() + ". Queued instructions are local to this TUI session.", kind: noticeInfo}
	case "/review":
		if m.outcome == nil || m.outcome.Worktree.Path == "" {
			m.notice = notice{text: "No completed or retained run is available to review yet.", kind: noticeError}
			return m, nil
		}
		m.screen = reviewScreen
		m.notice = notice{text: "Refreshing the retained-worktree review…", kind: noticeInfo}
		return m, loadReview(*m.outcome)
	case "/recent", "/threads":
		return m.openRecentRuns()
	case "/tree":
		return m.openThreadTree(m.screen)
	case "/status":
		m.commandOutput = m.sessionStatus()
		m.notice = notice{text: "Current configuration shown below.", kind: noticeInfo}
	case "/theme":
		if len(arguments) == 1 {
			m.commandOutput = "Current theme: " + m.config.Theme + "\n\nChoose one:\n  /theme gator\n  /theme contrast\n  /theme mono"
			m.notice = notice{text: "Choose a named theme; the change is saved immediately.", kind: noticeInfo}
			break
		}
		if len(arguments) != 2 || m.config.SetTheme == nil {
			m.notice = notice{text: "Use /theme gator, /theme contrast, or /theme mono.", kind: noticeError}
			break
		}
		if err := m.config.SetTheme(arguments[1]); err != nil {
			m.notice = notice{text: err.Error(), kind: noticeError}
			break
		}
		m.config.Theme = applyTheme(arguments[1])
		m.commandOutput = "Theme set to " + m.config.Theme + "."
		m.notice = notice{text: "Theme saved and applied.", kind: noticeSuccess}
	case "/verify":
		m.commandOutput = ""
		m.notice = notice{text: "Edit the allowed verification commands, one argv per line.", kind: noticeInfo}
		return m, m.openRuntimeDrawer(verificationField)
	case "/vim":
		m.toggleVimMode()
	case "/worktree":
		m.commandOutput = "Every new Gator run creates a detached worktree beside this repository. The agent can edit only that worktree; your active checkout stays unchanged."
		m.notice = notice{text: "Worktree isolation is always on for new runs.", kind: noticeInfo}
	}
	return m, nil
}

func (m Model) startOAuthLogin(providerName string) (tea.Model, tea.Cmd) {
	if custom, found := m.customProvider(providerName); found {
		if custom.APIKeyEnv == "" {
			m.notice = notice{text: "Custom provider " + custom.ID + " is configured without an API key.", kind: noticeInfo}
		} else {
			m.notice = notice{text: "Set " + custom.APIKeyEnv + " before running custom provider " + custom.ID + ".", kind: noticeInfo}
		}
		return m, nil
	}
	provider, err := modelprovider.ParseProvider(providerName)
	if err != nil {
		m.notice = notice{text: err.Error(), kind: noticeError}
		return m, nil
	}
	if !modelprovider.SupportsOAuthLogin(provider) {
		m.notice = notice{text: "Provider " + string(provider) + " uses " + modelprovider.CredentialHint(provider) + ".", kind: noticeInfo}
		return m, nil
	}
	if m.oauthLogin != nil {
		m.notice = notice{text: "OAuth login is already waiting for a browser callback. Press Ctrl+C to cancel it.", kind: noticeInfo}
		return m, nil
	}
	if command := delegatedConnectCommand(string(provider)); command != "" {
		clientIDEnvironment := oauthClientIDEnvironment(string(provider))
		useVendorConnection := provider == modelprovider.Claude || (clientIDEnvironment != "" && strings.TrimSpace(os.Getenv(clientIDEnvironment)) == "")
		if useVendorConnection {
			if m.config.NewConnectCommand != nil {
				process, processErr := m.config.NewConnectCommand(string(provider))
				if processErr != nil {
					m.notice = notice{text: processErr.Error(), kind: noticeError}
					return m, nil
				}
				m.notice = notice{text: "Opening the provider-owned sign-in in this terminal...", kind: noticeInfo}
				return m, tea.ExecProcess(process, func(err error) tea.Msg {
					return connectDoneMsg{provider: string(provider), err: err}
				})
			}
			if provider == modelprovider.Claude {
				m.commandOutput = "Claude Code uses an Anthropic API key in Gator. Gator does not use Claude.ai or Claude Code's stored subscription credential.\n\nExit this TUI and run:\n  " + command + "\n\nThen send a task through the Claude Code harness."
			} else {
				m.commandOutput = "Native Gator OAuth requires " + clientIDEnvironment + ".\n\nFor the no-registration vendor-CLI route, exit this TUI and run:\n  " + command + "\n\nThen use the matching gator delegate runtime."
			}
			m.notice = notice{text: "Use the vendor-CLI connection command shown below, or configure Gator's own OAuth client.", kind: noticeInfo}
			return m, nil
		}
	}
	if m.config.BeginOAuthLogin == nil {
		m.notice = notice{text: "OAuth login is not configured for this Gator build.", kind: noticeError}
		return m, nil
	}
	login, err := m.config.BeginOAuthLogin(string(provider))
	if err != nil {
		m.notice = notice{text: err.Error(), kind: noticeError}
		return m, nil
	}
	context, cancel := context.WithCancel(context.Background())
	m.oauthLogin = login
	m.oauthCancel = cancel
	m.oauthProvider = string(provider)
	m.commandOutput = "Open this URL to sign Gator in:\n" + login.URL()
	m.notice = notice{text: "Waiting for the browser callback. Press Ctrl+C to cancel OAuth login.", kind: noticeInfo}
	return m, func() tea.Msg {
		return oauthLoginDoneMsg{provider: string(provider), err: login.Complete(context)}
	}
}

func (m Model) openThreadTree(returnScreen screen) (tea.Model, tea.Cmd) {
	statePath := strings.TrimSpace(m.resumeStatePath)
	if statePath == "" {
		m.notice = notice{text: "No retained thread is available yet. Start or continue a run first.", kind: noticeInfo}
		return m, nil
	}
	turns, err := journal.LoadThreadLineage(statePath)
	if err != nil {
		m.notice = notice{text: "Load retained thread: " + err.Error(), kind: noticeError}
		return m, nil
	}
	if len(turns) == 0 {
		m.notice = notice{text: "No retained turns are available for this thread.", kind: noticeInfo}
		return m, nil
	}
	m.threadTurns = turns
	forks, err := journal.ListThreadForks(m.config.StateDir, m.config.RepositoryPath)
	if err != nil {
		m.notice = notice{text: "Load thread forks: " + err.Error(), kind: noticeError}
		return m, nil
	}
	m.threadForks = forks
	m.threadIndex = len(turns) - 1
	m.threadReturn = returnScreen
	m.screen = threadScreen
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

func providerDropdownOptions(stateDir string, customProviders []config.CustomProvider) []dropdownOption {
	descriptions := map[modelprovider.Provider]string{
		modelprovider.OpenAI:                  "OpenAI Responses API",
		modelprovider.AzureOpenAI:             "Azure OpenAI Chat Completions",
		modelprovider.AzureOpenAIResponses:    "Azure OpenAI Responses API",
		modelprovider.Anthropic:               "Anthropic Messages API",
		modelprovider.Gemini:                  "Gemini GenerateContent API",
		modelprovider.Mistral:                 "Mistral Chat Completions",
		modelprovider.XAI:                     "xAI API key or Grok/X account OAuth",
		modelprovider.Groq:                    "Groq Chat Completions",
		modelprovider.OpenRouter:              "OpenRouter API key or browser-minted key",
		modelprovider.Together:                "Together AI Chat Completions",
		modelprovider.Fireworks:               "Fireworks Chat Completions",
		modelprovider.DeepSeek:                "DeepSeek Chat Completions",
		modelprovider.Cerebras:                "Cerebras OpenAI-compatible Chat Completions",
		modelprovider.NVIDIA:                  "NVIDIA NIM OpenAI-compatible Chat Completions",
		modelprovider.HuggingFace:             "Hugging Face Inference Providers Chat Completions",
		modelprovider.MoonshotAI:              "Moonshot AI Kimi OpenAI-compatible Chat Completions",
		modelprovider.ZAI:                     "Z.AI GLM Coding Plan Chat Completions",
		modelprovider.ZAICodingCN:             "Z.AI GLM Coding Plan China Chat Completions",
		modelprovider.MiniMax:                 "MiniMax Anthropic-compatible Messages API",
		modelprovider.MiniMaxCN:               "MiniMax China Anthropic-compatible Messages API",
		modelprovider.Baseten:                 "Baseten OpenAI-compatible Chat Completions",
		modelprovider.VercelAIGateway:         "Vercel AI Gateway OpenAI-compatible Chat Completions",
		modelprovider.AntLing:                 "Ant Ling OpenAI-compatible Chat Completions",
		modelprovider.Xiaomi:                  "Xiaomi MiMo OpenAI-compatible Chat Completions",
		modelprovider.MoonshotAICN:            "Moonshot AI Kimi China OpenAI-compatible Chat Completions",
		modelprovider.CloudflareWorkers:       "Cloudflare Workers AI Chat Completions",
		modelprovider.CloudflareGateway:       "Cloudflare AI Gateway Chat Completions",
		modelprovider.AmazonBedrock:           "Amazon Bedrock Chat Completions",
		modelprovider.GoogleVertex:            "Google Vertex AI Chat Completions (ADC)",
		modelprovider.QwenTokenPlan:           "Qwen Token Plan OpenAI-compatible Chat Completions",
		modelprovider.QwenTokenPlanCN:         "Qwen Token Plan China OpenAI-compatible Chat Completions",
		modelprovider.QwenTokenPlanIndividual: "Qwen Token Plan Individual OpenAI-compatible Chat Completions",
		modelprovider.XiaomiTokenPlanCN:       "Xiaomi MiMo Token Plan China Chat Completions",
		modelprovider.XiaomiTokenPlanAMS:      "Xiaomi MiMo Token Plan Amsterdam Chat Completions",
		modelprovider.XiaomiTokenPlanSGP:      "Xiaomi MiMo Token Plan Singapore Chat Completions",
		modelprovider.OpenAICompatible:        "custom Chat Completions endpoint",
		modelprovider.Codex:                   "ChatGPT/Codex subscription",
		modelprovider.Copilot:                 "GitHub Copilot subscription",
		modelprovider.KimiCoding:              "Kimi Code subscription or Kimi API key",
		modelprovider.Radius:                  "Radius API key or account OAuth (Gator client)",
		modelprovider.OpenCode:                "OpenCode Zen API gateway",
		modelprovider.OpenCodeGo:              "OpenCode Go API gateway",
	}
	options := make([]dropdownOption, 0, len(descriptions))
	store, storeErr := auth.New(stateDir)
	for _, name := range modelprovider.Names() {
		provider, err := modelprovider.ParseProvider(name)
		if err != nil {
			continue
		}
		if provider == modelprovider.Claude {
			continue
		}
		description := descriptions[provider]
		if modelprovider.RequiresOAuthLogin(provider) {
			description = subscriptionDescription(description, name, store, storeErr)
		}
		options = append(options, dropdownOption{value: name, description: description})
	}
	options = append(options, customProviderOptions(customProviders)...)
	return options
}

func subscriptionDescription(description, provider string, store auth.Store, storeErr error) string {
	if storeErr == nil {
		credential, found, err := store.Read(provider)
		if err == nil && found && credential.IsOAuth() && !credential.Expired(time.Now()) {
			return description + " · signed in"
		}
	}
	if clientIDEnvironment := oauthClientIDEnvironment(provider); clientIDEnvironment != "" && strings.TrimSpace(os.Getenv(clientIDEnvironment)) == "" {
		if command := delegatedConnectCommand(provider); command != "" {
			return description + " · first-party CLI: " + command
		}
		return description + " · requires " + clientIDEnvironment
	}
	return description + " · sign in from /model"
}

func delegatedConnectCommand(provider string) string {
	switch provider {
	case string(modelprovider.Codex):
		return "gator connect codex"
	case string(modelprovider.Copilot):
		return "gator connect copilot"
	case string(modelprovider.KimiCoding):
		return "gator connect kimi"
	case string(modelprovider.XAI):
		return "gator connect xai"
	case string(modelprovider.Claude):
		return "gator connect claude"
	default:
		return ""
	}
}

func oauthClientIDEnvironment(provider string) string {
	switch provider {
	case string(modelprovider.Codex):
		return "GATOR_CODEX_OAUTH_CLIENT_ID"
	case string(modelprovider.Copilot):
		return "GATOR_COPILOT_OAUTH_CLIENT_ID"
	case string(modelprovider.KimiCoding):
		return "GATOR_KIMI_CODE_OAUTH_CLIENT_ID"
	case string(modelprovider.XAI):
		return "GATOR_XAI_OAUTH_CLIENT_ID"
	case string(modelprovider.Radius):
		return "GATOR_RADIUS_OAUTH_CLIENT_ID"
	default:
		return ""
	}
}

// modelDropdownOptions deliberately offers only model IDs Gator can recommend
// without guessing an arbitrary provider catalog. The text field remains
// editable for deployments, aliases, previews, and account-specific models.
func (m Model) modelDropdownOptions(providerName string) []dropdownOption {
	if custom, found := m.customProvider(providerName); found {
		options := make([]dropdownOption, 0, len(custom.Models))
		for _, modelName := range custom.Models {
			description := "configured custom-provider model"
			if modelName == custom.DefaultModel {
				description = "configured default"
			}
			options = append(options, dropdownOption{value: modelName, label: modelName, description: description})
		}
		return options
	}
	provider, err := modelprovider.ParseProvider(providerName)
	if err != nil {
		return nil
	}
	customDescription := "type a model ID supported by this provider"
	if provider == modelprovider.AzureOpenAI || provider == modelprovider.AzureOpenAIResponses {
		customDescription = "type the Azure deployment name"
	}
	options := []dropdownOption{{label: "custom model ID", description: customDescription, custom: true}}
	if provider == modelprovider.Copilot {
		options = append(copilotModelDropdownOptions(m.config.StateDir), options...)
	}
	models := modelprovider.CuratedModels(provider)
	for index := len(models) - 1; index >= 0; index-- {
		modelName := models[index]
		description := "Gator recommended default"
		if modelName != modelprovider.DefaultModel(provider) {
			description = "checked-in provider catalog"
		}
		options = append([]dropdownOption{{value: modelName, label: modelName, description: description}}, options...)
	}
	return options
}

func copilotModelDropdownOptions(stateDir string) []dropdownOption {
	store, err := auth.New(stateDir)
	if err != nil {
		return nil
	}
	credential, found, err := store.Read(string(modelprovider.Copilot))
	if err != nil || !found {
		return nil
	}
	models := make([]string, 0)
	for _, value := range strings.Split(credential.Extra["available_model_ids"], ",") {
		model := strings.TrimSpace(value)
		if model != "" && !strings.ContainsAny(model, "\r\n") {
			models = append(models, model)
		}
	}
	sort.Strings(models)
	options := make([]dropdownOption, 0, len(models))
	for _, model := range models {
		options = append(options, dropdownOption{value: model, label: model, description: "enabled for this Copilot account"})
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
		return matchingDropdownOptions(providerDropdownOptions(m.config.StateDir, m.config.CustomProviders), m.provider.Value())
	case modelField:
		return matchingDropdownOptions(m.modelDropdownOptions(m.provider.Value()), m.model.Value())
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
		m.delegateRuntime = ""
		m.provider.SetValue(selected.value)
		if _, modelName, _, err := m.resolveProviderAndModel(selected.value, ""); err == nil {
			m.model.SetValue(modelName)
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
		m.notice = notice{text: "Model selected: " + selected.value, kind: noticeInfo}
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
		if attachment.IsImage(reference) {
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
	runtime := "native Gator"
	if m.delegateRuntime != "" {
		runtime = delegatedRuntimeLabel(m.delegateRuntime)
	}
	effectiveSteps := m.effort.maxSteps(m.config.MaxSteps)
	if exact, err := parseExactTurnCap(m.runOptions.maxSteps.Value()); err == nil && exact > 0 {
		effectiveSteps = exact
	}
	return "repository: " + m.config.RepositoryPath + "\nmode: " + m.runMode.String() + "\nprovider: " + m.provider.Value() + "\nmodel: " + m.model.Value() + "\nruntime: " + runtime + "\neffort: " + m.effort.label() + "\nmax steps: " + fmt.Sprint(effectiveSteps) + " (base " + fmt.Sprint(m.config.MaxSteps) + ")\n" + m.runOptionsStatus() + "\n" + m.queueSummary() + "\nverification:\n" + verificationText
}

func (m Model) permissionsStatus() string {
	if m.delegateRuntime != "" {
		return "runtime: " + delegatedRuntimeLabel(m.delegateRuntime) + "\nGator: creates an isolated worktree and runs the listed verifier commands\nharness: owns agent tools, sandbox, approvals, and session state\nactive checkout: never edited by a delegated run"
	}
	if m.runMode == gatorrun.PlanMode {
		return "mode: enforced Plan\nwrites: disabled\ncommands: disabled\nreads: repository paths and Git state only\nactive checkout: never edited by a normal run"
	}
	verification, err := parseVerification(m.verification.Value())
	commands := "invalid verifier configuration: " + err.Error()
	if err == nil {
		commands = formatVerification(verification)
	}
	policy := m.config.Execution.Normalize()
	filesystem := "worktree and private scratch only"
	if len(policy.ReadOnlyRoots) > 0 || len(policy.WritableRoots) > 0 {
		filesystem += fmt.Sprintf(" · %d extra read-only and %d extra writable grant(s)", len(policy.ReadOnlyRoots), len(policy.WritableRoots))
	}
	environment := "filtered"
	if len(policy.Environment) > 0 {
		environment += fmt.Sprintf(" · %d explicitly forwarded variable(s)", len(policy.Environment))
	}
	sandboxStatus := string(policy.Mode) + " (OS-enforced Seatbelt on macOS or Bubblewrap on Linux; strict mode fails closed when unavailable)"
	if policy.Mode == "off" {
		sandboxStatus = "off (approved commands run with the Gator user's host authority)"
	}
	prefixes, _ := parseArgvLines(m.runOptions.prefixes.Value())
	allowed, _ := parseArgvLines(m.runOptions.allowed.Value())
	prefixText := "none"
	if len(prefixes) > 0 {
		prefixText = formatVerification(prefixes)
	}
	allowedText := "none"
	if len(allowed) > 0 {
		allowedText = formatVerification(allowed)
	}
	return "writes: isolated run worktree only (apply_patch)\nreads: repository paths only\ncommands: required --verify argv run immediately; literal prefixes and exact allow-command argv also run immediately; any other argv waits for y/enter (once), a (always this exact argv for this thread), or n (deny)\ncwd: isolated worktree\nsandbox: " + sandboxStatus + "\nnetwork: " + string(policy.Network) + "\nfilesystem: " + filesystem + "\nenvironment: " + environment + "\ncommands allowed without prompt:\n" + commands + "\nexact allow-command:\n" + allowedText + "\nliteral command prefixes:\n" + prefixText + "\nactive checkout: never edited by a normal run"
}

func (m Model) agentsStatus() string {
	lines := []string{"Project profiles can only narrow sandbox, network, mode, turn cap, and tool families. Roles are prompt-only and never expand tools."}
	profiles, err := instructions.ListProfiles(m.config.RepositoryPath)
	if err != nil {
		lines = append(lines, "Profiles: "+err.Error())
	} else if len(profiles) == 0 {
		lines = append(lines, "Profiles: none configured")
	} else {
		lines = append(lines, "Profiles:")
		for _, profile := range profiles {
			omit := "none"
			if len(profile.Policy.Omit) > 0 {
				omit = strings.Join(profile.Policy.Omit, ", ")
			}
			lines = append(lines, fmt.Sprintf("  %s  %s  omit=%s", profile.Name, profile.Description, omit))
		}
	}
	roles, err := instructions.LoadRoles(m.config.RepositoryPath)
	if err != nil {
		lines = append(lines, "Roles: "+err.Error())
	} else if len(roles) == 0 {
		lines = append(lines, "Roles: none configured")
	} else {
		lines = append(lines, "Roles:")
		for _, role := range roles {
			lines = append(lines, fmt.Sprintf("  %s  %s  %s", role.Name, role.Kind, role.Description))
		}
	}
	return strings.Join(lines, "\n")
}

func delegatedRuntimeForProvider(provider string) string {
	switch strings.TrimSpace(provider) {
	case string(modelprovider.Codex):
		return "codex"
	case string(modelprovider.Copilot):
		return "copilot"
	case string(modelprovider.KimiCoding):
		return "kimi"
	case string(modelprovider.Claude):
		return "claude"
	default:
		return ""
	}
}

func delegatedRuntimeLabel(runtime string) string {
	switch runtime {
	case "codex":
		return "Codex CLI harness"
	case "copilot":
		return "GitHub Copilot CLI harness"
	case "kimi":
		return "Kimi CLI harness"
	case "claude":
		return "Claude Code harness"
	default:
		return runtime + " CLI harness"
	}
}

func commandHelp() string {
	lines := make([]string, 0, len(slashCommands))
	for _, command := range slashCommands {
		lines = append(lines, command.name+" — "+command.description)
	}
	return strings.Join(lines, "\n")
}
