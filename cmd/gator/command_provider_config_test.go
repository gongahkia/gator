package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
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

func TestProviderDiscoverShowsAndAppliesOpenAICompatibleModelCatalog(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/models" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		_, _ = writer.Write([]byte(`{"data":[{"id":"large"},{"id":"small"},{"id":"large"}]}`))
	}))
	defer server.Close()
	store, err := config.DefaultStore()
	if err != nil {
		t.Fatalf("config store: %v", err)
	}
	settings := config.Default()
	settings.CustomProviders = []config.CustomProvider{{ID: "local-llm", BaseURL: server.URL + "/v1/chat/completions", Models: []string{"old"}, DefaultModel: "old"}}
	if err := store.Save(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	var output bytes.Buffer
	if err := providerCommand([]string{"discover", "local-llm"}, &output); err != nil {
		t.Fatalf("discover: %v", err)
	}
	if !strings.Contains(output.String(), "large") || !strings.Contains(output.String(), "--apply") {
		t.Fatalf("discover output = %q", output.String())
	}
	if err := providerCommand([]string{"discover", "local-llm", "--apply"}, &output); err != nil {
		t.Fatalf("apply discovery: %v", err)
	}
	settings, err = store.Load()
	if err != nil || strings.Join(settings.CustomProviders[0].Models, ",") != "large,small" || settings.CustomProviders[0].DefaultModel != "large" {
		t.Fatalf("discovered settings = %#v, err = %v", settings.CustomProviders, err)
	}
}
