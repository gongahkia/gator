package modelcatalog

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/config"
	modelprovider "github.com/gongahkia/gator/internal/model"
)

func TestAzureCloudEndpointExpandsResourceURLs(t *testing.T) {
	tests := []struct {
		provider modelprovider.Provider
		endpoint string
		version  string
		want     string
	}{
		{modelprovider.AzureOpenAI, "example-resource.openai.azure.com", "2025-04-01-preview", "https://example-resource.openai.azure.com/openai/deployments/review/chat/completions?api-version=2025-04-01-preview"},
		{modelprovider.AzureOpenAIResponses, "https://example-resource.openai.azure.com", "", "https://example-resource.openai.azure.com/openai/v1/responses?api-version=v1"},
	}
	for _, test := range tests {
		got, err := azureCloudEndpoint(test.provider, test.endpoint, "review", test.version)
		if err != nil {
			t.Fatalf("expand %s endpoint: %v", test.provider, err)
		}
		if got != test.want {
			t.Fatalf("%s endpoint = %q, want %q", test.provider, got, test.want)
		}
	}
}

func TestCloudModelSetupClearsCredentialAndUpdatesSelection(t *testing.T) {
	var saved CloudModelSetup
	panel := NewModelCatalogPanel(Config{
		LocalModels: &fakeLocalManager{},
		ProviderEndpoints: map[string]string{
			"azure-openai": "https://existing.openai.azure.com",
		},
		SaveCloudModel: func(setup CloudModelSetup) error {
			saved = setup
			return nil
		},
	})
	defer panel.Close()
	panel.model.width, panel.model.height = 100, 42
	form := newCloudModelSetupForm(modelprovider.AzureOpenAI, "review", "", nil, panel.model.inlineWidth())
	form.credential.SetValue("secret-api-key")
	form.endpoint.SetValue("example-resource.openai.azure.com")
	form.apiVersion.SetValue("2025-04-01-preview")
	form.focus = cloudSetupAPIVersionField
	panel.model.localModels.cloudSetup = &form

	if strings.Contains(panel.View(), "secret-api-key") {
		t.Fatal("cloud setup view exposed the typed credential")
	}
	_, command := panel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("save did not schedule the configuration callback")
	}
	panel.Update(command())
	if saved.APIKey != "secret-api-key" || saved.Model != "review" {
		t.Fatalf("saved setup = %#v", saved)
	}
	if panel.model.localModels.cloudSetup != nil || panel.model.provider.Value() != "azure-openai" || panel.model.model.Value() != "review" {
		t.Fatalf("selection = form:%#v provider:%q model:%q", panel.model.localModels.cloudSetup, panel.model.provider.Value(), panel.model.model.Value())
	}
	if strings.Contains(panel.View(), "secret-api-key") {
		t.Fatal("saved panel view exposed the typed credential")
	}
}

func TestEveryDirectCloudProviderHasAConfigurationForm(t *testing.T) {
	for _, providerName := range modelprovider.Names() {
		t.Run(providerName, func(t *testing.T) {
			provider, err := modelprovider.ParseProvider(providerName)
			if err != nil {
				t.Fatal(err)
			}
			endpoint := ""
			if provider == modelprovider.AzureOpenAI || provider == modelprovider.AzureOpenAIResponses {
				endpoint = "https://example-resource.openai.azure.com"
			}
			form := newCloudModelSetupForm(provider, "test-model", endpoint, nil, 100)
			if len(form.fields()) == 0 {
				t.Fatal("provider has no configuration fields")
			}
			if _, err := form.configuration(); err != nil {
				t.Fatalf("provider configuration form is unavailable: %v", err)
			}
		})
	}
}

