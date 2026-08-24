package tui

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	modelprovider "github.com/gongahkia/gator/internal/model"
)

type cloudSetupField uint8

const (
	cloudSetupModelField cloudSetupField = iota
	cloudSetupCredentialField
	cloudSetupEndpointField
	cloudSetupAPIVersionField
)

const (
	cloudCredentialAPIKey = "api_key"
	cloudCredentialBearer = "bearer_token"
)

type cloudModelSetupForm struct {
	provider       string
	model          textinput.Model
	credential     textinput.Model
	endpoint       textinput.Model
	apiVersion     textinput.Model
	credentialType string
	focus          cloudSetupField
	saving         bool
}

type cloudModelSetupSavedMsg struct {
	provider string
	model    string
	baseURL  string
	err      error
}

func (m Model) beginCloudModelSetup() (tea.Model, tea.Cmd) {
	if m.localModels.section != cloudModelSection {
		m.notice = notice{text: "Cloud configuration is available from the Cloud section.", kind: noticeInfo}
		return m, nil
	}
	cloud, found := m.selectedCloudModel()
	if !found {
		m.notice = notice{text: "Select a cloud provider before configuring it.", kind: noticeError}
		return m, nil
	}
	provider, err := modelprovider.ParseProvider(cloud.provider)
	if err != nil || !modelprovider.SupportsDirect(provider) {
		m.notice = notice{text: "This entry is a provider-owned harness or custom endpoint. Its setup is not configured from this form yet.", kind: noticeInfo}
		return m, nil
	}
	if !modelprovider.SupportsAPIKeyLogin(provider) {
		m.notice = notice{text: "This provider does not accept a Gator-managed API key. Use its sign-in action or externally managed credential flow.", kind: noticeInfo}
		return m, nil
	}
	form := newCloudModelSetupForm(provider, cloud.model, m.providerEndpoint(cloud.provider), m.inlineWidth())
	m.localModels.cloudSetup = &form
	return m, m.localModels.cloudSetup.focusInput()
}

func newCloudModelSetupForm(provider modelprovider.Provider, selectedModel, endpoint string, width int) cloudModelSetupForm {
	modelName := strings.TrimSpace(selectedModel)
	if modelName == "" {
		modelName = modelprovider.DefaultModel(provider)
	}
	model := cloudSetupInput("deployment or model ID", width)
	model.SetValue(modelName)
	credential := cloudSetupInput("leave empty to keep the current credential", width)
	credential.EchoMode = textinput.EchoPassword
	credential.EchoCharacter = '•'
	endpointInput := cloudSetupInput("provider default endpoint", width)
	endpointInput.SetValue(endpoint)
	version := cloudSetupInput("v1", width)
	if parsed, err := url.Parse(endpoint); err == nil {
		version.SetValue(parsed.Query().Get("api-version"))
	}
	if strings.TrimSpace(version.Value()) == "" {
		version.SetValue("v1")
	}
	return cloudModelSetupForm{
		provider:       string(provider),
		model:          model,
		credential:     credential,
		endpoint:       endpointInput,
		apiVersion:     version,
		credentialType: cloudCredentialAPIKey,
	}
}

func cloudSetupInput(placeholder string, width int) textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = placeholder
	input.CharLimit = 2 * 1024
	input.Width = max(24, width-2)
	input.Blur()
	return input
}

func (form *cloudModelSetupForm) usesAzureFields() bool {
	return form.provider == string(modelprovider.AzureOpenAI) || form.provider == string(modelprovider.AzureOpenAIResponses)
}

func (form *cloudModelSetupForm) canUseBearerToken() bool {
	return form.provider == string(modelprovider.AzureOpenAIResponses)
}

func (form *cloudModelSetupForm) fieldCount() int {
	if form.usesAzureFields() {
		return 4
	}
	return 3
}

