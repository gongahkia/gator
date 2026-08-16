// Package model resolves supported direct cloud APIs into provider-independent
// execution backends. Gator always retains the model and tool loop.
package model

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/model/anthropic"
	"github.com/gongahkia/gator/internal/model/chatcompletions"
	"github.com/gongahkia/gator/internal/model/gemini"
	"github.com/gongahkia/gator/internal/model/openai"
	"github.com/gongahkia/gator/internal/model/radius"
)

// Provider identifies a direct model backend or a retained legacy session
// provider that now fails without a fallback.
type Provider string

const (
	OpenAI           Provider = "openai"
	AzureOpenAI      Provider = "azure-openai"
	Anthropic        Provider = "anthropic"
	Gemini           Provider = "gemini"
	Mistral          Provider = "mistral"
	XAI              Provider = "xai"
	Groq             Provider = "groq"
	OpenRouter       Provider = "openrouter"
	Together         Provider = "together"
	Fireworks        Provider = "fireworks"
	DeepSeek         Provider = "deepseek"
	Cerebras         Provider = "cerebras"
	NVIDIA           Provider = "nvidia"
	HuggingFace      Provider = "huggingface"
	MoonshotAI       Provider = "moonshotai"
	OpenAICompatible Provider = "openai-compatible"
	Codex            Provider = "codex"
	Claude           Provider = "claude"
	Copilot          Provider = "copilot"
	KimiCoding       Provider = "kimi-coding"
	Radius           Provider = "radius"
	Cursor           Provider = "cursor"
)

// Config selects one provider. APIKey is optional only because the factory can
// obtain the provider's documented environment variable when it is unset.
type Config struct {
	Provider    Provider
	Model       string
	BaseURL     string
	APIKey      string
	Credentials *auth.Store
	Client      *http.Client
}

// Backend contains a direct model adapter. The agent and tool loop remain in
// Gator for every supported provider.
type Backend struct {
	Provider Provider
	Model    agent.Model
}

