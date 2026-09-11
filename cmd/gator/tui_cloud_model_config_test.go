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

func TestSaveTUICloudModelConfigurationStoresAzureSettingsAndCredential(t *testing.T) {
	settingsStore, err := config.New(t.TempDir())
	if err != nil {
		t.Fatalf("new settings store: %v", err)
	}
	stateDir := t.TempDir()
	save := saveTUICloudModelConfiguration(settingsStore, stateDir)
	endpoint := "https://example-resource.openai.azure.com/openai/deployments/review/chat/completions?api-version=2025-04-01-preview"
	if err := save(modelcatalog.CloudModelSetup{
		Provider:       "azure-openai",
		Model:          "review",
		BaseURL:        endpoint,
		APIKey:         "test-api-key",
		CredentialType: "api_key",
	}); err != nil {
		t.Fatalf("save Azure configuration: %v", err)
	}

	settings, err := settingsStore.Load()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if settings.Defaults.Provider != "azure-openai" || settings.Defaults.Model != "review" {
		t.Fatalf("defaults = %#v", settings.Defaults)
	}
	if got := settings.ProviderEndpoint("azure-openai"); got != endpoint {
		t.Fatalf("Azure endpoint = %q", got)
	}
	contents, err := os.ReadFile(settingsStore.Path())
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if strings.Contains(string(contents), "test-api-key") {
		t.Fatal("settings file contains the API key")
	}

	credentials, err := auth.New(stateDir)
	if err != nil {
		t.Fatalf("new credential store: %v", err)
	}
	credential, found, err := credentials.Read("azure-openai")
	if err != nil {
		t.Fatalf("read credential: %v", err)
	}
	if !found || !credential.IsAPIKey() || credential.Key != "test-api-key" {
		t.Fatalf("Azure credential was not stored as an API key")
	}
}

func TestSaveTUICloudModelConfigurationStoresAzureResponsesBearerToken(t *testing.T) {
	settingsStore, err := config.New(t.TempDir())
	if err != nil {
		t.Fatalf("new settings store: %v", err)
	}
	stateDir := t.TempDir()
	if err := saveTUICloudModelConfiguration(settingsStore, stateDir)(modelcatalog.CloudModelSetup{
		Provider:       "azure-openai-responses",
		Model:          "review",
		BaseURL:        "https://example-resource.openai.azure.com/openai/v1/responses?api-version=v1",
		APIKey:         "test-bearer-token",
		CredentialType: "bearer_token",
	}); err != nil {
		t.Fatalf("save Azure Responses configuration: %v", err)
	}
	credentials, err := auth.New(stateDir)
	if err != nil {
		t.Fatalf("new credential store: %v", err)
	}
	credential, found, err := credentials.Read("azure-openai-responses")
	if err != nil {
		t.Fatalf("read credential: %v", err)
	}
	if !found || !credential.IsBearerToken() || credential.Access != "test-bearer-token" {
		t.Fatalf("Azure Responses credential was not stored as a bearer token")
	}
}

