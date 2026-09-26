package model

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gongahkia/gator/internal/model/openai"
)

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

// SupportsAPIKeyLogin reports whether Gator can persist an API key for a
// supported direct backend.
func SupportsAPIKeyLogin(provider Provider) bool {
	return SupportsDirect(provider) && APIKeyEnvironment(provider) != ""
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
// guessing a catalog-specific model. Empty means the user must provide one.
func DefaultModel(provider Provider) string {
	switch provider {
	case OpenAI:
		return openai.DefaultModel()
	case OpenCode:
		return "gpt-5.6-terra"
	case OpenCodeGo:
		return "kimi-k2.6"
	case Anthropic:
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
	case AzureOpenAIResponses:
		return "AZURE_OPENAI_API_KEY"
	case CloudflareWorkers, CloudflareGateway:
		return "CLOUDFLARE_API_TOKEN"
	case Anthropic:
		return "ANTHROPIC_API_KEY"
	case Gemini:
		return "GEMINI_API_KEY"
	case KimiCoding:
		return "KIMI_API_KEY"
	case Radius:
		return "RADIUS_API_KEY"
	case MiniMax:
		return "MINIMAX_API_KEY"
	case MiniMaxCN:
		return "MINIMAX_CN_API_KEY"
	case OpenCode, OpenCodeGo:
		return "OPENCODE_API_KEY"
	default:
		if definition, ok := compatibleProviders[provider]; ok {
			return definition.apiKeyEnv
		}
		return "provider credentials"
	}
}

// APIKeyEnvironment returns the ambient API-key variable for providers that
// support one.
func APIKeyEnvironment(provider Provider) string {
	switch provider {
	case OpenAI:
		return "OPENAI_API_KEY"
	case AzureOpenAIResponses:
		return "AZURE_OPENAI_API_KEY"
	case CloudflareWorkers, CloudflareGateway:
		return "CLOUDFLARE_API_TOKEN"
	case Anthropic:
		return "ANTHROPIC_API_KEY"
	case Gemini:
		return "GEMINI_API_KEY"
	case KimiCoding:
		return "KIMI_API_KEY"
	case Radius:
		return "RADIUS_API_KEY"
	case MiniMax:
		return "MINIMAX_API_KEY"
	case MiniMaxCN:
		return "MINIMAX_CN_API_KEY"
	case OpenCode, OpenCodeGo:
		return "OPENCODE_API_KEY"
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
