package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/localmodel"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/tui"
)

type tuiModelManagementBackend struct {
	settings config.Store
	stateDir string
}

func newTUIModelManagementBackend(settings config.Store, stateDir string) tui.ModelManagementBackend {
	return &tuiModelManagementBackend{settings: settings, stateDir: stateDir}
}

func (backend *tuiModelManagementBackend) CredentialStatuses() ([]tui.StoredCredentialStatus, error) {
	credentials, err := auth.New(backend.stateDir)
	if err != nil {
		return nil, err
	}
	providers, err := credentials.Providers()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	statuses := make([]tui.StoredCredentialStatus, 0, len(providers))
	for _, provider := range providers {
		if strings.HasPrefix(provider, "mcp-") {
			continue
		}
		credential, found, err := credentials.Read(provider)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		statuses = append(statuses, storedCredentialStatus(provider, credential, now))
	}
	return statuses, nil
}

func (backend *tuiModelManagementBackend) RemoveCredential(providerName string) (tui.CredentialRemovalResult, error) {
	provider, err := model.ParseProvider(providerName)
	if err != nil {
		return tui.CredentialRemovalResult{}, err
	}
	if !model.SupportsDirect(provider) {
		return tui.CredentialRemovalResult{}, fmt.Errorf("provider %q has no Gator-managed cloud credential", provider)
	}
	storeKey := gatorCredentialStoreKey(provider)
	credentials, err := auth.New(backend.stateDir)
	if err != nil {
		return tui.CredentialRemovalResult{}, err
	}
	existing, found, err := credentials.Read(storeKey)
	if err != nil {
		return tui.CredentialRemovalResult{}, err
	}
	result := tui.CredentialRemovalResult{
		Provider:         string(provider),
		StoreKey:         storeKey,
		RemainingSources: remainingCredentialSources(provider),
	}
	if found {
		result.Kind = credentialKindLabel(existing)
		if err := credentials.Delete(storeKey); err != nil {
			return tui.CredentialRemovalResult{}, err
		}
		result.Removed = true
	}
	return result, nil
}

func (backend *tuiModelManagementBackend) SaveCustomProvider(setup tui.CustomProviderSetup) ([]config.CustomProvider, error) {
	provider, err := customProviderFromSetup(setup)
	if err != nil {
		return nil, err
	}
	if err := reservedCustomProviderIDError(provider.ID); err != nil {
		return nil, err
	}
	settings, err := backend.settings.Load()
	if err != nil {
		return nil, err
	}
	settings.CustomProviders = setCustomProvider(settings.CustomProviders, provider)
	if err := backend.settings.Save(settings); err != nil {
		return nil, err
	}
	return cloneCustomProviders(settings.CustomProviders), nil
}

func (backend *tuiModelManagementBackend) RemoveCustomProvider(id string) ([]config.CustomProvider, error) {
	id = strings.TrimSpace(id)
	if id == localmodel.ProviderID {
		return nil, fmt.Errorf("custom provider ID %q is reserved for the Local section of /model", id)
	}
	settings, err := backend.settings.Load()
	if err != nil {
		return nil, err
	}
	if !hasCustomProvider(settings.CustomProviders, id) {
		return nil, fmt.Errorf("custom provider %q is not configured", id)
	}
	settings.CustomProviders = removeCustomProvider(settings.CustomProviders, id)
	clearDeletedCustomProviderReferences(&settings, id)
	if err := backend.settings.Save(settings); err != nil {
		return nil, err
	}
	return cloneCustomProviders(settings.CustomProviders), nil
}

func (backend *tuiModelManagementBackend) DiscoverCustomProvider(id string) (tui.CustomProviderDiscovery, error) {
	id = strings.TrimSpace(id)
	if id == localmodel.ProviderID {
		return tui.CustomProviderDiscovery{}, fmt.Errorf("custom provider ID %q is reserved for the Local section of /model", id)
	}
	settings, err := backend.settings.Load()
	if err != nil {
		return tui.CustomProviderDiscovery{}, err
	}
	provider, found := findCustomProvider(settings.CustomProviders, id)
	if !found {
		return tui.CustomProviderDiscovery{}, fmt.Errorf("custom provider %q is not configured", id)
	}
	models, err := discoverModels(provider)
	if err != nil {
		return tui.CustomProviderDiscovery{}, err
	}
	if len(models) == 0 {
		return tui.CustomProviderDiscovery{}, fmt.Errorf("custom provider %q returned no model IDs", provider.ID)
	}
	defaultModel := provider.DefaultModel
	if !customProviderSupportsModel(config.CustomProvider{Models: models, DefaultModel: defaultModel}, defaultModel) {
		defaultModel = models[0]
	}
	return tui.CustomProviderDiscovery{ID: provider.ID, Models: models, DefaultModel: defaultModel}, nil
}

func (backend *tuiModelManagementBackend) ApplyCustomProviderDiscovery(id string, models []string) ([]config.CustomProvider, error) {
	id = strings.TrimSpace(id)
	if id == localmodel.ProviderID {
		return nil, fmt.Errorf("custom provider ID %q is reserved for the Local section of /model", id)
	}
	settings, err := backend.settings.Load()
	if err != nil {
		return nil, err
	}
	provider, found := findCustomProvider(settings.CustomProviders, id)
	if !found {
		return nil, fmt.Errorf("custom provider %q is not configured", id)
	}
	cleaned, err := normalizeCustomProviderModels(models)
	if err != nil {
		return nil, err
	}
	provider.Models = cleaned
	if !customProviderSupportsModel(provider, provider.DefaultModel) {
		provider.DefaultModel = cleaned[0]
	}
	settings.CustomProviders = setCustomProvider(settings.CustomProviders, provider)
	if err := backend.settings.Save(settings); err != nil {
		return nil, err
	}
	return cloneCustomProviders(settings.CustomProviders), nil
}