// New resolves one configured direct provider. It never launches a vendor CLI
// or reads another application's credential store.
func New(config Config) (Backend, error) {
	provider, err := ParseProvider(string(config.Provider))
	if err != nil {
		return Backend{}, err
	}
	config.Provider = provider
	if strings.TrimSpace(config.Model) == "" {
		config.Model = DefaultModel(provider)
	}
	switch provider {
	case OpenAI:
		apiKey, err := key(config, provider, "OPENAI_API_KEY")
		if err != nil {
			return Backend{}, err
		}
		if err := requireKey(apiKey, "OPENAI_API_KEY"); err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: openai.Responses{APIKey: apiKey, Model: config.Model, BaseURL: config.BaseURL, Client: config.Client}}, nil
	case Codex:
		credential, err := oauthCredential(config, provider)
		if err != nil {
			return Backend{}, err
		}
		accountID, err := codexAccountID(credential)
		if err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: openai.Responses{
			APIKey:  credential.Access,
			Model:   config.Model,
			BaseURL: codexResponsesURL(config.BaseURL),
			Headers: http.Header{
				"Chatgpt-Account-Id": []string{accountID},
				"Openai-Beta":        []string{"responses=experimental"},
				"Originator":         []string{"gator"},
			},
			Client: config.Client,
		}}, nil
	case Anthropic:
		apiKey, err := key(config, provider, "ANTHROPIC_API_KEY")
		if err != nil {
			return Backend{}, err
		}
		if err := requireKey(apiKey, "ANTHROPIC_API_KEY"); err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: anthropic.Messages{APIKey: apiKey, Model: config.Model, BaseURL: config.BaseURL, Client: config.Client}}, nil
	case Claude:
		credential, err := oauthCredential(config, provider)
		if err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: anthropic.Messages{
			APIKey:     credential.Access,
			Model:      config.Model,
			BaseURL:    config.BaseURL,
			BearerAuth: true,
			Headers: http.Header{
				"Anthropic-Dangerous-Direct-Browser-Access": []string{"true"},
				"Anthropic-Beta": []string{"oauth-2025-04-20"},
			},
			Client: config.Client,
		}}, nil
	case Copilot:
		credential, err := oauthCredential(config, provider)
		if err != nil {
			return Backend{}, err
		}
		if strings.TrimSpace(config.Model) == "" {
			return Backend{}, errors.New("--model is required for provider \"copilot\"; choose an enabled account model")
		}
		return Backend{Provider: provider, Model: chatcompletions.Model{Config: chatcompletions.Config{
			APIKey:       credential.Access,
			APIKeyEnv:    "Gator Copilot OAuth credential",
			BaseURL:      copilotChatURL(config.BaseURL, credential.Extra["base_url"]),
			Model:        config.Model,
			ProviderName: "GitHub Copilot",
			Headers: http.Header{
				"User-Agent":             []string{"GitHubCopilotChat/0.35.0"},
				"Editor-Version":         []string{"vscode/1.107.0"},
				"Editor-Plugin-Version":  []string{"copilot-chat/0.35.0"},
				"Copilot-Integration-Id": []string{"vscode-chat"},
			},
			RequestHeaders: copilotRequestHeaders,
			Client:         config.Client,
		}}}, nil
	case KimiCoding:
		apiKey, bearer, err := kimiCredential(config)
		if err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: anthropic.Messages{
			APIKey:     apiKey,
			Model:      config.Model,
			BaseURL:    kimiMessagesURL(config.BaseURL),
			BearerAuth: bearer,
			Client:     config.Client,
		}}, nil
	case Radius:
		apiKey, err := key(config, provider, "RADIUS_API_KEY")
		if err != nil {
			return Backend{}, err
		}
		if err := requireKey(apiKey, "RADIUS_API_KEY"); err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: radius.Messages{
			APIKey:  apiKey,
			Model:   config.Model,
			BaseURL: config.BaseURL,
			Gateway: os.Getenv("GATOR_RADIUS_GATEWAY"),
			Client:  config.Client,
		}}, nil
	case Gemini:
		apiKey, err := key(config, provider, "GEMINI_API_KEY")
		if err != nil {
			return Backend{}, err
		}
		if err := requireKey(apiKey, "GEMINI_API_KEY"); err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: gemini.GenerateContent{APIKey: apiKey, Model: config.Model, BaseURL: config.BaseURL, Client: config.Client}}, nil
	case AzureOpenAI, Mistral, XAI, Groq, OpenRouter, Together, Fireworks, DeepSeek, Cerebras, NVIDIA, HuggingFace, MoonshotAI, OpenAICompatible:
		compatible, err := compatibleConfig(provider, config)
		if err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: chatcompletions.Model{Config: compatible}}, nil
	case Cursor:
		return Backend{}, fmt.Errorf("provider %q has no supported direct model API integration; Gator will not launch the %s CLI", provider, provider)
	default:
		return Backend{}, fmt.Errorf("unsupported provider %q", provider)
	}
}

func compatibleConfig(provider Provider, config Config) (chatcompletions.Config, error) {
	definition, ok := compatibleProviders[provider]
	if !ok {
		return chatcompletions.Config{}, fmt.Errorf("provider %q is not OpenAI-compatible", provider)
	}
	apiKey, err := key(config, provider, definition.apiKeyEnv)
	if err != nil {
		return chatcompletions.Config{}, err
	}
	if err := requireKey(apiKey, definition.apiKeyEnv); err != nil {
		return chatcompletions.Config{}, err
	}
	baseURL := config.BaseURL
	if strings.TrimSpace(baseURL) == "" {
		baseURL = definition.baseURL
	}
	if strings.TrimSpace(config.Model) == "" {
		return chatcompletions.Config{}, fmt.Errorf("--model is required for provider %q", provider)
	}
	if strings.TrimSpace(baseURL) == "" {
		return chatcompletions.Config{}, fmt.Errorf("--base-url or GATOR_BASE_URL is required for provider %q", provider)
	}
	return chatcompletions.Config{
		APIKey:              apiKey,
		APIKeyEnv:           definition.apiKeyEnv,
		BaseURL:             baseURL,
		Model:               config.Model,
		ProviderName:        definition.name,
		AuthorizationHeader: definition.authorizationHeader,
		AuthorizationPrefix: definition.authorizationPrefix,
		Client:              config.Client,
	}, nil
}

type compatibleProvider struct {
	name                string
	apiKeyEnv           string
	baseURL             string
	authorizationHeader string
	authorizationPrefix string
}

