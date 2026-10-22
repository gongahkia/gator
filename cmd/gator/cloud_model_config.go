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
		credentialProvider, credential, storeCredential, err := tuiCloudCredential(provider, setup.APIKey)
		if err != nil {
			return err
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
func tuiCloudCredential(provider model.Provider, value string) (string, auth.Credential, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", auth.Credential{}, false, nil
	}
	if !model.SupportsAPIKeyLogin(provider) {
		return "", auth.Credential{}, false, fmt.Errorf("provider %q has no API-key setup", provider)
	}
	return string(provider), auth.Credential{Type: "api_key", Key: value}, true, nil
}

func cloneTUIProviderOptions(options map[string]string) map[string]string {
	cloned := make(map[string]string, len(options))
	for name, value := range options {
		cloned[name] = strings.TrimSpace(value)
	}
	return cloned
}
