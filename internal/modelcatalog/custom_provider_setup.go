package modelcatalog

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/localmodel"
	modelprovider "github.com/gongahkia/gator/internal/model"
)

type customProviderField uint8

const (
	customProviderIDField customProviderField = iota
	customProviderURLField
	customProviderEnvField
	customProviderModelsField
	customProviderDefaultField
)

type customProviderSetupForm struct {
	creating     bool
	id           textinput.Model
	baseURL      textinput.Model
	apiKeyEnv    textinput.Model
	models       textarea.Model
	defaultModel textinput.Model
	focus        customProviderField
	saving       bool
}

type customProviderSavedMsg struct {
	providers []config.CustomProvider
	id        string
	err       error
}

type customProviderRemovedMsg struct {
	providers []config.CustomProvider
	id        string
	err       error
}

type customProviderDiscoverMsg struct {
	generation uint64
	preview    CustomProviderDiscovery
	err        error
}

type customProviderAppliedMsg struct {
	providers []config.CustomProvider
	id        string
	err       error
}

type credentialRemovedMsg struct {
	result CredentialRemovalResult
	err    error
}

type credentialStatusesMsg struct {
	statuses []StoredCredentialStatus
	err      error
}

func (m Model) beginCustomProviderCreate() (tea.Model, tea.Cmd) {
	if m.localModels.section != cloudModelSection {
		m.notice = notice{text: "Custom providers are created from the Cloud section.", kind: noticeInfo}
		return m, nil
	}
	if m.config.ModelManagement == nil {
		m.notice = notice{text: "Custom provider management is unavailable in this TUI session.", kind: noticeError}
		return m, nil
	}
	form := newCustomProviderSetupForm(config.CustomProvider{}, true, m.inlineWidth())
	m.localModels.customSetup = &form
	return m, form.focusInput()
}

func (m Model) beginCustomProviderEdit() (tea.Model, tea.Cmd) {
	cloud, found := m.selectedCloudModel()
	if !found {
		m.notice = notice{text: "Select a custom provider before editing it.", kind: noticeError}
		return m, nil
	}
	provider, ok := m.customProvider(cloud.provider)
	if !ok {
		return m.beginCloudModelSetup()
	}
	if provider.ID == localmodel.ProviderID {
		m.notice = notice{text: "Gator-managed local Ollama is configured from the Local section.", kind: noticeInfo}
		return m, nil
	}
	if m.config.ModelManagement == nil {
		m.notice = notice{text: "Custom provider management is unavailable in this TUI session.", kind: noticeError}
		return m, nil
	}
	form := newCustomProviderSetupForm(provider, false, m.inlineWidth())
	m.localModels.customSetup = &form
	return m, form.focusInput()
}

func newCustomProviderSetupForm(provider config.CustomProvider, creating bool, width int) customProviderSetupForm {
	form := customProviderSetupForm{
		creating:     creating,
		id:           cloudSetupInput("lowercase id, for example team-gateway", width),
		baseURL:      cloudSetupInput("https://host/v1/chat/completions", width),
		apiKeyEnv:    cloudSetupInput("optional environment variable name", width),
		defaultModel: cloudSetupInput("must match one listed model ID", width),
		focus:        customProviderURLField,
	}
	form.models = textarea.New()
	form.models.Prompt = ""
	form.models.Placeholder = "one model ID per line"
	form.models.ShowLineNumbers = false
	form.models.CharLimit = 64 * 1024
	form.models.SetHeight(4)
	form.models.SetWidth(max(24, width-2))
	form.models.Blur()
	if creating {
		form.focus = customProviderIDField
	}
	form.id.SetValue(provider.ID)
	form.baseURL.SetValue(provider.BaseURL)
	form.apiKeyEnv.SetValue(provider.APIKeyEnv)
	form.models.SetValue(strings.Join(provider.Models, "\n"))
	form.defaultModel.SetValue(provider.DefaultModel)
	return form
}

