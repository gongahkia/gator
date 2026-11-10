package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/localmodel"
)

func TestWorkModelProcessingDistinguishesLocalEndpointAndRemoteWithoutURLSecrets(t *testing.T) {
	for _, test := range []struct{ provider, endpoint, kind, destination string }{
		{localmodel.ProviderID, "http://127.0.0.1:11434/v1/chat/completions", "local", "127.0.0.1:11434"},
		{"custom", "http://localhost:9000/v1/chat/completions", "local_endpoint", "localhost:9000"},
		{"custom", "http://[::1]:9000/v1/chat/completions", "local_endpoint", "[::1]:9000"},
		{"custom", "https://user:private-password@model.example/private-path?key=private-query", "remote", "model.example"},
		{"custom", "https://localhost.model.example/v1/chat/completions", "remote", "localhost.model.example"},
		{"openai", "", "remote", "openai"},
		{"custom", "not-an-endpoint", "unknown", ""},
	} {
		t.Run(test.endpoint, func(t *testing.T) {
			got := workModelProcessing(test.provider, test.endpoint)
			if got.Kind != test.kind || got.Destination != test.destination || got.Description == "" {
				t.Fatalf("processing = %#v", got)
			}
			encoded, _ := json.Marshal(got)
			for _, secret := range []string{"private-password", "private-path", "private-query", "user:"} {
				if strings.Contains(string(encoded), secret) {
					t.Fatal("processing exposed URL material", string(encoded))
				}
			}
		})
	}
}

func TestWorkModelReadinessChecksConfigurationWithoutCallingTheEndpoint(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	t.Setenv("OPENAI_API_KEY", "fixture-private-key")
	store, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings := config.Default()
	settings.Defaults = config.Defaults{Provider: "openai", Model: "gpt-5.6"}
	settings.ProviderEndpoints = map[string]string{"openai": server.URL}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	status, err := currentWorkModelStatus(store, t.TempDir())
	if err != nil || status.Issue != "" || status.Processing.Kind != "local_endpoint" || calls.Load() != 0 {
		t.Fatalf("readiness = %#v, %v; calls=%d", status, err, calls.Load())
	}
}

func TestWorkModelReadinessExplainsMissingCredentialsAndProviderConfiguration(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	store, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings := config.Default()
	settings.Defaults = config.Defaults{Provider: "openai", Model: "gpt-5.6"}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	status, err := currentWorkModelStatus(store, t.TempDir())
	if err != nil || !strings.Contains(status.Issue, "OPENAI_API_KEY") {
		t.Fatalf("missing credential = %#v, %v", status, err)
	}
	t.Setenv("AZURE_OPENAI_API_KEY", "fixture-private-key")
	t.Setenv("AZURE_OPENAI_BASE_URL", "")
	t.Setenv("AZURE_OPENAI_RESOURCE_NAME", "")
	settings.Defaults = config.Defaults{Provider: "azure-openai-responses", Model: "deployment"}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	status, err = currentWorkModelStatus(store, t.TempDir())
	if err != nil || status.Issue == "" {
		t.Fatalf("incomplete provider = %#v, %v", status, err)
	}
	settings.ProviderEndpoints = map[string]string{"azure-openai-responses": "https://resource.example/openai/v1"}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	status, err = currentWorkModelStatus(store, t.TempDir())
	if err != nil || status.Issue != "" {
		t.Fatalf("repaired provider = %#v, %v", status, err)
	}
	settings.ProviderEndpoints = nil
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AZURE_OPENAI_BASE_URL", "http://127.0.0.1:9000/openai/v1")
	status, err = currentWorkModelStatus(store, t.TempDir())
	if err != nil || status.Issue != "" || status.Processing.Kind != "local_endpoint" || status.Processing.Destination != "127.0.0.1:9000" {
		t.Fatalf("actual environment-resolved endpoint = %#v, %v", status, err)
	}
}

func TestCurrentWorkModelStatusReportsConfiguredLocalModel(t *testing.T) {
	store, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings := config.Default()
	settings.Defaults = config.Defaults{Provider: localmodel.ProviderID, Model: "qwen2.5-coder:7b"}
	settings.CustomProviders = []config.CustomProvider{{
		ID: localmodel.ProviderID, BaseURL: "http://127.0.0.1:11434/v1/chat/completions",
		Models: []string{"qwen2.5-coder:7b"}, DefaultModel: "qwen2.5-coder:7b",
	}}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	status, err := currentWorkModelStatus(store, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if status.Provider != localmodel.ProviderID || status.Model != "qwen2.5-coder:7b" || status.Access != "local model configured" {
		t.Fatalf("local model status = %#v", status)
	}
}

func TestCurrentWorkModelStatusResolvesImplicitCustomDefault(t *testing.T) {
	store, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings := config.Default()
	settings.Defaults.Provider = "custom-test"
	settings.Defaults.Model = ""
	settings.CustomProviders = []config.CustomProvider{{ID: "custom-test", BaseURL: "http://127.0.0.1:11434/v1/chat/completions", Models: []string{"configured-model"}, DefaultModel: "configured-model"}}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	status, err := currentWorkModelStatus(store, t.TempDir())
	if err != nil || status.Model != "configured-model" || status.Access != "custom endpoint configured (keyless)" {
		t.Fatalf("implicit model readiness = %#v, %v", status, err)
	}
}

func TestCurrentWorkModelStatusReportsStoredAPIKeyAuthentication(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	store, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings := config.Default()
	settings.Defaults = config.Defaults{Provider: "openai", Model: "gpt-5.6"}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	credentials, err := auth.New(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := credentials.Put("openai", auth.Credential{Type: "api_key", Key: "secret-api-key"}); err != nil {
		t.Fatal(err)
	}
	status, err := currentWorkModelStatus(store, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if status.Provider != "openai" || status.Model != "gpt-5.6" || status.Access != "authenticated with Gator api key" {
		t.Fatalf("provider status = %#v", status)
	}
	if status.Access == "secret-api-key" {
		t.Fatal("provider status exposed credential material")
	}
}

func TestCurrentWorkModelStatusReportsAmbientAndMissingCredentials(t *testing.T) {
	store, err := config.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings := config.Default()
	settings.Defaults = config.Defaults{Provider: "openai", Model: "gpt-5.6"}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}

	t.Setenv("OPENAI_API_KEY", "")
	status, err := currentWorkModelStatus(store, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if status.Access != "not authenticated; requires OPENAI_API_KEY" {
		t.Fatalf("missing credential status = %#v", status)
	}

	t.Setenv("OPENAI_API_KEY", "secret-key")
	status, err = currentWorkModelStatus(store, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if status.Access != "authenticated via OPENAI_API_KEY" {
		t.Fatalf("ambient credential status = %#v", status)
	}
}