var compatibleProviders = map[Provider]compatibleProvider{
	AzureOpenAI:      {name: "Azure OpenAI API", apiKeyEnv: "AZURE_OPENAI_API_KEY", baseURL: "", authorizationHeader: "api-key"},
	Mistral:          {name: "Mistral API", apiKeyEnv: "MISTRAL_API_KEY", baseURL: "https://api.mistral.ai/v1/chat/completions"},
	XAI:              {name: "xAI API", apiKeyEnv: "XAI_API_KEY", baseURL: "https://api.x.ai/v1/chat/completions"},
	Groq:             {name: "Groq API", apiKeyEnv: "GROQ_API_KEY", baseURL: "https://api.groq.com/openai/v1/chat/completions"},
	OpenRouter:       {name: "OpenRouter API", apiKeyEnv: "OPENROUTER_API_KEY", baseURL: "https://openrouter.ai/api/v1/chat/completions"},
	Together:         {name: "Together AI API", apiKeyEnv: "TOGETHER_API_KEY", baseURL: "https://api.together.xyz/v1/chat/completions"},
	Fireworks:        {name: "Fireworks AI API", apiKeyEnv: "FIREWORKS_API_KEY", baseURL: "https://api.fireworks.ai/inference/v1/chat/completions"},
	DeepSeek:         {name: "DeepSeek API", apiKeyEnv: "DEEPSEEK_API_KEY", baseURL: "https://api.deepseek.com/chat/completions"},
	Cerebras:         {name: "Cerebras Inference", apiKeyEnv: "CEREBRAS_API_KEY", baseURL: "https://api.cerebras.ai/v1/chat/completions"},
	NVIDIA:           {name: "NVIDIA NIM", apiKeyEnv: "NVIDIA_API_KEY", baseURL: "https://integrate.api.nvidia.com/v1/chat/completions"},
	HuggingFace:      {name: "Hugging Face Inference Providers", apiKeyEnv: "HF_TOKEN", baseURL: "https://router.huggingface.co/v1/chat/completions"},
	MoonshotAI:       {name: "Moonshot AI Kimi API", apiKeyEnv: "MOONSHOT_API_KEY", baseURL: "https://api.moonshot.ai/v1/chat/completions"},
	OpenAICompatible: {name: "OpenAI-compatible API", apiKeyEnv: "GATOR_COMPATIBLE_API_KEY", baseURL: ""},
}

// ParseProvider validates a user-visible provider name.
func ParseProvider(value string) (Provider, error) {
	provider := Provider(strings.ToLower(strings.TrimSpace(value)))
	if _, ok := allProviders[provider]; !ok {
		return "", fmt.Errorf("unknown provider %q; choose one of: %s", value, strings.Join(Names(), ", "))
	}
	return provider, nil
}

// Names returns direct provider names in stable display order.
func Names() []string {
	names := make([]string, 0, len(directProviders))
	for provider := range directProviders {
		names = append(names, string(provider))
	}
	sort.Strings(names)
	return names
}

// SupportsDirect reports whether Gator can construct a direct adapter for the
// provider without a vendor CLI or a vendor-owned agent loop.
func SupportsDirect(provider Provider) bool {
	_, ok := directProviders[provider]
	return ok
}

// RequiresOAuthLogin reports providers whose Gator adapter is backed by a
// subscription OAuth credential rather than a normal provider API key.
func RequiresOAuthLogin(provider Provider) bool {
	switch provider {
	case Codex, Claude, Copilot:
		return true
	default:
		return false
	}
}

// SupportsOAuthLogin reports providers that can use a Gator-managed OAuth
// credential in addition to, or instead of, their normal API-key path.
func SupportsOAuthLogin(provider Provider) bool {
	switch provider {
	case Codex, Claude, Copilot, KimiCoding, Radius, XAI, OpenRouter:
		return true
	default:
		return false
	}
}

// SupportsAPIKeyLogin reports whether gator login can safely persist an API
// key for the provider. Subscription providers use a separate OAuth flow.
func SupportsAPIKeyLogin(provider Provider) bool {
	return SupportsDirect(provider) && !RequiresOAuthLogin(provider)
}