func (form *customProviderSetupForm) fields() []customProviderField {
	if form.creating {
		return []customProviderField{customProviderIDField, customProviderURLField, customProviderEnvField, customProviderModelsField, customProviderDefaultField}
	}
	return []customProviderField{customProviderURLField, customProviderEnvField, customProviderModelsField, customProviderDefaultField}
}

func (form *customProviderSetupForm) moveFocus(delta int) tea.Cmd {
	fields := form.fields()
	index := 0
	for candidate, field := range fields {
		if field == form.focus {
			index = candidate
			break
		}
	}
	form.focus = fields[(index+delta+len(fields))%len(fields)]
	return form.focusInput()
}

func (form *customProviderSetupForm) focusInput() tea.Cmd {
	form.id.Blur()
	form.baseURL.Blur()
	form.apiKeyEnv.Blur()
	form.models.Blur()
	form.defaultModel.Blur()
	switch form.focus {
	case customProviderIDField:
		return form.id.Focus()
	case customProviderURLField:
		return form.baseURL.Focus()
	case customProviderEnvField:
		return form.apiKeyEnv.Focus()
	case customProviderModelsField:
		return form.models.Focus()
	default:
		return form.defaultModel.Focus()
	}
}

func (form *customProviderSetupForm) isLastField() bool {
	fields := form.fields()
	return len(fields) > 0 && form.focus == fields[len(fields)-1]
}

func (form *customProviderSetupForm) setup() (CustomProviderSetup, error) {
	models := splitCustomProviderModels(form.models.Value())
	setup := CustomProviderSetup{
		ID:           strings.TrimSpace(form.id.Value()),
		BaseURL:      strings.TrimSpace(form.baseURL.Value()),
		APIKeyEnv:    strings.TrimSpace(form.apiKeyEnv.Value()),
		Models:       models,
		DefaultModel: strings.TrimSpace(form.defaultModel.Value()),
	}
	if setup.DefaultModel == "" && len(models) > 0 {
		setup.DefaultModel = models[0]
	}
	if err := validateCustomProviderSetup(setup, form.creating); err != nil {
		return CustomProviderSetup{}, err
	}
	return setup, nil
}

func splitCustomProviderModels(value string) []string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	models := make([]string, 0, len(lines))
	for _, line := range lines {
		if model := strings.TrimSpace(line); model != "" {
			models = append(models, model)
		}
	}
	return models
}

func validateCustomProviderSetup(setup CustomProviderSetup, creating bool) error {
	if creating {
		if _, err := modelprovider.ParseProvider(setup.ID); err == nil {
			return fmt.Errorf("custom provider ID %q conflicts with a built-in Gator provider", setup.ID)
		}
		if setup.ID == localmodel.ProviderID {
			return fmt.Errorf("custom provider ID %q is reserved for the Local section", setup.ID)
		}
	}
	endpoint, err := url.Parse(setup.BaseURL)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil {
		return fmt.Errorf("base URL must be an absolute http(s) URL without credentials")
	}
	return nil
}

func customProviderWarnings(setup CustomProviderSetup) []string {
	warnings := make([]string, 0, 2)
	endpoint, err := url.Parse(setup.BaseURL)
	if err == nil && endpoint.Scheme == "http" && !isLoopbackHost(endpoint.Host) {
		warnings = append(warnings, "Cleartext HTTP to a non-loopback host. Prefer https unless this endpoint is on a trusted local network.")
	}
	if setup.APIKeyEnv == "" {
		warnings = append(warnings, "Keyless: requests send no Authorization header. Use this only for a local or otherwise unauthenticated endpoint.")
	} else {
		warnings = append(warnings, "API key is read from "+setup.APIKeyEnv+" at run time. Gator stores the variable name, never the key.")
	}
	return warnings
}

func isLoopbackHost(host string) bool {
	hostname := host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		hostname = parsedHost
	}
	if ip := net.ParseIP(hostname); ip != nil {
		return ip.IsLoopback()
	}
	return strings.EqualFold(hostname, "localhost")
}

