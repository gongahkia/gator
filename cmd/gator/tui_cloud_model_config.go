package main

import (
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/tui"
)

// saveTUICloudModelConfiguration keeps the UI's secret entry boundary small:
// the TUI only holds the typed value until this callback stores it in auth.json.
// Endpoint overrides remain non-secret settings shared by TUI and CLI runs.
func saveTUICloudModelConfiguration(settingsStore config.Store, stateDir string) func(tui.CloudModelSetup) error {
	return func(setup tui.CloudModelSetup) error {
		provider, err := model.ParseProvider(setup.Provider)
		if err != nil {
			return err
		}
		if !model.SupportsDirect(provider) || !model.SupportsAPIKeyLogin(provider) {
			return fmt.Errorf("provider %q does not accept a Gator-managed API key", provider)
		}
		settings, err := settingsStore.Load()
		if err != nil {
			return err
		}
		settings.Defaults.Provider = string(provider)
		settings.Defaults.Model = strings.TrimSpace(setup.Model)
		if settings.ProviderEndpoints == nil {
			settings.ProviderEndpoints = make(map[string]string)
		}
		if baseURL := strings.TrimSpace(setup.BaseURL); baseURL == "" {
			delete(settings.ProviderEndpoints, string(provider))
		} else {
			settings.ProviderEndpoints[string(provider)] = baseURL
		}
		if err := settingsStore.Save(settings); err != nil {
			return fmt.Errorf("save cloud model settings: %w", err)
		}

		credential := strings.TrimSpace(setup.APIKey)
		if credential == "" {
			return nil
		}
		credentials, err := auth.New(stateDir)
		if err != nil {
			return err
		}
		credentialType := strings.TrimSpace(setup.CredentialType)
		switch credentialType {
		case "", "api_key":
			if err := credentials.Put(string(provider), auth.Credential{Type: "api_key", Key: credential}); err != nil {
				return fmt.Errorf("store Gator API key: %w", err)
			}
		case "bearer_token":
			if provider != model.AzureOpenAIResponses {
				return fmt.Errorf("provider %q does not support a Gator-managed bearer token", provider)
			}
			if err := credentials.Put(string(provider), auth.Credential{Type: "bearer_token", Access: credential}); err != nil {
				return fmt.Errorf("store Gator bearer token: %w", err)
			}
		default:
			return fmt.Errorf("unsupported cloud credential type %q", credentialType)
		}
		return nil
	}
}
