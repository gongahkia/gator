// Package model resolves supported direct cloud APIs into provider-independent
// execution backends. Gator always retains the model and tool loop.
package model

import (
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/model/anthropic"
	"github.com/gongahkia/gator/internal/model/chatcompletions"
	"github.com/gongahkia/gator/internal/model/gemini"
	"github.com/gongahkia/gator/internal/model/openai"
	"github.com/gongahkia/gator/internal/model/radius"
)

// newBackend contains provider-specific adapter construction after the public
// factory has validated and normalized its shared configuration.
func newBackend(provider Provider, config Config) (Backend, error) {
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
	case AzureOpenAIResponses:
		if strings.TrimSpace(config.Model) == "" {
			return Backend{}, fmt.Errorf("--model is required for provider %q; use the Azure deployment name", provider)
		}
		credential, err := azureResponsesCredential(config)
		if err != nil {
			return Backend{}, err
		}
		baseURL, err := azureResponsesURL(config.BaseURL)
		if err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: openai.Responses{
			APIKey:              credential.value,
			APIKeyEnv:           credential.source,
			Model:               config.Model,
			BaseURL:             baseURL,
			AuthorizationHeader: credential.header,
			AuthorizationPrefix: credential.prefix,
			Client:              config.Client,
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
	case KimiCoding:
		apiKey, err := key(config, provider, "KIMI_API_KEY")
		if err != nil {
			return Backend{}, err
		}
		if err := requireKey(apiKey, "KIMI_API_KEY"); err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: anthropic.Messages{
			APIKey:     apiKey,
			Model:      config.Model,
			BaseURL:    kimiMessagesURL(config.BaseURL),
			BearerAuth: true,
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
			Gateway: providerOption(config, "gateway", "GATOR_RADIUS_GATEWAY"),
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
	case MiniMax, MiniMaxCN:
		if strings.TrimSpace(config.Model) == "" {
			return Backend{}, fmt.Errorf("--model is required for provider %q", provider)
		}
		apiKeyEnv := "MINIMAX_API_KEY"
		if provider == MiniMaxCN {
			apiKeyEnv = "MINIMAX_CN_API_KEY"
		}
		apiKey, err := key(config, provider, apiKeyEnv)
		if err != nil {
			return Backend{}, err
		}
		if err := requireKey(apiKey, apiKeyEnv); err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: anthropic.Messages{
			APIKey:  apiKey,
			Model:   config.Model,
			BaseURL: miniMaxMessagesURL(config.BaseURL, provider == MiniMaxCN),
			Client:  config.Client,
		}}, nil
	case OpenCode, OpenCodeGo:
		return openCodeBackend(provider, config)
	case CloudflareWorkers:
		compatible, err := cloudflareWorkersConfig(config)
		if err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: chatcompletions.Model{Config: compatible}}, nil
	case CloudflareGateway:
		return cloudflareGatewayBackend(config)
	case AzureOpenAI, Mistral, XAI, Groq, OpenRouter, Together, Fireworks, DeepSeek, Cerebras, NVIDIA, HuggingFace, MoonshotAI, ZAI, ZAICodingCN, Baseten, VercelAIGateway, AntLing, Xiaomi, MoonshotAICN, QwenTokenPlan, QwenTokenPlanCN, QwenTokenPlanIndividual, XiaomiTokenPlanCN, XiaomiTokenPlanAMS, XiaomiTokenPlanSGP, OpenAICompatible:
		compatible, err := compatibleConfig(provider, config)
		if err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: chatcompletions.Model{Config: compatible}}, nil
	default:
		return Backend{}, fmt.Errorf("unsupported provider %q", provider)
	}
}
