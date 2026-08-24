package tui

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
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
	cloudSetupProjectField
	cloudSetupLocationField
	cloudSetupAccountIDField
	cloudSetupGatewayIDField
	cloudSetupGatewayProtocolField
	cloudSetupRegionField
	cloudSetupAWSProfileField
	cloudSetupCredentialsPathField
	cloudSetupGatewayField
)

const (
	cloudCredentialAPIKey = "api_key"
	cloudCredentialBearer = "bearer_token"
)

var cloudIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,256}$`)
var cloudRegionPattern = regexp.MustCompile(`^[a-z]{2}(?:-gov)?-[a-z]+-\d+$`)

type cloudModelSetupForm struct {
	provider        string
	model           textinput.Model
	credential      textinput.Model
	endpoint        textinput.Model
	apiVersion      textinput.Model
	project         textinput.Model
	location        textinput.Model
	accountID       textinput.Model
	gatewayID       textinput.Model
	gatewayProtocol textinput.Model
	region          textinput.Model
	awsProfile      textinput.Model
	credentialsPath textinput.Model
	gateway         textinput.Model
	credentialType  string
	focus           cloudSetupField
	saving          bool
}

type cloudModelSetupSavedMsg struct {
	provider        string
	model           string
	baseURL         string
	options         map[string]string
	delegateRuntime string
	err             error
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
		m.notice = notice{text: "This entry is a custom endpoint rather than a built-in Gator cloud provider. Configure its endpoint from the provider manager.", kind: noticeInfo}
		return m, nil
	}
	form := newCloudModelSetupForm(provider, cloud.model, m.providerEndpoint(cloud.provider), m.providerOptions(cloud.provider), m.inlineWidth())
	m.localModels.cloudSetup = &form
	return m, m.localModels.cloudSetup.focusInput()
}

func newCloudModelSetupForm(provider modelprovider.Provider, selectedModel, endpoint string, options map[string]string, width int) cloudModelSetupForm {
	modelName := strings.TrimSpace(selectedModel)
	if modelName == "" {
		modelName = modelprovider.DefaultModel(provider)
	}
	form := cloudModelSetupForm{
		provider:        string(provider),
		model:           cloudSetupInput("deployment or model ID", width),
		credential:      cloudSetupInput("leave empty to keep the current credential", width),
		endpoint:        cloudSetupInput("provider default endpoint", width),
		apiVersion:      cloudSetupInput("v1", width),
		project:         cloudSetupInput("project ID (or leave empty to use the environment)", width),
		location:        cloudSetupInput("location, for example us-central1", width),
		accountID:       cloudSetupInput("account ID (or leave empty to use the environment)", width),
		gatewayID:       cloudSetupInput("AI Gateway ID (or leave empty to use the environment)", width),
		gatewayProtocol: cloudSetupInput("openai-responses, anthropic-messages, or workers-ai-chat-completions", width),
		region:          cloudSetupInput("AWS region, for example us-east-1", width),
		awsProfile:      cloudSetupInput("optional AWS shared profile", width),
		credentialsPath: cloudSetupInput("optional absolute ADC credentials path", width),
		gateway:         cloudSetupInput("optional Radius gateway URL", width),
		credentialType:  cloudCredentialAPIKey,
	}
	form.model.SetValue(modelName)
	form.endpoint.SetValue(strings.TrimSpace(endpoint))
	form.credential.EchoMode = textinput.EchoPassword
	form.credential.EchoCharacter = '•'
	form.project.SetValue(strings.TrimSpace(options["project"]))
	form.location.SetValue(strings.TrimSpace(options["location"]))
	form.accountID.SetValue(strings.TrimSpace(options["account_id"]))
	form.gatewayID.SetValue(strings.TrimSpace(options["gateway_id"]))
	form.gatewayProtocol.SetValue(strings.TrimSpace(options["gateway_protocol"]))
	form.region.SetValue(strings.TrimSpace(options["region"]))
	form.awsProfile.SetValue(strings.TrimSpace(options["profile"]))
	form.credentialsPath.SetValue(strings.TrimSpace(options["credentials_path"]))
	form.gateway.SetValue(strings.TrimSpace(options["gateway"]))
	if parsed, err := url.Parse(endpoint); err == nil {
		form.apiVersion.SetValue(parsed.Query().Get("api-version"))
	}
	if strings.TrimSpace(form.apiVersion.Value()) == "" {
		form.apiVersion.SetValue("v1")
	}
	if provider == modelprovider.AmazonBedrock || provider == modelprovider.GoogleVertex {
		form.credentialType = cloudCredentialBearer
	}
	return form
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

func (form *cloudModelSetupForm) providerKind() modelprovider.Provider {
	provider, _ := modelprovider.ParseProvider(form.provider)
	return provider
}

func (form *cloudModelSetupForm) usesAzureFields() bool {
	provider := form.providerKind()
	return provider == modelprovider.AzureOpenAI || provider == modelprovider.AzureOpenAIResponses
}

func (form *cloudModelSetupForm) canUseBearerToken() bool {
	return form.providerKind() == modelprovider.AzureOpenAIResponses
}

func (form *cloudModelSetupForm) needsCredentialInput() bool {
	switch form.providerKind() {
	case modelprovider.Codex, modelprovider.Copilot:
		return false
	default:
		return true
	}
}

func (form *cloudModelSetupForm) fields() []cloudSetupField {
	provider := form.providerKind()
	fields := []cloudSetupField{cloudSetupModelField}
	if form.needsCredentialInput() {
		fields = append(fields, cloudSetupCredentialField)
	}
	switch provider {
	case modelprovider.Claude:
		return fields
	case modelprovider.AzureOpenAI, modelprovider.AzureOpenAIResponses:
		return append(fields, cloudSetupEndpointField, cloudSetupAPIVersionField)
	case modelprovider.AmazonBedrock:
		return append(fields, cloudSetupEndpointField, cloudSetupRegionField, cloudSetupAWSProfileField)
	case modelprovider.GoogleVertex:
		return append(fields, cloudSetupEndpointField, cloudSetupProjectField, cloudSetupLocationField, cloudSetupCredentialsPathField)
	case modelprovider.CloudflareWorkers:
		return append(fields, cloudSetupEndpointField, cloudSetupAccountIDField)
	case modelprovider.CloudflareGateway:
		return append(fields, cloudSetupEndpointField, cloudSetupAccountIDField, cloudSetupGatewayIDField, cloudSetupGatewayProtocolField)
	case modelprovider.Radius:
		return append(fields, cloudSetupEndpointField, cloudSetupGatewayField)
	default:
		return append(fields, cloudSetupEndpointField)
	}
}

func (form *cloudModelSetupForm) focusedInput() *textinput.Model {
	switch form.focus {
	case cloudSetupCredentialField:
		return &form.credential
	case cloudSetupEndpointField:
		return &form.endpoint
	case cloudSetupAPIVersionField:
		return &form.apiVersion
	case cloudSetupProjectField:
		return &form.project
	case cloudSetupLocationField:
		return &form.location
	case cloudSetupAccountIDField:
		return &form.accountID
	case cloudSetupGatewayIDField:
		return &form.gatewayID
	case cloudSetupGatewayProtocolField:
		return &form.gatewayProtocol
	case cloudSetupRegionField:
		return &form.region
	case cloudSetupAWSProfileField:
		return &form.awsProfile
	case cloudSetupCredentialsPathField:
		return &form.credentialsPath
	case cloudSetupGatewayField:
		return &form.gateway
	default:
		return &form.model
	}
}

func (form *cloudModelSetupForm) focusInput() tea.Cmd {
	for _, field := range form.fields() {
		form.focusedInputFor(field).Blur()
	}
	return form.focusedInput().Focus()
}

func (form *cloudModelSetupForm) focusedInputFor(field cloudSetupField) *textinput.Model {
	previous := form.focus
	form.focus = field
	input := form.focusedInput()
	form.focus = previous
	return input
}

func (form *cloudModelSetupForm) moveFocus(delta int) tea.Cmd {
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

func (form *cloudModelSetupForm) updateFocusedInput(message tea.Msg) tea.Cmd {
	input := form.focusedInput()
	updated, command := input.Update(message)
	*input = updated
	return command
}

func (form *cloudModelSetupForm) isLastField() bool {
	fields := form.fields()
	return len(fields) > 0 && form.focus == fields[len(fields)-1]
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
		if !form.isLastField() {
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
		return cloudModelSetupSavedMsg{
			provider:        setup.Provider,
			model:           setup.Model,
			baseURL:         setup.BaseURL,
			options:         cloneProviderOptions(setup.Options),
			delegateRuntime: setup.DelegateRuntime,
			err:             err,
		}
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
	if (provider == modelprovider.AzureOpenAI || provider == modelprovider.AzureOpenAIResponses) && !cloudIdentifierPattern.MatchString(modelName) {
		return CloudModelSetup{}, fmt.Errorf("Azure deployment name contains unsupported characters")
	}
	baseURL := strings.TrimSpace(form.endpoint.Value())
	if form.usesAzureFields() {
		baseURL, err = azureCloudEndpoint(provider, baseURL, modelName, form.apiVersion.Value())
		if err != nil {
			return CloudModelSetup{}, err
		}
	} else if baseURL != "" {
		if err := validateCloudURL(baseURL, "endpoint"); err != nil {
			return CloudModelSetup{}, err
		}
	}
	options := cloudProviderOptions(form)
	if err := validateCloudProviderOptions(provider, options); err != nil {
		return CloudModelSetup{}, err
	}
	setup := CloudModelSetup{
		Provider:       string(provider),
		Model:          modelName,
		BaseURL:        baseURL,
		Options:        options,
		APIKey:         form.credential.Value(),
		CredentialType: form.credentialType,
	}
	if provider == modelprovider.Claude {
		setup.DelegateRuntime = "claude"
	}
	if !form.needsCredentialInput() {
		setup.APIKey = ""
		setup.CredentialType = ""
	}
	return setup, nil
}

func cloudProviderOptions(form *cloudModelSetupForm) map[string]string {
	values := map[string]string{
		"project":          form.project.Value(),
		"location":         form.location.Value(),
		"account_id":       form.accountID.Value(),
		"gateway_id":       form.gatewayID.Value(),
		"gateway_protocol": form.gatewayProtocol.Value(),
		"region":           form.region.Value(),
		"profile":          form.awsProfile.Value(),
		"credentials_path": form.credentialsPath.Value(),
		"gateway":          form.gateway.Value(),
	}
	options := make(map[string]string, len(values))
	for name, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			options[name] = value
		}
	}
	return options
}

func validateCloudProviderOptions(provider modelprovider.Provider, options map[string]string) error {
	for name, value := range options {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%s cannot contain a newline", strings.ReplaceAll(name, "_", " "))
		}
	}
	for _, name := range []string{"project", "location", "account_id", "gateway_id", "profile"} {
		if value := options[name]; value != "" && !cloudIdentifierPattern.MatchString(value) {
			return fmt.Errorf("%s contains unsupported characters", strings.ReplaceAll(name, "_", " "))
		}
	}
	if region := options["region"]; region != "" && !cloudRegionPattern.MatchString(region) {
		return fmt.Errorf("AWS region must look like us-east-1")
	}
	if path := options["credentials_path"]; path != "" && !filepath.IsAbs(path) {
		return fmt.Errorf("ADC credentials path must be absolute")
	}
	if gateway := options["gateway"]; gateway != "" {
		if err := validateCloudURL(gateway, "Radius gateway"); err != nil {
			return err
		}
	}
	if provider == modelprovider.CloudflareGateway {
		protocol := options["gateway_protocol"]
		if protocol != "" && protocol != "openai-responses" && protocol != "anthropic-messages" && protocol != "workers-ai-chat-completions" {
			return fmt.Errorf("Cloudflare AI Gateway protocol must be openai-responses, anthropic-messages, or workers-ai-chat-completions")
		}
	}
	return nil
}

func validateCloudURL(value, label string) error {
	endpoint, err := url.Parse(value)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil {
		return fmt.Errorf("%s must be an absolute http(s) URL", label)
	}
	return nil
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

func (m Model) providerOptions(provider string) map[string]string {
	configured := m.config.ProviderOptions[strings.ToLower(strings.TrimSpace(provider))]
	return cloneProviderOptions(configured)
}

func cloneProviderOptions(options map[string]string) map[string]string {
	if len(options) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(options))
	for name, value := range options {
		cloned[name] = strings.TrimSpace(value)
	}
	return cloned
}

func (m Model) cloudModelSetupView() string {
	form := m.localModels.cloudSetup
	if form == nil {
		return ""
	}
	provider := form.providerKind()
	providerLabel := form.provider
	if provider == modelprovider.Claude {
		providerLabel += " · Claude Code harness"
	} else if form.usesAzureFields() {
		providerLabel += " · Azure endpoint setup"
	}
	sections := []string{m.fieldView("Configure cloud model", form.configurationHint(), providerLabel)}
	for _, field := range form.fields() {
		label, hint, input := form.fieldPresentation(field)
		sections = append(sections, m.fieldView(label, hint, input.View()))
	}
	if form.canUseBearerToken() {
		mode := "API key"
		if form.credentialType == cloudCredentialBearer {
			mode = "Bearer token"
		}
		sections = append(sections, m.inline(dimStyle.Render("a toggles Azure Responses credential type · current: "+mode)))
	}
	if provider == modelprovider.Codex || provider == modelprovider.Copilot {
		sections = append(sections, m.inline(dimStyle.Render("Authentication is account OAuth. Press l in the model catalog to sign in; this form only saves model and endpoint selection.")))
	}
	if form.saving {
		sections = append(sections, m.inline(dimStyle.Render("Saving cloud configuration…")))
	}
	return strings.Join(sections, "\n")
}

func (form *cloudModelSetupForm) configurationHint() string {
	switch form.providerKind() {
	case modelprovider.AmazonBedrock:
		return "Credentials are masked and stored only in Gator's private auth file. Leave the bearer token empty to keep one already stored or use the standard AWS credential chain."
	case modelprovider.GoogleVertex:
		return "Credentials are masked and stored only in Gator's private auth file. Leave the access token empty to keep one already stored or use Application Default Credentials."
	case modelprovider.Claude:
		return "Enter an Anthropic API key for Claude Code. It is masked and stored only in Gator's private auth file; Claude.ai subscription credentials are never imported."
	case modelprovider.Codex, modelprovider.Copilot:
		return "No credential is entered here. Use the provider-owned OAuth sign-in action from the Cloud catalog."
	default:
		return "Credentials are masked and saved only in Gator's private auth file. Leaving the credential field empty preserves any existing stored credential."
	}
}

func (form *cloudModelSetupForm) fieldPresentation(field cloudSetupField) (string, string, *textinput.Model) {
	switch field {
	case cloudSetupCredentialField:
		label := "API key"
		hint := "Masked. Leave empty to retain the stored credential."
		switch form.providerKind() {
		case modelprovider.AzureOpenAIResponses:
			if form.credentialType == cloudCredentialBearer {
				label, hint = "Azure bearer token", "Masked Microsoft Entra token. Gator does not refresh user-supplied tokens."
			}
		case modelprovider.AmazonBedrock:
			label, hint = "AWS bearer token", "Optional and masked. Leave empty to use or retain standard AWS credentials."
		case modelprovider.GoogleVertex:
			label, hint = "Google Cloud access token", "Optional and masked. Leave empty to use or retain Application Default Credentials."
		case modelprovider.Claude:
			label, hint = "Anthropic API key", "Masked. It is passed only to the Claude Code child process for a delegated run."
		}
		return label, hint, &form.credential
	case cloudSetupEndpointField:
		if form.usesAzureFields() {
			return "Azure resource URL or endpoint", "A resource root is expanded safely; a full Azure endpoint is also accepted.", &form.endpoint
		}
		return "Endpoint override", "Optional. Leave empty to use the provider default endpoint.", &form.endpoint
	case cloudSetupAPIVersionField:
		return "Azure API version", "Saved into the endpoint URL. Use the version enabled for this Azure resource.", &form.apiVersion
	case cloudSetupProjectField:
		return "Google Cloud project", "Optional only when GOOGLE_CLOUD_PROJECT or GCLOUD_PROJECT is already configured.", &form.project
	case cloudSetupLocationField:
		return "Google Cloud location", "Optional only when GOOGLE_CLOUD_LOCATION is already configured.", &form.location
	case cloudSetupAccountIDField:
		return "Cloudflare account ID", "Optional only when CLOUDFLARE_ACCOUNT_ID is already configured or the endpoint is fully explicit.", &form.accountID
	case cloudSetupGatewayIDField:
		return "Cloudflare AI Gateway ID", "Optional only when CLOUDFLARE_AI_GATEWAY_ID is already configured.", &form.gatewayID
	case cloudSetupGatewayProtocolField:
		return "Cloudflare AI Gateway protocol", "Choose the request protocol compatible with the selected model family.", &form.gatewayProtocol
	case cloudSetupRegionField:
		return "AWS region", "Optional. Overrides AWS SDK region resolution for this provider only.", &form.region
	case cloudSetupAWSProfileField:
		return "AWS shared profile", "Optional. Selects an existing AWS shared profile without copying its credentials.", &form.awsProfile
	case cloudSetupCredentialsPathField:
		return "ADC credentials path", "Optional absolute Google Application Default Credentials file path. Gator reads it only when it needs a token.", &form.credentialsPath
	case cloudSetupGatewayField:
		return "Radius gateway URL", "Optional Gator Radius gateway override.", &form.gateway
	default:
		return "Deployment or model", "Use the Azure deployment name for Azure; use the provider model ID elsewhere.", &form.model
	}
}