// SupportsPDFAttachments reports whether Gator's adapter can encode a PDF
// using the provider's documented native document input. Generic Chat
// Completions endpoints do not share a stable file-input contract.
func SupportsPDFAttachments(provider Provider) bool {
	switch provider {
	case OpenAI, Codex, Anthropic, Claude, Gemini:
		return true
	default:
		return false
	}
}

// DefaultModel returns the stable default where Gator can select one without
// guessing a catalog-specific model. Empty means the user must provide one.
func DefaultModel(provider Provider) string {
	switch provider {
	case OpenAI, Codex:
		return openai.DefaultModel()
	case Anthropic, Claude:
		return "claude-sonnet-5"
	case KimiCoding:
		return "kimi-for-coding"
	case Radius:
		return "auto"
	case Gemini:
		return "gemini-3.5-flash"
	case Mistral:
		return "mistral-large-latest"
	default:
		return ""
	}
}

// EffectiveModel returns an explicit model unchanged or the provider's stable
// direct-API default when one is available.
func EffectiveModel(provider Provider, requested string) string {
	if strings.TrimSpace(requested) != "" {
		return strings.TrimSpace(requested)
	}
	return DefaultModel(provider)
}

// CredentialHint describes the prerequisite without exposing secrets.
func CredentialHint(provider Provider) string {
	switch provider {
	case OpenAI:
		return "OPENAI_API_KEY"
	case Codex:
		return "Gator Codex OAuth credential"
	case Anthropic:
		return "ANTHROPIC_API_KEY"
	case Claude:
		return "Gator Claude OAuth credential"
	case Gemini:
		return "GEMINI_API_KEY"
	case Copilot:
		return "Gator GitHub Copilot OAuth credential"
	case KimiCoding:
		return "KIMI_API_KEY or Gator Kimi Code OAuth credential"
	case Radius:
		return "RADIUS_API_KEY or Gator Radius OAuth credential"
	case Cursor:
		return "no supported direct credential"
	default:
		if definition, ok := compatibleProviders[provider]; ok {
			return definition.apiKeyEnv
		}
		return "provider credentials"
	}
}

// APIKeyEnvironment returns the ambient API-key variable for providers that
// support one. OAuth-only providers return an empty string.
func APIKeyEnvironment(provider Provider) string {
	switch provider {
	case OpenAI:
		return "OPENAI_API_KEY"
	case Anthropic:
		return "ANTHROPIC_API_KEY"
	case Gemini:
		return "GEMINI_API_KEY"
	case KimiCoding:
		return "KIMI_API_KEY"
	case Radius:
		return "RADIUS_API_KEY"
	default:
		if definition, ok := compatibleProviders[provider]; ok {
			return definition.apiKeyEnv
		}
		return ""
	}
}

// key resolves credentials in the same order exposed by the CLI: an explicit
// run key, Gator's provider-scoped local credential, then the provider's
// ambient environment variable. It never reads another application's state.
func key(config Config, provider Provider, environment string) (string, error) {
	if config.APIKey != "" {
		return config.APIKey, nil
	}
	if config.Credentials != nil {
		credential, found, err := config.Credentials.Read(string(provider))
		if err != nil {
			return "", fmt.Errorf("read Gator credential for %q: %w", provider, err)
		}
		if found && credential.IsAPIKey() {
			return credential.Key, nil
		}
		if found && credential.IsOAuth() && (provider == XAI || provider == Radius) {
			if credential.Expired(time.Now()) {
				return "", fmt.Errorf("Gator OAuth credential for %q expired; run 'gator login %s --subscription'", provider, provider)
			}
			return credential.Access, nil
		}
	}
	return os.Getenv(environment), nil
}

func kimiCredential(config Config) (string, bool, error) {
	if config.APIKey != "" {
		return config.APIKey, true, nil
	}
	if config.Credentials != nil {
		credential, found, err := config.Credentials.Read(string(KimiCoding))
		if err != nil {
			return "", false, fmt.Errorf("read Gator credential for %q: %w", KimiCoding, err)
		}
		if found && credential.IsOAuth() {
			if credential.Expired(time.Now()) {
				return "", false, fmt.Errorf("Gator OAuth credential for %q expired; run 'gator login %s --subscription'", KimiCoding, KimiCoding)
			}
			return credential.Access, true, nil
		}
		if found && credential.IsAPIKey() {
			return credential.Key, true, nil
		}
	}
	key := os.Getenv("KIMI_API_KEY")
	if strings.TrimSpace(key) == "" {
		return "", false, errors.New("KIMI_API_KEY or a Gator Kimi Code OAuth credential is required")
	}
	return key, true, nil
}

