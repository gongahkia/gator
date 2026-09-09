package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/config"
)

func TestConfigureSetsAndShowsDefaults(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	var output bytes.Buffer
	if err := configure([]string{"set", "default-provider", "anthropic"}, &output); err != nil {
		t.Fatalf("set provider: %v", err)
	}
	if !strings.Contains(output.String(), "Updated default-provider") {
		t.Fatalf("set provider output = %q", output.String())
	}
	output.Reset()
	if err := configure([]string{"set", "default-model", "claude-sonnet"}, &output); err != nil {
		t.Fatalf("set model: %v", err)
	}
	store, err := config.DefaultStore()
	if err != nil {
		t.Fatalf("default store: %v", err)
	}
	settings, err := store.Load()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if settings.Defaults.Provider != "anthropic" || settings.Defaults.Model != "claude-sonnet" {
		t.Fatalf("settings = %#v", settings)
	}
	output.Reset()
	if err := configure(nil, &output); err != nil {
		t.Fatalf("show configuration: %v", err)
	}
	if !strings.Contains(output.String(), `"provider": "anthropic"`) {
		t.Fatalf("show output = %q", output.String())
	}
}

func TestConfigureRejectsUnknownProvider(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	var output bytes.Buffer
	err := configure([]string{"set", "default-provider", "not-real"}, &output)
	if err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("configure error = %v", err)
	}
}

func TestConfiguredDefaultsPreservesEnvironmentOverride(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	store, err := config.DefaultStore()
	if err != nil {
		t.Fatalf("default store: %v", err)
	}
	settings := config.Default()
	settings.Defaults.Provider = "anthropic"
	settings.Defaults.Model = "configured-model"
	if err := store.Save(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	t.Setenv("GATOR_PROVIDER", "openai")
	t.Setenv("GATOR_MODEL", "environment-model")
	provider, err := providerFromEnvironment()
	if err != nil {
		t.Fatalf("provider from environment: %v", err)
	}
	if provider != "openai" || modelFromEnvironment(provider) != "environment-model" {
		t.Fatalf("provider/model = %q/%q", provider, modelFromEnvironment(provider))
	}
}

func TestSelectWorkOnboardingProviderPersistsProviderAndDefaultModel(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	if err := selectWorkOnboardingProvider("openai"); err != nil {
		t.Fatal(err)
	}
	store, err := config.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	settings, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Defaults.Provider != "openai" || settings.Defaults.Model == "" {
		t.Fatalf("Work defaults = %#v", settings.Defaults)
	}
}

func TestWorkTUIProviderCommandsKeepSecretsOutOfArguments(t *testing.T) {
	apiKeyLogin := workTUIProviderCommand("login", "openai")
	if got := strings.Join(apiKeyLogin.Args[1:], " "); got != "login openai --prompt" {
		t.Fatalf("API-key login args = %q", got)
	}
	oauthLogin := workTUIProviderCommand("login", "codex")
	if got := strings.Join(oauthLogin.Args[1:], " "); got != "login codex" {
		t.Fatalf("OAuth login args = %q", got)
	}
	setup := workTUIProviderCommand("setup", "anthropic")
	if got := strings.Join(setup.Args[1:], " "); got != "connect anthropic" {
		t.Fatalf("setup args = %q", got)
	}
}

func TestWorkTUIProviderPickersExposeOnlySupportedActions(t *testing.T) {
	if got := workTUIProviderChoices("setup"); !slices.Equal(got, []string{"openai", "anthropic", "gemini"}) {
		t.Fatalf("setup choices = %#v", got)
	}
	loginChoices := workTUIProviderChoices("login")
	if !slices.Contains(loginChoices, "openai") || !slices.Contains(loginChoices, "codex") || slices.Contains(loginChoices, "claude") || slices.Contains(loginChoices, "google-vertex") {
		t.Fatalf("login choices = %#v", loginChoices)
	}
	connectChoices := workTUIProviderChoices("connect")
	if !slices.Contains(connectChoices, "anthropic") || !slices.Contains(connectChoices, "claude") || slices.Contains(connectChoices, "google-vertex") {
		t.Fatalf("connect choices = %#v", connectChoices)
	}
}
