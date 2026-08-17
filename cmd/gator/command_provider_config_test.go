package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/config"
)

func TestProviderCommandPersistsKeylessLocalEndpoint(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	var output bytes.Buffer
	err := providerCommand([]string{"add", "local-llm", "--base-url", "http://127.0.0.1:11434/v1/chat/completions", "--model", "qwen3-coder"}, &output)
	if err != nil {
		t.Fatalf("add provider: %v", err)
	}
	store, err := config.DefaultStore()
	if err != nil {
		t.Fatalf("config store: %v", err)
	}
	settings, err := store.Load()
	if err != nil || len(settings.CustomProviders) != 1 {
		t.Fatalf("providers = %#v, err = %v", settings.CustomProviders, err)
	}
	provider := settings.CustomProviders[0]
	if provider.ID != "local-llm" || provider.DefaultModel != "qwen3-coder" || provider.APIKeyEnv != "" {
		t.Fatalf("provider = %#v", provider)
	}
	resolved, modelName, err := resolveConfiguredProvider("local-llm", "")
	if err != nil || resolved != "local-llm" || modelName != "qwen3-coder" {
		t.Fatalf("resolve = %q %q %v", resolved, modelName, err)
	}
	executor, err := newExecutor("local-llm", "", "")
	if err != nil || executor.Model == nil {
		t.Fatalf("new executor = %#v, err = %v", executor, err)
	}
	output.Reset()
	if err := providerCommand([]string{"list"}, &output); err != nil {
		t.Fatalf("list provider: %v", err)
	}
	if !strings.Contains(output.String(), "local-llm") || !strings.Contains(output.String(), "no API key") {
		t.Fatalf("list output = %q", output.String())
	}
}

func TestProviderCommandRejectsBuiltInProviderID(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	var output bytes.Buffer
	err := providerCommand([]string{"add", "openai", "--base-url", "http://127.0.0.1:11434/v1/chat/completions", "--model", "qwen3-coder"}, &output)
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("add built-in provider error = %v", err)
	}
}