func (m Model) updateCustomProviderSetup(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	form := m.localModels.customSetup
	if form == nil || form.saving {
		return m, nil
	}
	switch message.String() {
	case "esc", "ctrl+c":
		m.localModels.customSetup = nil
		m.notice = notice{text: "Custom provider edit cancelled. Configuration was not saved.", kind: noticeInfo}
		return m, nil
	case "tab", "down":
		if form.focus == customProviderModelsField && message.String() == "down" {
			break
		}
		return m, form.moveFocus(1)
	case "shift+tab", "up":
		if form.focus == customProviderModelsField && message.String() == "up" {
			break
		}
		return m, form.moveFocus(-1)
	case "enter":
		if form.focus == customProviderModelsField {
			break
		}
		if !form.isLastField() {
			return m, form.moveFocus(1)
		}
		return m.reviewCustomProviderSetup()
	}
	if form.focus == customProviderModelsField {
		var command tea.Cmd
		form.models, command = form.models.Update(message)
		return m, command
	}
	var command tea.Cmd
	switch form.focus {
	case customProviderIDField:
		form.id, command = form.id.Update(message)
	case customProviderURLField:
		form.baseURL, command = form.baseURL.Update(message)
	case customProviderEnvField:
		form.apiKeyEnv, command = form.apiKeyEnv.Update(message)
	default:
		form.defaultModel, command = form.defaultModel.Update(message)
	}
	return m, command
}

func (m Model) reviewCustomProviderSetup() (tea.Model, tea.Cmd) {
	form := m.localModels.customSetup
	if form == nil {
		return m, nil
	}
	setup, err := form.setup()
	if err != nil {
		m.notice = notice{text: err.Error(), kind: noticeError}
		return m, nil
	}
	if form.creating {
		if _, exists := m.customProvider(setup.ID); exists {
			m.notice = notice{text: "Custom provider " + setup.ID + " already exists. Edit it instead of creating a duplicate.", kind: noticeError}
			return m, nil
		}
	}
	m.localModels.pendingCustom = &setup
	m.localModels.confirmation = localModelConfirmSaveCustom
	m.notice = notice{text: "Review the custom provider destination, then confirm to save metadata only.", kind: noticeInfo}
	return m, nil
}

func (m Model) saveReviewedCustomProvider() (tea.Model, tea.Cmd) {
	if m.localModels.pendingCustom == nil || m.config.ModelManagement == nil {
		m.notice = notice{text: "Custom provider management is unavailable in this TUI session.", kind: noticeError}
		return m, nil
	}
	setup := *m.localModels.pendingCustom
	if m.localModels.customSetup != nil {
		m.localModels.customSetup.saving = true
	}
	return m, func() tea.Msg {
		providers, err := m.config.ModelManagement.SaveCustomProvider(setup)
		return customProviderSavedMsg{providers: providers, id: setup.ID, err: err}
	}
}

func (form *customProviderSetupForm) fieldPresentation(field customProviderField) (string, string, string) {
	switch field {
	case customProviderIDField:
		return "Provider ID", "Lowercase letters, digits, and hyphens. Cannot match a built-in provider or gator-local.", form.id.View()
	case customProviderURLField:
		return "Chat Completions URL", "Absolute http(s) URL. Discovery requires a path ending in /chat/completions.", form.baseURL.View()
	case customProviderEnvField:
		return "API key environment variable", "Optional. Leave empty for a keyless local endpoint. Do not paste a key.", form.apiKeyEnv.View()
	case customProviderModelsField:
		return "Model IDs", "One provider model ID per line. These are sent unchanged to the endpoint.", form.models.View()
	default:
		return "Default model", "Must be one of the listed model IDs.", form.defaultModel.View()
	}
}

func customProviderReviewText(setup CustomProviderSetup) string {
	host := setup.BaseURL
	if endpoint, err := url.Parse(setup.BaseURL); err == nil {
		host = endpoint.Scheme + "://" + endpoint.Host + endpoint.Path
	}
	lines := []string{
		"ID: " + setup.ID,
		"Endpoint: " + host,
		fmt.Sprintf("Models: %d · default %s", len(setup.Models), setup.DefaultModel),
	}
	lines = append(lines, customProviderWarnings(setup)...)
	return strings.Join(lines, "\n")
}
