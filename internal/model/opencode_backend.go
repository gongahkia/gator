package model

import (
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/model/anthropic"
	"github.com/gongahkia/gator/internal/model/chatcompletions"
	"github.com/gongahkia/gator/internal/model/gemini"
	"github.com/gongahkia/gator/internal/model/openai"
)

func openCodeBackend(provider Provider, config Config) (Backend, error) {
	apiKey, err := key(config, provider, "OPENCODE_API_KEY")
	if err != nil {
		return Backend{}, err
	}
	if err := requireKey(apiKey, "OPENCODE_API_KEY"); err != nil {
		return Backend{}, err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://opencode.ai/zen"
		if provider == OpenCodeGo {
			baseURL += "/go"
		}
	}
	protocol, found := openCodeModelProtocol(provider, config.Model)
	if !found {
		return Backend{}, fmt.Errorf("model %q is not in the embedded %s catalog version %s; choose a documented OpenCode model or update Gator's catalog", config.Model, openCodeProviderName(provider), openCodeCatalogVersion)
	}
	switch protocol {
	case openCodeResponses:
		return Backend{Provider: provider, Model: openai.Responses{
			APIKey:    apiKey,
			APIKeyEnv: "OPENCODE_API_KEY",
			Model:     config.Model,
			BaseURL:   openCodeEndpoint(baseURL, "/responses"),
			Client:    config.Client,
		}}, nil
	case openCodeAnthropic:
		return Backend{Provider: provider, Model: anthropic.Messages{
			APIKey:  apiKey,
			Model:   config.Model,
			BaseURL: openCodeEndpoint(baseURL, "/messages"),
			Client:  config.Client,
		}}, nil
	case openCodeGemini:
		return Backend{Provider: provider, Model: gemini.GenerateContent{
			APIKey:  apiKey,
			Model:   config.Model,
			BaseURL: openCodeAPIVersionBase(baseURL),
			Client:  config.Client,
		}}, nil
	case openCodeChatCompletions:
		providerName := "OpenCode Zen"
		if provider == OpenCodeGo {
			providerName = "OpenCode Go"
		}
		return Backend{Provider: provider, Model: chatcompletions.Model{Config: chatcompletions.Config{
			APIKey:       apiKey,
			APIKeyEnv:    "OPENCODE_API_KEY",
			BaseURL:      openCodeEndpoint(baseURL, "/chat/completions"),
			Model:        config.Model,
			ProviderName: providerName,
			Client:       config.Client,
		}}}, nil
	default:
		return Backend{}, fmt.Errorf("model %q has an unsupported %s protocol in catalog version %s", config.Model, openCodeProviderName(provider), openCodeCatalogVersion)
	}
}

func openCodeEndpoint(baseURL, suffix string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, suffix) {
		return baseURL
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + suffix
	}
	return baseURL + "/v1" + suffix
}

func openCodeAPIVersionBase(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL
	}
	return baseURL + "/v1"
}
