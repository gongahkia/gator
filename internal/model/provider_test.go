package model

import (
	"net/http"
	"testing"

	"github.com/gongahkia/gator/internal/model/chatcompletions"
)

func TestNewBuildsCloudBackendsWithoutCrossProviderCredentials(t *testing.T) {
	tests := []struct {
		provider Provider
		key      string
		baseURL  string
	}{
		{provider: OpenAI, key: "openai-key"},
		{provider: Anthropic, key: "anthropic-key"},
		{provider: Gemini, key: "gemini-key"},
		{provider: Mistral, key: "mistral-key"},
		{provider: AzureOpenAI, key: "azure-key", baseURL: "https://example.test/chat/completions?api-version=2025-01-01"},
		{provider: OpenAICompatible, key: "compatible-key", baseURL: "https://example.test/v1/chat/completions"},
	}
	for _, test := range tests {
		t.Run(string(test.provider), func(t *testing.T) {
			backend, err := New(Config{Provider: test.provider, APIKey: test.key, Model: "test-model", BaseURL: test.baseURL, Client: http.DefaultClient})
			if err != nil {
				t.Fatalf("new backend: %v", err)
			}
			if backend.Provider != test.provider || backend.Model == nil || backend.Harness != nil {
				t.Fatalf("backend = %#v", backend)
			}
		})
	}
}

func TestNewRequiresConfiguredKeyAndCompatibleEndpoint(t *testing.T) {
	if _, err := New(Config{Provider: Anthropic, Model: "test"}); err == nil {
		t.Fatal("missing Anthropic key was accepted")
	}
	if _, err := New(Config{Provider: OpenAICompatible, APIKey: "test", Model: "test"}); err == nil {
		t.Fatal("missing compatible endpoint was accepted")
	}
	if _, err := ParseProvider("unknown"); err == nil {
		t.Fatal("unknown provider was accepted")
	}
}

func TestAzureUsesRawAPIKeyHeader(t *testing.T) {
	backend, err := New(Config{Provider: AzureOpenAI, APIKey: "azure-key", Model: "deployment", BaseURL: "https://example.test/chat/completions"})
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	adapter, ok := backend.Model.(chatcompletions.Model)
	if !ok || adapter.Config.AuthorizationHeader != "api-key" || adapter.Config.AuthorizationPrefix != "" {
		t.Fatalf("adapter = %#v", backend.Model)
	}
}

func TestEffectiveModelUsesProviderDefaults(t *testing.T) {
	if got := EffectiveModel(Anthropic, ""); got != "claude-sonnet-5" {
		t.Fatalf("Anthropic default = %q", got)
	}
	if got := EffectiveModel(Gemini, "  chosen-model  "); got != "chosen-model" {
		t.Fatalf("explicit model = %q", got)
	}
	if got := EffectiveModel(Codex, ""); got != "" {
		t.Fatalf("CLI default = %q", got)
	}
}
