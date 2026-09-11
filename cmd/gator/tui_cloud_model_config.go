package main

import (
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/modelcatalog"
)

// saveTUICloudModelConfiguration keeps the UI's secret entry boundary small:
// the TUI only holds the typed value until this callback stores it in auth.json.
// Endpoint overrides remain non-secret settings shared by TUI and CLI runs.
func saveTUICloudModelConfiguration(settingsStore config.Store, stateDir string) func(modelcatalog.CloudModelSetup) error {
	return func(setup modelcatalog.CloudModelSetup) error {
		provider, err := model.ParseProvider(setup.Provider)
		if err != nil {
			return err
		}
		if !model.SupportsDirect(provider) {
			return fmt.Errorf("provider %q has no direct Gator cloud configuration", provider)
		}
		credentialProvider, credential, storeCredential, err := tuiCloudCredential(provider, setup.APIKey, setup.CredentialType)
		if err != nil {
			return err
		}
		settings, err := settingsStore.Load()
		if err != nil {
			return err
		}
		if provider != model.Claude {
			settings.Defaults.Provider = string(provider)
			settings.Defaults.Model = strings.TrimSpace(setup.Model)
		}
		if settings.ProviderEndpoints == nil {
			settings.ProviderEndpoints = make(map[string]string)
		}
		if baseURL := strings.TrimSpace(setup.BaseURL); baseURL == "" {
			delete(settings.ProviderEndpoints, string(provider))
		} else {
			settings.ProviderEndpoints[string(provider)] = baseURL
		}
		if settings.ProviderOptions == nil {
			settings.ProviderOptions = make(map[string]map[string]string)
		}
		if len(setup.Options) == 0 {
			delete(settings.ProviderOptions, string(provider))
		} else {
			settings.ProviderOptions[string(provider)] = cloneTUIProviderOptions(setup.Options)
		}
		if err := settingsStore.Save(settings); err != nil {
			return fmt.Errorf("save cloud model settings: %w", err)
		}

		if !storeCredential {
			return nil
		}
		credentials, err := auth.New(stateDir)
		if err != nil {
			return err
		}
		if err := credentials.Put(credentialProvider, credential); err != nil {
			return fmt.Errorf("store Gator cloud credential: %w", err)
		}
		return nil
	}
}

// tuiCloudCredential validates the credential mode before any non-secret
// settings are written. This prevents an account-mode submission from
// partially changing the selected provider or endpoint.
func tuiCloudCredential(provider model.Provider, value, credentialType string) (string, auth.Credential, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", auth.Credential{}, false, nil
	}
	credentialType = strings.TrimSpace(credentialType)
	switch {
	case provider == model.Claude:
		if credentialType != "" && credentialType != "api_key" {
			return "", auth.Credential{}, false, fmt.Errorf("Claude Code accepts an Anthropic API key")
		}
		return string(model.Anthropic), auth.Credential{Type: "api_key", Key: value}, true, nil
	case provider == model.AzureOpenAIResponses:
		if credentialType == "" || credentialType == "api_key" {
			return string(provider), auth.Credential{Type: "api_key", Key: value}, true, nil
		}
		if credentialType == "bearer_token" {
			return string(provider), auth.Credential{Type: "bearer_token", Access: value}, true, nil
		}
	case provider == model.AmazonBedrock || provider == model.GoogleVertex:
		if credentialType == "" || credentialType == "bearer_token" {
			return string(provider), auth.Credential{Type: "bearer_token", Access: value}, true, nil
		}
	case model.SupportsAPIKeyLogin(provider):
		if credentialType == "" || credentialType == "api_key" {
			return string(provider), auth.Credential{Type: "api_key", Key: value}, true, nil
		}
		return "", auth.Credential{}, false, fmt.Errorf("provider %q accepts an API key, not %q", provider, credentialType)
	default:
		return "", auth.Credential{}, false, fmt.Errorf("provider %q uses account sign-in; use the sign-in action in /model", provider)
	}
	return "", auth.Credential{}, false, fmt.Errorf("provider %q does not support credential type %q", provider, credentialType)
}

func cloneTUIProviderOptions(options map[string]string) map[string]string {
	cloned := make(map[string]string, len(options))
	for name, value := range options {
		cloned[name] = strings.TrimSpace(value)
	}
	return cloned
}
