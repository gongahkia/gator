package model

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/model/anthropic"
	"github.com/gongahkia/gator/internal/model/chatcompletions"
	"github.com/gongahkia/gator/internal/model/gemini"
	"github.com/gongahkia/gator/internal/model/openai"
)

func TestNewBuildsCloudBackendsWithoutCrossProviderCredentials(t *testing.T) {
	tests := []struct {
		provider  Provider
		key       string
		baseURL   string
		model     string
		accountID string
		gatewayID string
		protocol  string
	}{
		{provider: OpenAI, key: "openai-key"},
		{provider: Anthropic, key: "anthropic-key"},
		{provider: Gemini, key: "gemini-key"},
		{provider: Mistral, key: "mistral-key"},
		{provider: AzureOpenAI, key: "azure-key", baseURL: "https://example.test/chat/completions?api-version=2025-01-01"},
		{provider: AzureOpenAIResponses, key: "azure-key", baseURL: "https://example.test"},
		{provider: OpenAICompatible, key: "compatible-key", baseURL: "https://example.test/v1/chat/completions"},
		{provider: KimiCoding, key: "kimi-key"},
		{provider: Radius, key: "radius-key"},
		{provider: Cerebras, key: "cerebras-key"},
		{provider: NVIDIA, key: "nvidia-key"},
		{provider: HuggingFace, key: "hf-key"},
		{provider: MoonshotAI, key: "moonshot-key"},
		{provider: ZAI, key: "zai-key"},
		{provider: ZAICodingCN, key: "zai-cn-key"},
		{provider: MiniMax, key: "minimax-key"},
		{provider: MiniMaxCN, key: "minimax-cn-key"},
		{provider: OpenCode, key: "opencode-key", model: "gpt-5.6-terra"},
		{provider: OpenCodeGo, key: "opencode-go-key", model: "kimi-k2.6"},
		{provider: Baseten, key: "baseten-key"},
		{provider: VercelAIGateway, key: "vercel-key"},
		{provider: AntLing, key: "ant-ling-key"},
		{provider: Xiaomi, key: "mimo-key"},
		{provider: MoonshotAICN, key: "moonshot-cn-key"},
		{provider: CloudflareWorkers, key: "cloudflare-key", baseURL: "https://example.test/ai/v1/chat/completions"},
		{provider: CloudflareGateway, key: "cloudflare-key", baseURL: "https://example.test/ai/v1", model: "openai/gpt-5.6", accountID: "account-123", gatewayID: "gateway-123", protocol: "openai-responses"},
		{provider: QwenTokenPlan, key: "qwen-token-plan-key"},
		{provider: QwenTokenPlanCN, key: "qwen-token-plan-cn-key"},
		{provider: QwenTokenPlanIndividual, key: "qwen-token-plan-individual-key"},
		{provider: XiaomiTokenPlanCN, key: "mimo-token-plan-cn-key"},
		{provider: XiaomiTokenPlanAMS, key: "mimo-token-plan-ams-key"},
		{provider: XiaomiTokenPlanSGP, key: "mimo-token-plan-sgp-key"},
	}
	for _, test := range tests {
		t.Run(string(test.provider), func(t *testing.T) {
			model := test.model
			if model == "" {
				model = "test-model"
			}
			backend, err := New(Config{Provider: test.provider, APIKey: test.key, Model: model, BaseURL: test.baseURL, Client: http.DefaultClient, CloudflareAccountID: test.accountID, CloudflareGatewayID: test.gatewayID, CloudflareGatewayProtocol: test.protocol})
			if err != nil {
				t.Fatalf("new backend: %v", err)
			}
			if backend.Provider != test.provider || backend.Model == nil {
				t.Fatalf("backend = %#v", backend)
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
	for _, provider := range []Provider{AzureOpenAIResponses, MiniMax, MiniMaxCN} {
		if _, err := New(Config{Provider: provider, APIKey: "test-key"}); err == nil || !strings.Contains(err.Error(), "--model") {
			t.Fatalf("provider %q missing-model error = %v", provider, err)
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

func TestAzureResponsesUsesRawAPIKeyHeaderAndV1Endpoint(t *testing.T) {
	backend, err := New(Config{Provider: AzureOpenAIResponses, APIKey: "azure-key", Model: "deployment", BaseURL: "https://example.test/openai/v1"})
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	adapter, ok := backend.Model.(openai.Responses)
	if !ok || adapter.AuthorizationHeader != "api-key" || adapter.AuthorizationPrefix != "" || adapter.BaseURL != "https://example.test/openai/v1/responses?api-version=v1" {
		t.Fatalf("adapter = %#v", backend.Model)
	}
}

func TestAzureResponsesUsesOnlyAPIKeyAuthentication(t *testing.T) {
	t.Setenv("AZURE_OPENAI_API_KEY", "")
	t.Setenv("AZURE_OPENAI_AUTH_TOKEN", "entra-token")
	backend, err := New(Config{Provider: AzureOpenAIResponses, APIKey: "azure-key", Model: "deployment", BaseURL: "https://example.test"})
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	adapter, ok := backend.Model.(openai.Responses)
	if !ok || adapter.AuthorizationHeader != "api-key" || adapter.AuthorizationPrefix != "" || adapter.APIKey != "azure-key" {
		t.Fatalf("adapter = %#v", backend.Model)
	}
}

func TestAzureResponsesPrefersAmbientAPIKeyOverAmbientEntraToken(t *testing.T) {
	t.Setenv("AZURE_OPENAI_API_KEY", "azure-key")
	t.Setenv("AZURE_OPENAI_AUTH_TOKEN", "entra-token")
	backend, err := New(Config{Provider: AzureOpenAIResponses, Model: "deployment", BaseURL: "https://example.test"})
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	adapter, ok := backend.Model.(openai.Responses)
	if !ok || adapter.AuthorizationHeader != "api-key" || adapter.APIKey != "azure-key" {
		t.Fatalf("adapter = %#v", backend.Model)
	}
}

func TestAzureResponsesEndpointConfiguration(t *testing.T) {
	t.Setenv("AZURE_OPENAI_API_VERSION", "preview")
	for _, test := range []struct {
		name string
		base string
		want string
	}{
		{name: "resource root", base: "https://resource.openai.azure.com", want: "https://resource.openai.azure.com/openai/v1/responses?api-version=preview"},
		{name: "full endpoint preserves version", base: "https://resource.openai.azure.com/openai/v1/responses?api-version=custom", want: "https://resource.openai.azure.com/openai/v1/responses?api-version=custom"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := azureResponsesURL(test.base)
			if err != nil || got != test.want {
				t.Fatalf("azureResponsesURL(%q) = %q, %v", test.base, got, err)
			}
		})
	}
	t.Setenv("AZURE_OPENAI_BASE_URL", "")
	t.Setenv("AZURE_OPENAI_RESOURCE_NAME", "resource")
	got, err := azureResponsesURL("")
	if err != nil || got != "https://resource.openai.azure.com/openai/v1/responses?api-version=preview" {
		t.Fatalf("resource endpoint = %q, %v", got, err)
	}
}

func TestBasetenUsesDocumentedAPIKeyAuthorizationPrefix(t *testing.T) {
	backend, err := New(Config{Provider: Baseten, APIKey: "baseten-key", Model: "model"})
	if err != nil {
		t.Fatalf("new Baseten backend: %v", err)
	}
	adapter, ok := backend.Model.(chatcompletions.Model)
	if !ok || adapter.Config.AuthorizationHeader != "" || adapter.Config.AuthorizationPrefix != "Api-Key " {
		t.Fatalf("Baseten adapter = %#v", backend.Model)
	}
}

func TestZAICodingPlanUsesDedicatedEndpoint(t *testing.T) {
	backend, err := New(Config{Provider: ZAI, APIKey: "zai-key", Model: "glm-5.1"})
	if err != nil {
		t.Fatalf("new Z.AI backend: %v", err)
	}
	adapter, ok := backend.Model.(chatcompletions.Model)
	if !ok || adapter.Config.BaseURL != "https://api.z.ai/api/coding/paas/v4/chat/completions" {
		t.Fatalf("Z.AI adapter = %#v", backend.Model)
	}
}

func TestZAICodingPlanChinaUsesDedicatedEndpoint(t *testing.T) {
	backend, err := New(Config{Provider: ZAICodingCN, APIKey: "zai-cn-key", Model: "glm-5.1"})
	if err != nil {
		t.Fatalf("new Z.AI China backend: %v", err)
	}
	adapter, ok := backend.Model.(chatcompletions.Model)
	if !ok || adapter.Config.BaseURL != "https://open.bigmodel.cn/api/coding/paas/v4/chat/completions" || adapter.Config.APIKeyEnv != "ZAI_CODING_CN_API_KEY" {
		t.Fatalf("Z.AI China adapter = %#v", backend.Model)
	}
}

func TestMiniMaxUsesAnthropicMessagesEndpoints(t *testing.T) {
	for _, test := range []struct {
		provider Provider
		key      string
		endpoint string
	}{
		{provider: MiniMax, key: "minimax-key", endpoint: "https://api.minimax.io/anthropic/v1/messages"},
		{provider: MiniMaxCN, key: "minimax-cn-key", endpoint: "https://api.minimaxi.com/anthropic/v1/messages"},
	} {
		t.Run(string(test.provider), func(t *testing.T) {
			backend, err := New(Config{Provider: test.provider, APIKey: test.key, Model: "MiniMax-M2.7"})
			if err != nil {
				t.Fatalf("new backend: %v", err)
			}
			adapter, ok := backend.Model.(anthropic.Messages)
			if !ok || adapter.BaseURL != test.endpoint {
				t.Fatalf("MiniMax adapter = %#v", backend.Model)
			}
		})
	}
}

func TestOpenCodeCatalogRoutesPublishedModelsToTheirProtocols(t *testing.T) {
	for _, test := range []struct {
		provider Provider
		model    string
		assert   func(*testing.T, Backend)
	}{
		{provider: OpenCode, model: "gpt-5.6-terra", assert: func(t *testing.T, backend Backend) {
			adapter, ok := backend.Model.(openai.Responses)
			if !ok || adapter.BaseURL != "https://opencode.ai/zen/v1/responses" {
				t.Fatalf("Responses adapter = %#v", backend.Model)
			}
		}},
		{provider: OpenCode, model: "claude-sonnet-5", assert: func(t *testing.T, backend Backend) {
			adapter, ok := backend.Model.(anthropic.Messages)
			if !ok || adapter.BaseURL != "https://opencode.ai/zen/v1/messages" {
				t.Fatalf("Messages adapter = %#v", backend.Model)
			}
		}},
		{provider: OpenCode, model: "gemini-3.5-flash", assert: func(t *testing.T, backend Backend) {
			adapter, ok := backend.Model.(gemini.GenerateContent)
			if !ok || adapter.BaseURL != "https://opencode.ai/zen/v1" {
				t.Fatalf("GenerateContent adapter = %#v", backend.Model)
			}
		}},
		{provider: OpenCode, model: "kimi-k2.6", assert: func(t *testing.T, backend Backend) {
			adapter, ok := backend.Model.(chatcompletions.Model)
			if !ok || adapter.Config.BaseURL != "https://opencode.ai/zen/v1/chat/completions" {
				t.Fatalf("Chat Completions adapter = %#v", backend.Model)
			}
		}},
		{provider: OpenCodeGo, model: "grok-4.5", assert: func(t *testing.T, backend Backend) {
			adapter, ok := backend.Model.(openai.Responses)
			if !ok || adapter.BaseURL != "https://opencode.ai/zen/go/v1/responses" {
				t.Fatalf("Responses adapter = %#v", backend.Model)
			}
		}},
		{provider: OpenCodeGo, model: "minimax-m3", assert: func(t *testing.T, backend Backend) {
			adapter, ok := backend.Model.(anthropic.Messages)
			if !ok || adapter.BaseURL != "https://opencode.ai/zen/go/v1/messages" {
				t.Fatalf("Messages adapter = %#v", backend.Model)
			}
		}},
		{provider: OpenCodeGo, model: "kimi-k2.6", assert: func(t *testing.T, backend Backend) {
			adapter, ok := backend.Model.(chatcompletions.Model)
			if !ok || adapter.Config.BaseURL != "https://opencode.ai/zen/go/v1/chat/completions" {
				t.Fatalf("Chat Completions adapter = %#v", backend.Model)
			}
		}},
	} {
		t.Run(string(test.provider)+"/"+test.model, func(t *testing.T) {
			backend, err := New(Config{Provider: test.provider, APIKey: "opencode-key", Model: test.model})
			if err != nil {
				t.Fatalf("new backend: %v", err)
			}
			test.assert(t, backend)
		})
	}
}

func TestOpenCodeCatalogRejectsUnknownModels(t *testing.T) {
	for _, provider := range []Provider{OpenCode, OpenCodeGo} {
		t.Run(string(provider), func(t *testing.T) {
			_, err := New(Config{Provider: provider, APIKey: "opencode-key", Model: "not-a-published-model"})
			if err == nil || !strings.Contains(err.Error(), "embedded") || !strings.Contains(err.Error(), "not-a-published-model") {
				t.Fatalf("unknown-model error = %v", err)
			}
		})
	}
}

func TestOpenCodeCatalogHasOnlySupportedProtocols(t *testing.T) {
	for provider, entries := range openCodeCatalog {
		if len(entries) == 0 {
			t.Fatalf("%s has an empty catalog", provider)
		}
		for model, protocol := range entries {
			if model == "" || protocol < openCodeChatCompletions || protocol > openCodeGemini {
				t.Fatalf("invalid catalog entry %s/%s = %d", provider, model, protocol)
			}
		}
	}
}

func TestCuratedModelsExposeTheCheckedInOpenCodeCatalog(t *testing.T) {
	for _, provider := range []Provider{OpenCode, OpenCodeGo} {
		models := CuratedModels(provider)
		if len(models) != len(openCodeCatalog[provider]) || models[0] != DefaultModel(provider) {
			t.Fatalf("curated models for %s = %#v", provider, models)
		}
		seen := make(map[string]bool, len(models))
		for _, model := range models {
			if seen[model] {
				t.Fatalf("duplicate curated model %q for %s", model, provider)
			}
			seen[model] = true
			if _, found := openCodeModelProtocol(provider, model); !found {
				t.Fatalf("curated model %q is not routed for %s", model, provider)
			}
		}
	}
	if got := CuratedModels(OpenAI); len(got) != 1 || got[0] != DefaultModel(OpenAI) {
		t.Fatalf("OpenAI curated models = %#v", got)
	}
}

func TestEveryOpenCodeCatalogEntryBuilds(t *testing.T) {
	for provider, entries := range openCodeCatalog {
		for model := range entries {
			t.Run(string(provider)+"/"+model, func(t *testing.T) {
				backend, err := New(Config{Provider: provider, APIKey: "opencode-key", Model: model})
				if err != nil || backend.Model == nil {
					t.Fatalf("new backend = %#v, %v", backend, err)
				}
			})
		}
	}
}

func TestCloudflareUsesAccountScopedOpenAIEndpoint(t *testing.T) {
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "account_123")
	backend, err := New(Config{Provider: CloudflareWorkers, APIKey: "cloudflare-key", Model: "@cf/openai/gpt-oss-20b"})
	if err != nil {
		t.Fatalf("new Cloudflare backend: %v", err)
	}
	adapter, ok := backend.Model.(chatcompletions.Model)
	if !ok || adapter.Config.BaseURL != "https://api.cloudflare.com/client/v4/accounts/account_123/ai/v1/chat/completions" {
		t.Fatalf("Cloudflare adapter = %#v", backend.Model)
	}
}

func TestCloudflareRequiresAccountIDWithoutEndpointOverride(t *testing.T) {
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "")
	if _, err := New(Config{Provider: CloudflareGateway, APIKey: "cloudflare-key", Model: "openai/gpt-5.2"}); err == nil || !strings.Contains(err.Error(), "CLOUDFLARE_ACCOUNT_ID") {
		t.Fatalf("missing Cloudflare account ID error = %v", err)
	}
}

func TestProviderOptionsConfigureCloudflareGateway(t *testing.T) {
	backend, err := New(Config{Provider: CloudflareGateway, APIKey: "cloudflare-key", Model: "openai/gpt-5.6", ProviderOptions: map[string]string{"account_id": "account-123", "gateway_id": "gateway-123", "gateway_protocol": "openai-responses"}})
	if err != nil {
		t.Fatalf("new Cloudflare Gateway backend: %v", err)
	}
	adapter, ok := backend.Model.(openai.Responses)
	if !ok || adapter.Headers.Get("Cf-Aig-Gateway-Id") != "gateway-123" || !strings.Contains(adapter.BaseURL, "/accounts/account-123/") {
		t.Fatalf("Cloudflare Gateway adapter = %#v", backend.Model)
	}
}

func TestEffectiveModelUsesProviderDefaults(t *testing.T) {
	if got := EffectiveModel(Anthropic, ""); got != "claude-sonnet-5" {
		t.Fatalf("Anthropic default = %q", got)
	}
	if got := EffectiveModel(Gemini, "  chosen-model  "); got != "chosen-model" {
		t.Fatalf("explicit model = %q", got)
	}
	if got := EffectiveModel(KimiCoding, ""); got != "kimi-for-coding" {
		t.Fatalf("Kimi default = %q", got)
	}
	if got := EffectiveModel(OpenCode, ""); got != "gpt-5.6-terra" {
		t.Fatalf("OpenCode default = %q", got)
	}
	if got := EffectiveModel(OpenCodeGo, ""); got != "kimi-k2.6" {
		t.Fatalf("OpenCode Go default = %q", got)
	}
	for _, unavailable := range []string{"codex", "claude", "copilot", "cursor", "amazon-bedrock", "google-vertex"} {
		if _, err := ParseProvider(unavailable); err == nil {
			t.Fatalf("removed provider %q is still accepted", unavailable)
		}
	}
}