func TestCustomProviderLifecycleRequiresReviewAndKeepsSecretsOut(t *testing.T) {
	backend := &fakeManagementBackend{}
	m := newModel(Config{LocalModels: &fakeLocalManager{}, ModelManagement: backend})
	m.width, m.height = 100, 42
	m.localModels.section = cloudModelSection

	next, _ := m.updateLocalModels(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = next.(Model)
	form := m.localModels.customSetup
	if form == nil {
		t.Fatal("custom provider form did not open")
	}
	form.id.SetValue("team-gateway")
	form.baseURL.SetValue("https://models.example.com/v1/chat/completions")
	form.apiKeyEnv.SetValue("TEAM_GATEWAY_API_KEY")
	form.models.SetValue("coding-large\ncoding-small")
	form.defaultModel.SetValue("coding-large")
	form.focus = customProviderDefaultField

	next, _ = m.updateLocalModels(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.localModels.confirmation != localModelConfirmSaveCustom || m.localModels.pendingCustom == nil {
		t.Fatal("custom provider was not staged for explicit review")
	}
	if strings.Contains(customProviderReviewText(*m.localModels.pendingCustom), "secret") {
		t.Fatal("custom-provider review rendered credential material")
	}
	next, command := m.updateLocalModels(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if command == nil {
		t.Fatal("confirmed custom provider did not schedule persistence")
	}
	next, _ = m.Update(command())
	m = next.(Model)
	if len(backend.providers) != 1 || backend.providers[0].APIKeyEnv != "TEAM_GATEWAY_API_KEY" {
		t.Fatalf("saved providers = %#v", backend.providers)
	}
	if m.provider.Value() != "team-gateway" || m.config.BaseURL != "https://models.example.com/v1/chat/completions" {
		t.Fatalf("selection = provider:%q endpoint:%q", m.provider.Value(), m.config.BaseURL)
	}

	m = selectCloudProvider(t, m, "team-gateway")
	next, _ = m.updateLocalModels(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(Model)
	if m.localModels.confirmation != localModelConfirmRemoveCustom {
		t.Fatal("custom provider removal skipped confirmation")
	}
	next, command = m.updateLocalModels(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(command())
	m = next.(Model)
	if len(backend.providers) != 0 {
		t.Fatalf("providers after removal = %#v", backend.providers)
	}
}

func TestLocalModelPullAndUseUpdateTheActiveSelection(t *testing.T) {
	manager := &lifecycleLocalManager{catalog: LocalCatalog{
		RuntimeURL: "http://127.0.0.1:11434", RuntimeVersion: "test", Executable: "/usr/bin/ollama",
		Models: []LocalModel{{ID: "reviewed", OllamaModel: "qwen2.5-coder:7b", Name: "Reviewed", Download: "4.7 GB"}},
	}}
	m := newModel(Config{LocalModels: manager})
	m.localModels.catalog = manager.catalog
	m.localModels.section = localModelSection
	m.localModels.action = localModelIdle

	next, _ := m.updateLocalModels(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = next.(Model)
	if m.localModels.confirmation != localModelConfirmPull {
		t.Fatal("local download skipped confirmation")
	}
	next, _ = m.updateLocalModels(tea.KeyMsg{Type: tea.KeyEnter})
	m = finishCatalogOperation(t, next.(Model))
	if !manager.pulled || !m.localModels.catalog.Models[0].Installed {
		t.Fatal("local model pull did not refresh installation state")
	}
	next, _ = m.updateLocalModels(tea.KeyMsg{Type: tea.KeyEnter})
	m = finishCatalogOperation(t, next.(Model))
	if !manager.used || m.provider.Value() != "gator-local" || m.model.Value() != "qwen2.5-coder:7b" {
		t.Fatalf("local use = used:%v provider:%q model:%q", manager.used, m.provider.Value(), m.model.Value())
	}
}

func selectCloudProvider(t *testing.T, m Model, provider string) Model {
	t.Helper()
	for index, entry := range m.cloudModels() {
		if entry.provider == provider {
			m.localModels.section = cloudModelSection
			m.localModels.cloudIndex = index
			return m
		}
	}
	t.Fatalf("cloud provider %q was not found", provider)
	return Model{}
}

func finishCatalogOperation(t *testing.T, m Model) Model {
	t.Helper()
	for range 8 {
		if m.localModels.operation == nil {
			return m
		}
		next, _ := m.Update(waitForLocalModelOperation(m.localModels.operation)())
		m = next.(Model)
	}
	t.Fatal("local model operation did not finish")
	return Model{}
}

type fakeManagementBackend struct {
	providers []config.CustomProvider
}

func (b *fakeManagementBackend) CredentialStatuses() ([]StoredCredentialStatus, error) {
	return nil, nil
}
func (b *fakeManagementBackend) RemoveCredential(provider string) (CredentialRemovalResult, error) {
	return CredentialRemovalResult{Provider: provider, StoreKey: provider, Removed: true}, nil
}
func (b *fakeManagementBackend) SaveCustomProvider(setup CustomProviderSetup) ([]config.CustomProvider, error) {
	b.providers = []config.CustomProvider{{ID: setup.ID, BaseURL: setup.BaseURL, APIKeyEnv: setup.APIKeyEnv, Models: setup.Models, DefaultModel: setup.DefaultModel}}
	return b.providers, nil
}
func (b *fakeManagementBackend) RemoveCustomProvider(string) ([]config.CustomProvider, error) {
	b.providers = nil
	return nil, nil
}
func (b *fakeManagementBackend) DiscoverCustomProvider(id string) (CustomProviderDiscovery, error) {
	return CustomProviderDiscovery{ID: id}, nil
}
func (b *fakeManagementBackend) ApplyCustomProviderDiscovery(string, []string) ([]config.CustomProvider, error) {
	return b.providers, nil
}

type lifecycleLocalManager struct {
	catalog LocalCatalog
	pulled  bool
	used    bool
}

func (m *lifecycleLocalManager) Status(context.Context) (LocalCatalog, error) { return m.catalog, nil }
func (m *lifecycleLocalManager) Start(context.Context) (LocalCatalog, error)  { return m.catalog, nil }
func (m *lifecycleLocalManager) Pull(_ context.Context, _ string, report func(LocalProgress)) (LocalCatalog, error) {
	m.pulled = true
	m.catalog.Models[0].Installed = true
	report(LocalProgress{Status: "complete", Completed: 1, Total: 1})
	return m.catalog, nil
}
func (m *lifecycleLocalManager) Use(context.Context, string) (LocalUpdate, error) {
	m.used = true
	return LocalUpdate{
		Catalog: m.catalog,
		CustomProviders: []config.CustomProvider{{
			ID: "gator-local", BaseURL: "http://127.0.0.1:11434/v1/chat/completions",
			Models: []string{"qwen2.5-coder:7b"}, DefaultModel: "qwen2.5-coder:7b",
		}},
		Provider: "gator-local", Model: "qwen2.5-coder:7b",
	}, nil
}
func (m *lifecycleLocalManager) Remove(context.Context, string) (LocalUpdate, error) {
	return LocalUpdate{Catalog: m.catalog}, nil
}
func (m *lifecycleLocalManager) Rename(context.Context, string, string, string) (map[string]string, error) {
	return nil, nil
}