func TestSaveTUICloudModelConfigurationStoresSpecialProviderCredentialsAndMetadata(t *testing.T) {
	settingsStore, err := config.New(t.TempDir())
	if err != nil {
		t.Fatalf("new settings store: %v", err)
	}
	stateDir := t.TempDir()
	save := saveTUICloudModelConfiguration(settingsStore, stateDir)
	for _, setup := range []modelcatalog.CloudModelSetup{
		{Provider: "claude", Model: "claude-sonnet-5", APIKey: "anthropic-key", CredentialType: "api_key", DelegateRuntime: "claude"},
		{Provider: "amazon-bedrock", Model: "openai.gpt-oss-20b-1:0", APIKey: "bedrock-token", CredentialType: "bearer_token", Options: map[string]string{"region": "eu-west-1", "profile": "engineering"}},
		{Provider: "google-vertex", Model: "google/gemini-2.0-flash-001", APIKey: "vertex-token", CredentialType: "bearer_token", Options: map[string]string{"project": "project-123", "location": "us-central1", "credentials_path": "/tmp/gator-adc.json"}},
		{Provider: "cloudflare-ai-gateway", Model: "openai/gpt-5.6", APIKey: "cloudflare-token", CredentialType: "api_key", Options: map[string]string{"account_id": "account-123", "gateway_id": "gateway-123", "gateway_protocol": "openai-responses"}},
		{Provider: "codex", Model: "gpt-5.6"},
	} {
		if err := save(setup); err != nil {
			t.Fatalf("save %s configuration: %v", setup.Provider, err)
		}
	}

	settings, err := settingsStore.Load()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if options := settings.OptionsForProvider("cloudflare-ai-gateway"); options["gateway_id"] != "gateway-123" || options["gateway_protocol"] != "openai-responses" {
		t.Fatalf("Cloudflare settings = %#v", options)
	}
	if options := settings.OptionsForProvider("google-vertex"); options["project"] != "project-123" || options["location"] != "us-central1" {
		t.Fatalf("Vertex settings = %#v", options)
	}
	if options := settings.OptionsForProvider("amazon-bedrock"); options["profile"] != "engineering" {
		t.Fatalf("Bedrock settings = %#v", options)
	}
	contents, err := os.ReadFile(settingsStore.Path())
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	for _, secret := range []string{"anthropic-key", "bedrock-token", "vertex-token", "cloudflare-token"} {
		if strings.Contains(string(contents), secret) {
			t.Fatalf("settings file contains cloud credential %q", secret)
		}
	}

	credentials, err := auth.New(stateDir)
	if err != nil {
		t.Fatalf("new credential store: %v", err)
	}
	for _, expected := range []struct {
		provider string
		bearer   bool
	}{
		{provider: "anthropic"},
		{provider: "amazon-bedrock", bearer: true},
		{provider: "google-vertex", bearer: true},
		{provider: "cloudflare-ai-gateway"},
	} {
		credential, found, err := credentials.Read(expected.provider)
		if err != nil {
			t.Fatalf("read %s credential: %v", expected.provider, err)
		}
		if !found || (expected.bearer && !credential.IsBearerToken()) || (!expected.bearer && !credential.IsAPIKey()) {
			t.Fatalf("credential metadata for %s is incorrect", expected.provider)
		}
	}
	if _, found, err := credentials.Read("codex"); err != nil || found {
		t.Fatalf("Codex form should not store an OAuth credential: found=%t err=%v", found, err)
	}
}

func TestSaveTUICloudModelConfigurationRejectsTypedCredentialForAccountProviderBeforeSettingsChange(t *testing.T) {
	settingsStore, err := config.New(t.TempDir())
	if err != nil {
		t.Fatalf("new settings store: %v", err)
	}
	err = saveTUICloudModelConfiguration(settingsStore, t.TempDir())(modelcatalog.CloudModelSetup{
		Provider:       "codex",
		Model:          "gpt-5.6",
		APIKey:         "must-not-be-persisted",
		CredentialType: "api_key",
	})
	if err == nil || !strings.Contains(err.Error(), "account sign-in") {
		t.Fatalf("account credential error = %v", err)
	}
	settings, err := settingsStore.Load()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if settings.Defaults.Provider != "" || settings.Defaults.Model != "" || len(settings.ProviderEndpoints) != 0 || len(settings.ProviderOptions) != 0 {
		t.Fatalf("invalid credential unexpectedly changed settings: %#v", settings)
	}
	if contents, readErr := os.ReadFile(settingsStore.Path()); readErr == nil && strings.Contains(string(contents), "must-not-be-persisted") {
		t.Fatal("settings file contains rejected account credential")
	}
}

func TestSaveTUICloudModelConfigurationSupportsEveryDirectProvider(t *testing.T) {
	settingsStore, err := config.New(t.TempDir())
	if err != nil {
		t.Fatalf("new settings store: %v", err)
	}
	save := saveTUICloudModelConfiguration(settingsStore, t.TempDir())
	for _, providerName := range model.Names() {
		t.Run(providerName, func(t *testing.T) {
			provider, err := model.ParseProvider(providerName)
			if err != nil {
				t.Fatalf("parse provider: %v", err)
			}
			setup := modelcatalog.CloudModelSetup{Provider: providerName, Model: "test-model"}
			switch provider {
			case model.Codex, model.Copilot:
				// OAuth is configured by the in-TUI sign-in action, not a text field.
			case model.Claude:
				setup.APIKey, setup.CredentialType = "test-api-key", "api_key"
			case model.AmazonBedrock, model.GoogleVertex:
				setup.APIKey, setup.CredentialType = "test-bearer-token", "bearer_token"
			default:
				setup.APIKey, setup.CredentialType = "test-api-key", "api_key"
			}
			if err := save(setup); err != nil {
				t.Fatalf("save provider configuration: %v", err)
			}
		})
	}
}
