package model

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/model/anthropic"
	"github.com/gongahkia/gator/internal/model/chatcompletions"
	"github.com/gongahkia/gator/internal/model/openai"
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
			if backend.Provider != test.provider || backend.Model == nil {
				t.Fatalf("backend = %#v", backend)
			}
		})
	}
}

func TestNewBuildsSubscriptionAdaptersFromGatorOAuthCredentials(t *testing.T) {
	for _, test := range []struct {
		provider Provider
		extra    map[string]string
	}{
		{provider: Codex, extra: map[string]string{"chatgpt_account_id": "account_123"}},
		{provider: Claude},
	} {
		t.Run(string(test.provider), func(t *testing.T) {
			store, err := auth.New(t.TempDir())
			if err != nil {
				t.Fatalf("new credentials: %v", err)
			}
			credential := auth.Credential{Type: "oauth", Access: "access-token", Refresh: "refresh-token", Expires: time.Now().Add(time.Hour).UnixMilli(), Extra: test.extra}
			if err := store.Put(string(test.provider), credential); err != nil {
				t.Fatalf("store credential: %v", err)
			}
			backend, err := New(Config{Provider: test.provider, Model: "test-model", Credentials: &store})
			if err != nil {
				t.Fatalf("new subscription backend: %v", err)
			}
			switch adapter := backend.Model.(type) {
			case openai.Responses:
				if adapter.APIKey != "access-token" || adapter.BaseURL != "https://chatgpt.com/backend-api/codex/responses" || adapter.Headers.Get("Chatgpt-Account-Id") != "account_123" {
					t.Fatalf("Codex adapter = %#v", adapter)
				}
			case anthropic.Messages:
				if adapter.APIKey != "access-token" {
					t.Fatalf("Claude adapter = %#v", adapter)
				}
			default:
				t.Fatalf("subscription adapter = %T", backend.Model)
			}
		})
	}
}

func TestNewUsesProviderScopedStoredCredentialBeforeEnvironment(t *testing.T) {
	store, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatalf("new credentials: %v", err)
	}
	if err := store.Put("openai", auth.Credential{Type: "api_key", Key: "stored-openai-key"}); err != nil {
		t.Fatalf("store OpenAI credential: %v", err)
	}
	if err := store.Put("anthropic", auth.Credential{Type: "api_key", Key: "stored-anthropic-key"}); err != nil {
		t.Fatalf("store Anthropic credential: %v", err)
	}
	backend, err := New(Config{Provider: OpenAI, Model: "test-model", Credentials: &store})
	if err != nil {
		t.Fatalf("new OpenAI backend: %v", err)
	}
	adapter, ok := backend.Model.(openai.Responses)
	if !ok || adapter.APIKey != "stored-openai-key" {
		t.Fatalf("OpenAI adapter = %#v", backend.Model)
	}
	if _, err := New(Config{Provider: Gemini, Model: "test-model", Credentials: &store}); err == nil || !strings.Contains(err.Error(), "GEMINI_API_KEY") {
		t.Fatalf("Gemini unexpectedly used another provider's credential: %v", err)
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
	if _, err := New(Config{Provider: Codex}); err == nil || !strings.Contains(err.Error(), "OAuth credential") {
		t.Fatalf("missing Codex OAuth credential error = %v", err)
	}
	for _, provider := range []Provider{Copilot, Cursor} {
		if _, err := New(Config{Provider: provider}); err == nil || !strings.Contains(err.Error(), "will not launch") {
			t.Fatalf("provider %q error = %v", provider, err)
		}
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
	if got := EffectiveModel(Codex, ""); got != DefaultModel(OpenAI) {
		t.Fatalf("Codex default = %q", got)
	}
	if got := EffectiveModel(Claude, ""); got != DefaultModel(Anthropic) {
		t.Fatalf("Claude default = %q", got)
	}
	for _, unavailable := range []string{"copilot", "cursor"} {
		provider, err := ParseProvider(unavailable)
		if err != nil || SupportsDirect(provider) {
			t.Fatalf("direct support for %q = %v, parse error = %v", unavailable, SupportsDirect(provider), err)
		}
		for _, name := range Names() {
			if name == unavailable {
				t.Fatalf("unavailable provider %q appeared in direct provider list", unavailable)
			}
		}
	}
}
