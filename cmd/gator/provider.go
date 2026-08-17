package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/model"
	gatorrun "github.com/gongahkia/gator/internal/run"
)

func providerFromEnvironment() (model.Provider, error) {
	defaults, err := configuredDefaults()
	if err != nil {
		return "", err
	}
	provider := defaults.Provider
	if configured := os.Getenv("GATOR_PROVIDER"); configured != "" {
		provider = configured
	}
	parsed, err := model.ParseProvider(provider)
	if err != nil {
		return "", fmt.Errorf("GATOR_PROVIDER: %w", err)
	}
	return parsed, nil
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

func newExecutor(providerName, modelName, baseURL string) (gatorrun.Executor, error) {
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
	return gatorrun.Executor{Model: backend.Model}, nil
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
