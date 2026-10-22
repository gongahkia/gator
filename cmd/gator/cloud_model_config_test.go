package main

import (
	"os"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/modelcatalog"
)

func TestSaveTUICloudModelConfigurationStoresAPIKeyAndSettings(t *testing.T) {
	settingsStore, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	endpoint := "https://example-resource.openai.azure.com/openai/deployments/review/chat/completions?api-version=2025-04-01-preview"
	err = saveTUICloudModelConfiguration(settingsStore, stateDir)(modelcatalog.CloudModelSetup{
		Provider: "azure-openai", Model: "review", BaseURL: endpoint, APIKey: "test-api-key",
	})
	if err != nil {
		t.Fatalf("save Azure configuration: %v", err)
	}
	settings, err := settingsStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Defaults.Provider != "azure-openai" || settings.Defaults.Model != "review" || settings.ProviderEndpoint("azure-openai") != endpoint {
		t.Fatalf("settings = %#v", settings)
	}
	contents, err := os.ReadFile(settingsStore.Path())
	if err != nil || strings.Contains(string(contents), "test-api-key") {
		t.Fatalf("settings leaked API key: %v %s", err, contents)
	}
	credentials, err := auth.New(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	credential, found, err := credentials.Read("azure-openai")
	if err != nil || !found || !credential.IsAPIKey() || credential.Key != "test-api-key" {
		t.Fatalf("stored credential = %#v found=%v err=%v", credential, found, err)
	}
}

func TestSaveTUICloudModelConfigurationRejectsRemovedProviderWithoutWriting(t *testing.T) {
	settingsStore, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = saveTUICloudModelConfiguration(settingsStore, t.TempDir())(modelcatalog.CloudModelSetup{
		Provider: "codex", Model: "gpt-5.6", APIKey: "must-not-be-persisted",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("removed provider error = %v", err)
	}
	settings, err := settingsStore.Load()
	if err != nil || settings.Defaults.Provider != "" || settings.Defaults.Model != "" {
		t.Fatalf("settings unexpectedly changed: %#v err=%v", settings, err)
	}
}

func TestSaveTUICloudModelConfigurationSupportsEveryAPIKeyProvider(t *testing.T) {
	settingsStore, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	save := saveTUICloudModelConfiguration(settingsStore, t.TempDir())
	for _, providerName := range model.Names() {
		t.Run(providerName, func(t *testing.T) {
			provider, err := model.ParseProvider(providerName)
			if err != nil {
				t.Fatal(err)
			}
			if !model.SupportsAPIKeyLogin(provider) {
				t.Fatalf("%s lacks API-key setup", provider)
			}
			if err := save(modelcatalog.CloudModelSetup{Provider: providerName, Model: "test-model", APIKey: "test-api-key"}); err != nil {
				t.Fatalf("save provider configuration: %v", err)
			}
		})
	}
}