func oauthCredential(config Config, provider Provider) (auth.Credential, error) {
	if config.Credentials == nil {
		return auth.Credential{}, fmt.Errorf("Gator OAuth credential for %q is required; run 'gator login %s'", provider, provider)
	}
	credential, found, err := config.Credentials.Read(string(provider))
	if err != nil {
		return auth.Credential{}, fmt.Errorf("read Gator credential for %q: %w", provider, err)
	}
	if !found || !credential.IsOAuth() {
		return auth.Credential{}, fmt.Errorf("Gator OAuth credential for %q is required; run 'gator login %s'", provider, provider)
	}
	if credential.Expired(time.Now()) {
		return auth.Credential{}, fmt.Errorf("Gator OAuth credential for %q expired; run 'gator login %s'", provider, provider)
	}
	return credential, nil
}

func codexResponsesURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "https://chatgpt.com/backend-api/codex/responses"
	}
	if strings.HasSuffix(baseURL, "/codex/responses") {
		return baseURL
	}
	if strings.HasSuffix(baseURL, "/codex") {
		return baseURL + "/responses"
	}
	return baseURL + "/codex/responses"
}

func kimiMessagesURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "https://api.kimi.com/coding/v1/messages"
	}
	if strings.HasSuffix(baseURL, "/messages") {
		return baseURL
	}
	return baseURL + "/v1/messages"
}

func codexAccountID(credential auth.Credential) (string, error) {
	if accountID := strings.TrimSpace(credential.Extra["chatgpt_account_id"]); accountID != "" {
		return accountID, nil
	}
	parts := strings.Split(credential.Access, ".")
	if len(parts) != 3 {
		return "", errors.New("Codex OAuth credential has no ChatGPT account identifier")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errors.New("decode Codex OAuth credential account identifier")
	}
	var claims struct {
		Auth struct {
			AccountID string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || strings.TrimSpace(claims.Auth.AccountID) == "" {
		return "", errors.New("Codex OAuth credential has no ChatGPT account identifier")
	}
	return claims.Auth.AccountID, nil
}

func copilotChatURL(override, credentialURL string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(override), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(credentialURL), "/")
	}
	if baseURL == "" {
		baseURL = "https://api.individual.githubcopilot.com"
	}
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return baseURL + "/chat/completions"
}

func copilotRequestHeaders(turn agent.TurnRequest) http.Header {
	initiator := "user"
	if len(turn.Messages) > 0 && turn.Messages[len(turn.Messages)-1].Role != agent.RoleUser {
		initiator = "agent"
	}
	return http.Header{
		"X-Initiator":   []string{initiator},
		"Openai-Intent": []string{"conversation-edits"},
	}
}

func requireKey(value, environment string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", environment)
	}
	return nil
}

// allProviders includes retained legacy provider names so loading a previous
// session produces a clear no-fallback error instead of treating its metadata
// as malformed. Names exposes only providers that can currently execute.
var allProviders = map[Provider]struct{}{
	OpenAI: {}, AzureOpenAI: {}, Anthropic: {}, Gemini: {}, Mistral: {}, XAI: {}, Groq: {}, OpenRouter: {}, Together: {}, Fireworks: {}, DeepSeek: {}, Cerebras: {}, NVIDIA: {}, HuggingFace: {}, MoonshotAI: {}, OpenAICompatible: {}, Codex: {}, Claude: {}, Copilot: {}, KimiCoding: {}, Radius: {}, Cursor: {},
}

var directProviders = map[Provider]struct{}{
	OpenAI: {}, AzureOpenAI: {}, Anthropic: {}, Gemini: {}, Mistral: {}, XAI: {}, Groq: {}, OpenRouter: {}, Together: {}, Fireworks: {}, DeepSeek: {}, Cerebras: {}, NVIDIA: {}, HuggingFace: {}, MoonshotAI: {}, OpenAICompatible: {}, Codex: {}, Claude: {}, Copilot: {}, KimiCoding: {}, Radius: {},
}
