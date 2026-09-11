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
	return SupportsDirect(provider) && !RequiresOAuthLogin(provider) && provider != GoogleVertex
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
	case OpenCode:
		return "gpt-5.6-terra"
	case OpenCodeGo:
		return "kimi-k2.6"
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
	case AzureOpenAIResponses:
		return "AZURE_OPENAI_API_KEY or AZURE_OPENAI_AUTH_TOKEN"
	case AmazonBedrock:
		return "AWS_BEARER_TOKEN_BEDROCK or standard AWS credential chain"
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
	case MiniMax:
		return "MINIMAX_API_KEY"
	case MiniMaxCN:
		return "MINIMAX_CN_API_KEY"
	case OpenCode, OpenCodeGo:
		return "OPENCODE_API_KEY"
	case Cursor:
		return "no supported direct credential"
	case GoogleVertex:
		return "GATOR_VERTEX_ACCESS_TOKEN or Google Application Default Credentials"
	default:
		if definition, ok := compatibleProviders[provider]; ok {
			return definition.apiKeyEnv
		}
		return "provider credentials"
	}
}

// AmbientCredentialAvailable reports whether a provider with externally
// managed credentials has a configured, readable credential source.
func AmbientCredentialAvailable(provider Provider) bool {
	switch provider {
	case GoogleVertex:
		return vertex.FromEnvironment().Available()
	case AmazonBedrock:
		_, available := bedrock.AmbientSource()
		return available
	case AzureOpenAIResponses:
		return strings.TrimSpace(os.Getenv("AZURE_OPENAI_API_KEY")) != "" || strings.TrimSpace(os.Getenv("AZURE_OPENAI_AUTH_TOKEN")) != ""
	default:
		return false
	}
}

// AmbientCredentialSource returns a non-secret description suitable for local
// diagnostics. It is empty when a provider has no configured ambient source.
func AmbientCredentialSource(provider Provider) string {
	if provider == AmazonBedrock {
		source, _ := bedrock.AmbientSource()
		return source
	}
	return ""
}

// APIKeyEnvironment returns the ambient API-key variable for providers that
// support one. OAuth-only providers return an empty string.
func APIKeyEnvironment(provider Provider) string {
	switch provider {
	case OpenAI:
		return "OPENAI_API_KEY"
	case AzureOpenAIResponses:
		return "AZURE_OPENAI_API_KEY"
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