func customProviderFromSetup(setup tui.CustomProviderSetup) (config.CustomProvider, error) {
	models, err := normalizeCustomProviderModels(setup.Models)
	if err != nil {
		return config.CustomProvider{}, err
	}
	defaultModel := strings.TrimSpace(setup.DefaultModel)
	if defaultModel == "" {
		defaultModel = models[0]
	}
	return config.CustomProvider{
		ID:           strings.TrimSpace(setup.ID),
		BaseURL:      strings.TrimSpace(setup.BaseURL),
		APIKeyEnv:    strings.TrimSpace(setup.APIKeyEnv),
		Models:       models,
		DefaultModel: defaultModel,
	}, nil
}

func normalizeCustomProviderModels(models []string) ([]string, error) {
	cleaned := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		if len(modelName) > 512 || strings.ContainsAny(modelName, "\r\n") {
			return nil, fmt.Errorf("custom provider model %q is invalid", modelName)
		}
		if _, exists := seen[modelName]; exists {
			continue
		}
		seen[modelName] = struct{}{}
		cleaned = append(cleaned, modelName)
	}
	if len(cleaned) == 0 {
		return nil, fmt.Errorf("a custom provider requires at least one model ID")
	}
	if len(cleaned) > 128 {
		return nil, fmt.Errorf("a custom provider supports at most 128 models")
	}
	return cleaned, nil
}

func clearDeletedCustomProviderReferences(settings *config.Settings, id string) {
	if strings.EqualFold(strings.TrimSpace(settings.Defaults.Provider), id) {
		settings.Defaults.Provider = ""
		settings.Defaults.Model = ""
	}
	if len(settings.ModelAliases) == 0 {
		return
	}
	prefix := strings.ToLower(id) + ":"
	for key := range settings.ModelAliases {
		if strings.HasPrefix(strings.ToLower(key), prefix) {
			delete(settings.ModelAliases, key)
		}
	}
}

func storedCredentialStatus(storeKey string, credential auth.Credential, now time.Time) tui.StoredCredentialStatus {
	return tui.StoredCredentialStatus{
		Provider: storeKey,
		StoreKey: storeKey,
		Present:  true,
		Kind:     credentialKindLabel(credential),
		Expired:  credential.Expired(now),
	}
}

func credentialKindLabel(credential auth.Credential) string {
	switch {
	case credential.IsOAuth():
		return "OAuth credential"
	case credential.IsBearerToken():
		return "bearer token"
	default:
		return "API key"
	}
}

func gatorCredentialStoreKey(provider model.Provider) string {
	if provider == model.Claude {
		return string(model.Anthropic)
	}
	return string(provider)
}

func remainingCredentialSources(provider model.Provider) []string {
	sources := make([]string, 0, 4)
	seen := map[string]struct{}{}
	add := func(source string) {
		source = strings.TrimSpace(source)
		if source == "" {
			return
		}
		if _, exists := seen[source]; exists {
			return
		}
		seen[source] = struct{}{}
		sources = append(sources, source)
	}
	addEnv := func(name string) {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			add(name)
		}
	}
	switch provider {
	case model.Claude, model.Anthropic:
		addEnv("ANTHROPIC_API_KEY")
	case model.AzureOpenAI, model.AzureOpenAIResponses:
		addEnv("AZURE_OPENAI_API_KEY")
		addEnv("AZURE_OPENAI_AUTH_TOKEN")
	case model.AmazonBedrock:
		addEnv("AWS_BEARER_TOKEN_BEDROCK")
		if model.AmbientCredentialAvailable(provider) {
			if source := model.AmbientCredentialSource(provider); source != "" {
				add(source)
			} else {
				add("standard AWS credential chain")
			}
		}
	case model.GoogleVertex:
		addEnv("GATOR_VERTEX_ACCESS_TOKEN")
		addEnv("GOOGLE_APPLICATION_CREDENTIALS")
		if model.AmbientCredentialAvailable(provider) {
			add("Google Application Default Credentials")
		}
	default:
		addEnv(model.APIKeyEnvironment(provider))
		if model.AmbientCredentialAvailable(provider) {
			if source := model.AmbientCredentialSource(provider); source != "" {
				add(source)
			} else {
				add(model.CredentialHint(provider))
			}
		}
	}
	return sources
}

func describeCredentialRemoval(result tui.CredentialRemovalResult) string {
	if result.Removed {
		text := "Removed the Gator " + result.Kind + " for " + result.Provider + " from Gator's private credential file."
		if len(result.RemainingSources) > 0 {
			return text + " This process can still authenticate via " + strings.Join(result.RemainingSources, ", ") + "."
		}
		return text + " No remaining ambient credential source was detected in this process."
	}
	text := "No Gator credential was stored for " + result.Provider + "."
	if len(result.RemainingSources) > 0 {
		return text + " This process can still authenticate via " + strings.Join(result.RemainingSources, ", ") + "."
	}
	return text
}