func (form *cloudModelSetupForm) focusInput() tea.Cmd {
	form.model.Blur()
	form.credential.Blur()
	form.endpoint.Blur()
	form.apiVersion.Blur()
	switch form.focus {
	case cloudSetupCredentialField:
		return form.credential.Focus()
	case cloudSetupEndpointField:
		return form.endpoint.Focus()
	case cloudSetupAPIVersionField:
		return form.apiVersion.Focus()
	default:
		return form.model.Focus()
	}
}

func (form *cloudModelSetupForm) moveFocus(delta int) tea.Cmd {
	count := form.fieldCount()
	form.focus = cloudSetupField((int(form.focus) + delta + count) % count)
	return form.focusInput()
}

func (form *cloudModelSetupForm) updateFocusedInput(message tea.Msg) tea.Cmd {
	var command tea.Cmd
	switch form.focus {
	case cloudSetupCredentialField:
		form.credential, command = form.credential.Update(message)
	case cloudSetupEndpointField:
		form.endpoint, command = form.endpoint.Update(message)
	case cloudSetupAPIVersionField:
		form.apiVersion, command = form.apiVersion.Update(message)
	default:
		form.model, command = form.model.Update(message)
	}
	return command
}

func (m Model) updateCloudModelSetup(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	form := m.localModels.cloudSetup
	if form == nil || form.saving {
		return m, nil
	}
	switch message.String() {
	case "esc", "ctrl+c":
		m.localModels.cloudSetup = nil
		m.notice = notice{text: "Cloud model configuration cancelled. No credential was saved.", kind: noticeInfo}
		return m, nil
	case "tab", "down":
		return m, form.moveFocus(1)
	case "shift+tab", "up":
		return m, form.moveFocus(-1)
	case "a":
		if form.canUseBearerToken() {
			if form.credentialType == cloudCredentialAPIKey {
				form.credentialType = cloudCredentialBearer
			} else {
				form.credentialType = cloudCredentialAPIKey
			}
			form.credential.Reset()
			m.notice = notice{text: "Azure Responses credential type changed. Enter a replacement only when you want to overwrite the stored credential.", kind: noticeInfo}
			return m, form.focusInput()
		}
	case "enter":
		if form.focus != cloudSetupField(form.fieldCount()-1) {
			return m, form.moveFocus(1)
		}
		return m.saveCloudModelSetup()
	}
	return m, form.updateFocusedInput(message)
}

func (m Model) saveCloudModelSetup() (tea.Model, tea.Cmd) {
	form := m.localModels.cloudSetup
	if form == nil || m.config.SaveCloudModel == nil {
		m.notice = notice{text: "Cloud model configuration is unavailable in this TUI session.", kind: noticeError}
		return m, nil
	}
	setup, err := form.configuration()
	if err != nil {
		m.notice = notice{text: err.Error(), kind: noticeError}
		return m, nil
	}
	form.saving = true
	form.credential.Reset()
	return m, func() tea.Msg {
		err := m.config.SaveCloudModel(setup)
		return cloudModelSetupSavedMsg{provider: setup.Provider, model: setup.Model, baseURL: setup.BaseURL, err: err}
	}
}

func (form *cloudModelSetupForm) configuration() (CloudModelSetup, error) {
	provider, err := modelprovider.ParseProvider(form.provider)
	if err != nil {
		return CloudModelSetup{}, err
	}
	modelName := strings.TrimSpace(form.model.Value())
	if modelName == "" {
		modelName = modelprovider.DefaultModel(provider)
	}
	if modelName == "" {
		return CloudModelSetup{}, fmt.Errorf("a deployment or model ID is required for %s", provider)
	}
	baseURL := strings.TrimSpace(form.endpoint.Value())
	if form.usesAzureFields() {
		baseURL, err = azureCloudEndpoint(provider, baseURL, modelName, form.apiVersion.Value())
		if err != nil {
			return CloudModelSetup{}, err
		}
	} else if baseURL != "" {
		endpoint, parseErr := url.Parse(baseURL)
		if parseErr != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil {
			return CloudModelSetup{}, fmt.Errorf("endpoint must be an absolute http(s) URL")
		}
	}
	credentialType := form.credentialType
	if credentialType == "" {
		credentialType = cloudCredentialAPIKey
	}
	return CloudModelSetup{
		Provider:       string(provider),
		Model:          modelName,
		BaseURL:        baseURL,
		APIKey:         form.credential.Value(),
		CredentialType: credentialType,
	}, nil
}

