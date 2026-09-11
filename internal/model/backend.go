// Package model resolves supported direct cloud APIs into provider-independent
// execution backends. Gator always retains the model and tool loop.
package model

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/model/anthropic"
	"github.com/gongahkia/gator/internal/model/bedrock"
	"github.com/gongahkia/gator/internal/model/chatcompletions"
	"github.com/gongahkia/gator/internal/model/gemini"
	"github.com/gongahkia/gator/internal/model/openai"
	"github.com/gongahkia/gator/internal/model/radius"
	"github.com/gongahkia/gator/internal/model/vertex"
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
	case AmazonBedrock:
		compatible, err := bedrockConfig(config)
		if err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: chatcompletions.Model{Config: compatible}}, nil
	case GoogleVertex:
		compatible, err := vertexConfig(config)
		if err != nil {
			return Backend{}, err
		}
		return Backend{Provider: provider, Model: chatcompletions.Model{Config: compatible}}, nil
	case AzureOpenAI, Mistral, XAI, Groq, OpenRouter, Together, Fireworks, DeepSeek, Cerebras, NVIDIA, HuggingFace, MoonshotAI, ZAI, ZAICodingCN, Baseten, VercelAIGateway, AntLing, Xiaomi, MoonshotAICN, QwenTokenPlan, QwenTokenPlanCN, QwenTokenPlanIndividual, XiaomiTokenPlanCN, XiaomiTokenPlanAMS, XiaomiTokenPlanSGP, OpenAICompatible:
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

