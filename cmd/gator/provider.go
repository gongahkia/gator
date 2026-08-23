package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/extension"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/model/chatcompletions"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/tools"
)

func providerFromEnvironment() (model.Provider, error) {
	provider, err := providerNameFromEnvironment()
	if err != nil {
		return "", err
	}
	parsed, err := model.ParseProvider(provider)
	if err != nil {
		return "", fmt.Errorf("GATOR_PROVIDER: %w", err)
	}
	return parsed, nil
}

func providerNameFromEnvironment() (string, error) {
	defaults, err := configuredDefaults()
	if err != nil {
		return "", err
	}
	provider := defaults.Provider
	if configured := os.Getenv("GATOR_PROVIDER"); configured != "" {
		provider = configured
	}
	if _, _, err := resolveConfiguredProvider(provider, ""); err != nil {
		return "", fmt.Errorf("GATOR_PROVIDER: %w", err)
	}
	return provider, nil
}

func modelFromEnvironment(provider model.Provider) string {
	if model := os.Getenv("GATOR_MODEL"); model != "" {
		return model
	}
	if defaults, err := configuredDefaults(); err == nil && defaults.Model != "" {
		return defaults.Model
	}
	return model.DefaultModel(provider)
}

func modelFromProviderName(provider string) string {
	if modelName := os.Getenv("GATOR_MODEL"); modelName != "" {
		return modelName
	}
	if _, modelName, err := resolveConfiguredProvider(provider, ""); err == nil {
		return modelName
	}
	return ""
}

func newExecutor(providerName, modelName, baseURL string) (gatorrun.Executor, error) {
	settings, err := loadSettings()
	if err != nil {
		return gatorrun.Executor{}, err
	}
	custom, found := configuredCustomProvider(settings, providerName)
	if found {
		return newCustomExecutor(settings, custom, modelName, baseURL)
	}
	provider, err := model.ParseProvider(providerName)
	if err != nil {
		return gatorrun.Executor{}, err
	}
	if provider == model.Claude {
		return gatorrun.Executor{}, fmt.Errorf("Claude.ai subscription OAuth is not a supported native Gator provider; use provider %q with an API key or 'gator delegate claude run ...'", model.Anthropic)
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return gatorrun.Executor{}, err
	}
	if err := refreshProviderCredential(context.Background(), provider, credentials, oauthFlow, time.Now()); err != nil {
		return gatorrun.Executor{}, err
	}
	backend, err := model.New(model.Config{Provider: provider, Model: modelName, BaseURL: baseURL, Credentials: &credentials})
	if err != nil {
		return gatorrun.Executor{}, err
	}
	return executorWithExtensions(backend.Model, settings)
}

func newCustomExecutor(settings config.Settings, provider config.CustomProvider, modelName, baseURL string) (gatorrun.Executor, error) {
	if strings.TrimSpace(modelName) == "" {
		modelName = provider.DefaultModel
	}
	if !customProviderSupportsModel(provider, modelName) {
		return gatorrun.Executor{}, fmt.Errorf("model %q is not configured for custom provider %q", modelName, provider.ID)
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = provider.BaseURL
	}
	apiKey := ""
	if provider.APIKeyEnv != "" {
		apiKey = os.Getenv(provider.APIKeyEnv)
	}
	backend := chatcompletions.Model{Config: chatcompletions.Config{
		APIKey:           apiKey,
		APIKeyEnv:        provider.APIKeyEnv,
		AllowEmptyAPIKey: provider.APIKeyEnv == "",
		BaseURL:          baseURL,
		Model:            modelName,
		ProviderName:     "custom provider " + provider.ID,
	}}
	return executorWithExtensions(backend, settings)
}

func executorWithExtensions(backend agent.Model, settings config.Settings) (gatorrun.Executor, error) {
	extensions, err := extension.DefaultResolver(settings)
	if err != nil {
		return gatorrun.Executor{}, err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return gatorrun.Executor{}, err
	}
	return gatorrun.Executor{
		Model:          backend,
		Extensions:     extensions,
		HookTrusts:     settings.HookTrusts,
		LSPTrusts:      settings.LSPTrusts,
		MCPTrusts:      settings.MCPTrusts,
		MCPCredentials: credentials,
		HTTP: tools.HTTPFetchOptions{
			BraveSearchAPIKey: strings.TrimSpace(os.Getenv("BRAVE_SEARCH_API_KEY")),
		},
		Sandbox: settings.Execution,
	}, nil
}

// resolveConfiguredProvider uses the persisted custom-model catalog first,
// then Gator's built-in direct adapters. It returns the default model whenever
// callers did not specify one.
func resolveConfiguredProvider(providerName, requestedModel string) (string, string, error) {
	settings, err := loadSettings()
	if err != nil {
		return "", "", err
	}
	if custom, found := configuredCustomProvider(settings, providerName); found {
		modelName := strings.TrimSpace(requestedModel)
		if modelName == "" {
			modelName = custom.DefaultModel
		}
		if !customProviderSupportsModel(custom, modelName) {
			return "", "", fmt.Errorf("model %q is not configured for custom provider %q", modelName, custom.ID)
		}
		return custom.ID, modelName, nil
	}
	provider, err := model.ParseProvider(providerName)
	if err != nil {
		return "", "", err
	}
	return string(provider), model.EffectiveModel(provider, requestedModel), nil
}

func loadSettings() (config.Settings, error) {
	store, err := config.DefaultStore()
	if err != nil {
		return config.Settings{}, err
	}
	return store.Load()
}

func configuredCustomProvider(settings config.Settings, providerName string) (config.CustomProvider, bool) {
	for _, provider := range settings.CustomProviders {
		if strings.EqualFold(provider.ID, strings.TrimSpace(providerName)) {
			return provider, true
		}
	}
	return config.CustomProvider{}, false
}

func customProviderSupportsModel(provider config.CustomProvider, modelName string) bool {
	for _, configured := range provider.Models {
		if configured == modelName {
			return true
		}
	}
	return false
}

func refreshProviderCredential(ctx context.Context, provider model.Provider, credentials auth.Store, flowFor func(model.Provider) (auth.BrowserFlow, error), now time.Time) error {
	if !model.SupportsOAuthLogin(provider) {
		return nil
	}
	credential, found, err := credentials.Read(string(provider))
	if err != nil || !found || !credential.IsOAuth() || credential.Expires == 0 || credential.Expires > now.Add(5*time.Minute).UnixMilli() {
		return err
	}
	if provider == model.Copilot {
		refreshContext, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		refreshed, err := refreshCopilotCredential(refreshContext, credential)
		if err != nil {
			return fmt.Errorf("refresh %s OAuth credential: %w", provider, err)
		}
		if err := credentials.Put(string(provider), refreshed); err != nil {
			return fmt.Errorf("store refreshed %s OAuth credential: %w", provider, err)
		}
		return nil
	}
	flow, err := flowFor(provider)
	if err != nil {
		return fmt.Errorf("refresh %s OAuth credential: %w", provider, err)
	}
	refreshContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	refreshed, err := flow.Refresh(refreshContext, credential)
	if err != nil {
		return fmt.Errorf("refresh %s OAuth credential: %w", provider, err)
	}
	if err := credentials.Put(string(provider), refreshed); err != nil {
		return fmt.Errorf("store refreshed %s OAuth credential: %w", provider, err)
	}
	return nil
}

func gatorCredentials() (auth.Store, error) {
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return auth.Store{}, err
	}
	return auth.New(stateDir)
}

func displayModel(value string) string {
	if strings.TrimSpace(value) == "" {
		return "provider default"
	}
	return value
}
