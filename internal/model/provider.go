// Package model resolves Gator's supported cloud APIs and locally installed
// subscription CLI harnesses into provider-independent execution backends.
package model

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/harness"
	"github.com/gongahkia/gator/internal/model/anthropic"
	"github.com/gongahkia/gator/internal/model/chatcompletions"
	"github.com/gongahkia/gator/internal/model/gemini"
	"github.com/gongahkia/gator/internal/model/openai"
)

// Provider identifies a supported model backend.
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
	OpenAICompatible Provider = "openai-compatible"
	Codex            Provider = "codex"
	Claude           Provider = "claude"
	Copilot          Provider = "copilot"
	Cursor           Provider = "cursor"
)

// Config selects one provider. APIKey is optional only because the factory can
// obtain the provider's documented environment variable when it is unset.
type Config struct {
	Provider Provider
	Model    string
	BaseURL  string
	APIKey   string
	Client   *http.Client
}

// Backend has exactly one execution mode. Native models retain Gator's tool
// loop; Harness uses a vendor CLI's own supported agent loop in the Gator
// worktree and returns to Gator for verification and review.
type Backend struct {
	Provider Provider
	Model    agent.Model
	Harness  harness.Runner
}

// New resolves one configured provider. It never loads another program's
// OAuth files: subscription credentials remain owned by the vendor CLI.
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
		apiKey := key(config, "OPENAI_API_KEY")
		if err := requireKey(apiKey, "OPENAI_API_KEY"); err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: openai.Responses{APIKey: apiKey, Model: config.Model, BaseURL: config.BaseURL, Client: config.Client}}, nil
	case Anthropic:
		apiKey := key(config, "ANTHROPIC_API_KEY")
		if err := requireKey(apiKey, "ANTHROPIC_API_KEY"); err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: anthropic.Messages{APIKey: apiKey, Model: config.Model, BaseURL: config.BaseURL, Client: config.Client}}, nil
	case Gemini:
		apiKey := key(config, "GEMINI_API_KEY")
		if err := requireKey(apiKey, "GEMINI_API_KEY"); err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: gemini.GenerateContent{APIKey: apiKey, Model: config.Model, BaseURL: config.BaseURL, Client: config.Client}}, nil
	case AzureOpenAI, Mistral, XAI, Groq, OpenRouter, Together, Fireworks, DeepSeek, OpenAICompatible:
		compatible, err := compatibleConfig(provider, config)
		if err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: chatcompletions.Model{Config: compatible}}, nil
	case Codex, Claude, Copilot, Cursor:
		cli, err := harness.New(harness.Provider(provider))
		if err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Harness: cli}, nil
	default:
		return Backend{}, fmt.Errorf("unsupported provider %q", provider)
	}
}

func compatibleConfig(provider Provider, config Config) (chatcompletions.Config, error) {
	definition, ok := compatibleProviders[provider]
	if !ok {
		return chatcompletions.Config{}, fmt.Errorf("provider %q is not OpenAI-compatible", provider)
	}
	apiKey := key(config, definition.apiKeyEnv)
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

// Names returns supported provider names in stable display order.
func Names() []string {
	names := make([]string, 0, len(allProviders))
	for provider := range allProviders {
		names = append(names, string(provider))
	}
	sort.Strings(names)
	return names
}

// IsHarness reports whether the provider invokes an already-authenticated CLI
// rather than Gator's native model/tool loop.
func IsHarness(provider Provider) bool {
	switch provider {
	case Codex, Claude, Copilot, Cursor:
		return true
	default:
		return false
	}
}

// SupportsPDFAttachments reports whether Gator's adapter can encode a PDF
// using the provider's documented native document input. Generic Chat
// Completions endpoints do not share a stable file-input contract.
func SupportsPDFAttachments(provider Provider) bool {
	switch provider {
	case OpenAI, Anthropic, Gemini:
		return true
	default:
		return false
	}
}

// DefaultModel returns the stable default where Gator can select one without
// guessing a catalog-specific model. Empty means the user or CLI chooses it.
func DefaultModel(provider Provider) string {
	switch provider {
	case OpenAI:
		return openai.DefaultModel()
	case Anthropic:
		return "claude-sonnet-5"
	case Gemini:
		return "gemini-3.5-flash"
	case Mistral:
		return "mistral-large-latest"
	default:
		return ""
	}
}

// EffectiveModel returns an explicit model unchanged or the provider's stable
// default when one is available. An empty result means the selected vendor CLI
// is responsible for choosing its own configured default.
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
	case Anthropic:
		return "ANTHROPIC_API_KEY"
	case Gemini:
		return "GEMINI_API_KEY"
	case Codex, Claude, Copilot, Cursor:
		return string(provider) + " CLI login"
	default:
		if definition, ok := compatibleProviders[provider]; ok {
			return definition.apiKeyEnv
		}
		return "provider credentials"
	}
}

func key(config Config, environment string) string {
	if config.APIKey != "" {
		return config.APIKey
	}
	return os.Getenv(environment)
}

func requireKey(value, environment string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", environment)
	}
	return nil
}

var allProviders = map[Provider]struct{}{
	OpenAI: {}, AzureOpenAI: {}, Anthropic: {}, Gemini: {}, Mistral: {}, XAI: {}, Groq: {}, OpenRouter: {}, Together: {}, Fireworks: {}, DeepSeek: {}, OpenAICompatible: {}, Codex: {}, Claude: {}, Copilot: {}, Cursor: {},
}