func azureCloudEndpoint(provider modelprovider.Provider, configured, deployment, version string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return "", fmt.Errorf("an Azure resource URL or full endpoint is required")
	}
	if !strings.Contains(configured, "://") {
		configured = "https://" + configured
	}
	endpoint, err := url.Parse(configured)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil {
		return "", fmt.Errorf("Azure endpoint must be an absolute http(s) URL")
	}
	path := strings.TrimRight(endpoint.Path, "/")
	if provider == modelprovider.AzureOpenAIResponses {
		switch {
		case strings.HasSuffix(path, "/responses"):
		case strings.HasSuffix(path, "/openai/v1"):
			path += "/responses"
		default:
			path += "/openai/v1/responses"
		}
	} else if !strings.HasSuffix(path, "/chat/completions") {
		path += "/openai/deployments/" + deployment + "/chat/completions"
	}
	endpoint.Path = path
	query := endpoint.Query()
	if query.Get("api-version") == "" {
		version = strings.TrimSpace(version)
		if version == "" {
			version = "v1"
		}
		query.Set("api-version", version)
	}
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func (m Model) providerEndpoint(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == strings.ToLower(strings.TrimSpace(m.provider.Value())) && strings.TrimSpace(m.config.BaseURL) != "" {
		return m.config.BaseURL
	}
	return strings.TrimSpace(m.config.ProviderEndpoints[provider])
}

func (m Model) cloudModelSetupView() string {
	form := m.localModels.cloudSetup
	if form == nil {
		return ""
	}
	providerLabel := form.provider
	if form.usesAzureFields() {
		providerLabel += " · Azure endpoint setup"
	}
	sections := []string{
		m.fieldView("Configure cloud model", "Credentials are masked and saved only in Gator's private auth file. Leaving the credential field empty preserves any existing stored credential.", providerLabel),
		m.fieldView("Deployment or model", "Use the Azure deployment name for Azure; use the provider model ID elsewhere.", form.model.View()),
	}
	credentialLabel := "API key"
	credentialDescription := "Masked. Leave empty to retain the stored credential."
	if form.credentialType == cloudCredentialBearer {
		credentialLabel = "Azure bearer token"
		credentialDescription = "Masked Microsoft Entra token. Gator does not refresh user-supplied tokens."
	}
	sections = append(sections, m.fieldView(credentialLabel, credentialDescription, form.credential.View()))
	endpointLabel := "Endpoint override"
	endpointDescription := "Optional. Leave empty to use the provider default endpoint."
	if form.usesAzureFields() {
		endpointLabel = "Azure resource URL or endpoint"
		endpointDescription = "A resource root is expanded safely; a full Azure endpoint is also accepted."
	}
	sections = append(sections, m.fieldView(endpointLabel, endpointDescription, form.endpoint.View()))
	if form.usesAzureFields() {
		sections = append(sections, m.fieldView("Azure API version", "Saved into the endpoint URL. Use the version enabled for this Azure resource.", form.apiVersion.View()))
	}
	if form.canUseBearerToken() {
		mode := "API key"
		if form.credentialType == cloudCredentialBearer {
			mode = "Bearer token"
		}
		sections = append(sections, m.inline(dimStyle.Render("a toggles Azure Responses credential type · current: "+mode)))
	}
	if form.saving {
		sections = append(sections, m.inline(dimStyle.Render("Saving cloud configuration…")))
	}
	return strings.Join(sections, "\n")
}
