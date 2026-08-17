package model

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/model/anthropic"
	"github.com/gongahkia/gator/internal/model/chatcompletions"
	"github.com/gongahkia/gator/internal/model/gemini"
	"github.com/gongahkia/gator/internal/model/openai"
	"github.com/gongahkia/gator/internal/model/radius"
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
		{provider: OpenCode, key: "opencode-key"},
		{provider: OpenCodeGo, key: "opencode-go-key"},
		{provider: Baseten, key: "baseten-key"},
		{provider: VercelAIGateway, key: "vercel-key"},
		{provider: AntLing, key: "ant-ling-key"},
		{provider: Xiaomi, key: "mimo-key"},
		{provider: MoonshotAICN, key: "moonshot-cn-key"},
		{provider: CloudflareWorkers, key: "cloudflare-key", baseURL: "https://example.test/ai/v1/chat/completions"},
		{provider: CloudflareGateway, key: "cloudflare-key", baseURL: "https://example.test/ai/v1/chat/completions"},
		{provider: AmazonBedrock, key: "bedrock-key"},
		{provider: QwenTokenPlan, key: "qwen-token-plan-key"},
		{provider: QwenTokenPlanCN, key: "qwen-token-plan-cn-key"},
		{provider: QwenTokenPlanIndividual, key: "qwen-token-plan-individual-key"},
		{provider: XiaomiTokenPlanCN, key: "mimo-token-plan-cn-key"},
		{provider: XiaomiTokenPlanAMS, key: "mimo-token-plan-ams-key"},
		{provider: XiaomiTokenPlanSGP, key: "mimo-token-plan-sgp-key"},
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
		{provider: Copilot, extra: map[string]string{"base_url": "https://api.example.test"}},
		{provider: KimiCoding},
		{provider: Radius},
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
					t.Fatalf("Anthropic-compatible subscription adapter = %#v", adapter)
				}
				if test.provider == Claude && (!adapter.BearerAuth || adapter.Headers.Get("Anthropic-Beta") != "oauth-2025-04-20") {
					t.Fatalf("Claude OAuth adapter = %#v", adapter)
				}
				if test.provider == KimiCoding && (!adapter.BearerAuth || adapter.BaseURL != "https://api.kimi.com/coding/v1/messages") {
					t.Fatalf("Kimi OAuth adapter = %#v", adapter)
				}
			case chatcompletions.Model:
				if adapter.Config.APIKey != "access-token" || adapter.Config.BaseURL != "https://api.example.test/chat/completions" || adapter.Config.RequestHeaders == nil {
					t.Fatalf("Copilot adapter = %#v", adapter)
				}
			case radius.Messages:
				if test.provider != Radius || adapter.APIKey != "access-token" || adapter.Model != "test-model" {
					t.Fatalf("Radius adapter = %#v", adapter)
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

func TestNewUsesStoredXAISubscriptionCredential(t *testing.T) {
	store, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatalf("new credentials: %v", err)
	}
	if err := store.Put("xai", auth.Credential{Type: "oauth", Access: "xai-access", Refresh: "xai-refresh", Expires: time.Now().Add(time.Hour).UnixMilli()}); err != nil {
		t.Fatalf("store xAI credential: %v", err)
	}
	backend, err := New(Config{Provider: XAI, Model: "grok-test", Credentials: &store})
	if err != nil {
		t.Fatalf("new xAI backend: %v", err)
	}
	adapter, ok := backend.Model.(chatcompletions.Model)
	if !ok || adapter.Config.APIKey != "xai-access" {
		t.Fatalf("xAI adapter = %#v", backend.Model)
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
	for _, provider := range []Provider{Cursor} {
		if _, err := New(Config{Provider: provider}); err == nil || !strings.Contains(err.Error(), "will not launch") {
			t.Fatalf("provider %q error = %v", provider, err)
		}
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

func TestAzureResponsesAuthenticationModesAreMutuallyExclusive(t *testing.T) {
	for _, test := range []struct {
		name              string
		apiKey            string
		environmentToken  string
		storedBearerToken string
		wantHeader        string
		wantValue         string
	}{
		{name: "explicit API key", apiKey: "azure-key", environmentToken: "entra-token", wantHeader: "api-key", wantValue: "azure-key"},
		{name: "ambient Entra token", environmentToken: "entra-token", wantHeader: "Authorization", wantValue: "Bearer entra-token"},
		{name: "stored Entra token", storedBearerToken: "stored-token", wantHeader: "Authorization", wantValue: "Bearer stored-token"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("AZURE_OPENAI_API_KEY", "")
			t.Setenv("AZURE_OPENAI_AUTH_TOKEN", test.environmentToken)
			config := Config{Provider: AzureOpenAIResponses, APIKey: test.apiKey, Model: "deployment", BaseURL: "https://example.test"}
			if test.storedBearerToken != "" {
				store, err := auth.New(t.TempDir())
				if err != nil {
					t.Fatalf("new credentials: %v", err)
				}
				if err := store.Put(string(AzureOpenAIResponses), auth.Credential{Type: "bearer_token", Access: test.storedBearerToken}); err != nil {
					t.Fatalf("store bearer token: %v", err)
				}
				config.Credentials = &store
			}
			backend, err := New(config)
			if err != nil {
				t.Fatalf("new backend: %v", err)
			}
			adapter, ok := backend.Model.(openai.Responses)
			if !ok {
				t.Fatalf("adapter = %T", backend.Model)
			}
			if got := adapter.AuthorizationHeader; got != test.wantHeader || adapter.AuthorizationPrefix+adapter.APIKey != test.wantValue {
				t.Fatalf("authentication = %q %q", got, adapter.AuthorizationPrefix+adapter.APIKey)
			}
			if test.wantHeader == "api-key" && adapter.AuthorizationPrefix != "" {
				t.Fatalf("API-key adapter set an authorization prefix %q", adapter.AuthorizationPrefix)
			}
			if test.wantHeader == "Authorization" && adapter.AuthorizationHeader == "api-key" {
				t.Fatal("bearer adapter also configured api-key authentication")
			}
		})
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

func TestOpenCodeRoutesModelFamiliesToTheirPublishedProtocols(t *testing.T) {
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

func TestBedrockUsesRegionScopedOpenAIEndpoint(t *testing.T) {
	t.Setenv("AWS_REGION", "us-west-2")
	backend, err := New(Config{Provider: AmazonBedrock, APIKey: "bedrock-key", Model: "openai.gpt-oss-20b-1:0"})
	if err != nil {
		t.Fatalf("new Bedrock backend: %v", err)
	}
	adapter, ok := backend.Model.(chatcompletions.Model)
	if !ok || adapter.Config.BaseURL != "https://bedrock-mantle.us-west-2.api.aws/v1/chat/completions" {
		t.Fatalf("Bedrock adapter = %#v", backend.Model)
	}
}

func TestVertexUsesADCBackedOpenAIEndpoint(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "project-123")
	t.Setenv("GOOGLE_CLOUD_LOCATION", "us-central1")
	t.Setenv("GATOR_VERTEX_ACCESS_TOKEN", "vertex-access-token")
	backend, err := New(Config{Provider: GoogleVertex, Model: "google/gemini-2.0-flash-001"})
	if err != nil {
		t.Fatalf("new Vertex backend: %v", err)
	}
	adapter, ok := backend.Model.(chatcompletions.Model)
	if !ok || adapter.Config.APIKeySource == nil || adapter.Config.BaseURL != "https://us-central1-aiplatform.googleapis.com/v1/projects/project-123/locations/us-central1/endpoints/openapi/chat/completions" {
		t.Fatalf("Vertex adapter = %#v", backend.Model)
	}
}

func TestVertexRequiresProjectAndLocation(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("GCLOUD_PROJECT", "")
	t.Setenv("GOOGLE_CLOUD_LOCATION", "")
	if _, err := New(Config{Provider: GoogleVertex, Model: "google/gemini-2.0-flash-001"}); err == nil || !strings.Contains(err.Error(), "GOOGLE_CLOUD_PROJECT") {
		t.Fatalf("missing Vertex configuration error = %v", err)
	}
}

func TestCloudflareRequiresAccountIDWithoutEndpointOverride(t *testing.T) {
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "")
	if _, err := New(Config{Provider: CloudflareGateway, APIKey: "cloudflare-key", Model: "openai/gpt-5.2"}); err == nil || !strings.Contains(err.Error(), "CLOUDFLARE_ACCOUNT_ID") {
		t.Fatalf("missing Cloudflare account ID error = %v", err)
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
	if got := EffectiveModel(KimiCoding, ""); got != "kimi-for-coding" {
		t.Fatalf("Kimi default = %q", got)
	}
	if got := EffectiveModel(OpenCode, ""); got != "gpt-5.6-terra" {
		t.Fatalf("OpenCode default = %q", got)
	}
	if got := EffectiveModel(OpenCodeGo, ""); got != "kimi-k2.6" {
		t.Fatalf("OpenCode Go default = %q", got)
	}
	for _, unavailable := range []string{"cursor"} {
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
