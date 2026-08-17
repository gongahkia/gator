package main

import (
	"bytes"
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
