package model

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gongahkia/gator/internal/model/anthropic"
	"github.com/gongahkia/gator/internal/model/chatcompletions"
	"github.com/gongahkia/gator/internal/model/openai"
)

func vertexConfig(config Config) (chatcompletions.Config, error) {
	project := providerOption(config, "project", "GOOGLE_CLOUD_PROJECT")
	if project == "" {
		project = strings.TrimSpace(os.Getenv("GCLOUD_PROJECT"))
	}
	if project == "" {
		return chatcompletions.Config{}, errors.New("GOOGLE_CLOUD_PROJECT or GCLOUD_PROJECT is required")
	}
	location := providerOption(config, "location", "GOOGLE_CLOUD_LOCATION")
	if location == "" {
		return chatcompletions.Config{}, errors.New("GOOGLE_CLOUD_LOCATION is required")
	}
	baseURL := strings.TrimSpace(config.BaseURL)
	if baseURL == "" {
		baseURL = "https://" + location + "-aiplatform.googleapis.com/v1/projects/" + project + "/locations/" + location + "/endpoints/openapi/chat/completions"
	}
	if strings.TrimSpace(config.Model) == "" {
		return chatcompletions.Config{}, fmt.Errorf("--model is required for provider %q", GoogleVertex)
	}
	credentials, err := vertexCredentialSource(config)
	if err != nil {
		return chatcompletions.Config{}, err
	}
	return chatcompletions.Config{
		APIKeySource: credentials.Token,
		APIKeyEnv:    "GATOR_VERTEX_ACCESS_TOKEN or Google Application Default Credentials",
		BaseURL:      baseURL,
		Model:        config.Model,
		ProviderName: "Google Vertex AI",
		Client:       config.Client,
	}, nil
}

func cloudflareWorkersConfig(config Config) (chatcompletions.Config, error) {
	apiKey, err := key(config, CloudflareWorkers, "CLOUDFLARE_API_TOKEN")
	if err != nil {
		return chatcompletions.Config{}, err
	}
	if err := requireKey(apiKey, "CLOUDFLARE_API_TOKEN"); err != nil {
		return chatcompletions.Config{}, err
	}
	baseURL := strings.TrimSpace(config.BaseURL)
	if baseURL == "" {
		accountID := providerOption(config, "account_id", "CLOUDFLARE_ACCOUNT_ID")
		if accountID == "" {
			return chatcompletions.Config{}, errors.New("CLOUDFLARE_ACCOUNT_ID is required")
		}
		baseURL = "https://api.cloudflare.com/client/v4/accounts/" + accountID + "/ai/v1/chat/completions"
	}
	if strings.TrimSpace(config.Model) == "" {
		return chatcompletions.Config{}, fmt.Errorf("--model is required for provider %q", CloudflareWorkers)
	}
	return chatcompletions.Config{
		APIKey:       apiKey,
		APIKeyEnv:    "CLOUDFLARE_API_TOKEN",
		BaseURL:      baseURL,
		Model:        config.Model,
		ProviderName: "Cloudflare Workers AI",
		Client:       config.Client,
	}, nil
}

type cloudflareGatewayProtocol string

const (
	cloudflareGatewayOpenAIResponses          cloudflareGatewayProtocol = "openai-responses"
	cloudflareGatewayAnthropicMessages        cloudflareGatewayProtocol = "anthropic-messages"
	cloudflareGatewayWorkersAIChatCompletions cloudflareGatewayProtocol = "workers-ai-chat-completions"
)

type cloudflareGatewayConfig struct {
	apiKey   string
	model    string
	baseURL  string
	gateway  string
	protocol cloudflareGatewayProtocol
}

// cloudflareGatewayBackend uses Cloudflare's account REST API, where each
// native request schema has an explicit endpoint. The caller must select the
// schema rather than relying on an ambiguous model-name heuristic.
func cloudflareGatewayBackend(config Config) (Backend, error) {
	configured, err := resolveCloudflareGatewayConfig(config)
	if err != nil {
		return Backend{}, err
	}
	headers := http.Header{"Cf-Aig-Gateway-Id": []string{configured.gateway}}
	switch configured.protocol {
	case cloudflareGatewayOpenAIResponses:
		return Backend{Provider: CloudflareGateway, Model: openai.Responses{
			APIKey:    configured.apiKey,
			APIKeyEnv: "CLOUDFLARE_API_TOKEN",
			Model:     configured.model,
			BaseURL:   cloudflareGatewayEndpoint(configured.baseURL, "/responses"),
			Headers:   headers,
			Client:    config.Client,
		}}, nil
	case cloudflareGatewayAnthropicMessages:
		return Backend{Provider: CloudflareGateway, Model: anthropic.Messages{
			APIKey:     configured.apiKey,
			Model:      configured.model,
			BaseURL:    cloudflareGatewayEndpoint(configured.baseURL, "/messages"),
			BearerAuth: true,
			Headers:    headers,
			Client:     config.Client,
		}}, nil
	case cloudflareGatewayWorkersAIChatCompletions:
		return Backend{Provider: CloudflareGateway, Model: chatcompletions.Model{Config: chatcompletions.Config{
			APIKey:       configured.apiKey,
			APIKeyEnv:    "CLOUDFLARE_API_TOKEN",
			BaseURL:      cloudflareGatewayEndpoint(configured.baseURL, "/chat/completions"),
			Model:        configured.model,
			ProviderName: "Cloudflare AI Gateway Workers AI",
			Headers:      headers,
			Client:       config.Client,
		}}}, nil
	default:
		return Backend{}, fmt.Errorf("unsupported Cloudflare AI Gateway protocol %q", configured.protocol)
	}
}

func resolveCloudflareGatewayConfig(config Config) (cloudflareGatewayConfig, error) {
	apiKey, err := key(config, CloudflareGateway, "CLOUDFLARE_API_TOKEN")
	if err != nil {
		return cloudflareGatewayConfig{}, err
	}
	if err := requireKey(apiKey, "CLOUDFLARE_API_TOKEN"); err != nil {
		return cloudflareGatewayConfig{}, err
	}
	if strings.TrimSpace(config.Model) == "" {
		return cloudflareGatewayConfig{}, fmt.Errorf("--model is required for provider %q", CloudflareGateway)
	}
	accountID := strings.TrimSpace(config.CloudflareAccountID)
	if accountID == "" {
		accountID = providerOption(config, "account_id", "")
	}
	if accountID == "" {
		accountID = strings.TrimSpace(os.Getenv("CLOUDFLARE_ACCOUNT_ID"))
	}
	if accountID == "" {
		return cloudflareGatewayConfig{}, errors.New("CLOUDFLARE_ACCOUNT_ID is required for provider \"cloudflare-ai-gateway\"")
	}
	gatewayID := strings.TrimSpace(config.CloudflareGatewayID)
	if gatewayID == "" {
		gatewayID = providerOption(config, "gateway_id", "")
	}
	if gatewayID == "" {
		gatewayID = strings.TrimSpace(os.Getenv("CLOUDFLARE_AI_GATEWAY_ID"))
	}
	if gatewayID == "" {
		return cloudflareGatewayConfig{}, errors.New("CLOUDFLARE_AI_GATEWAY_ID is required for provider \"cloudflare-ai-gateway\"")
	}
	protocolOption := config.CloudflareGatewayProtocol
	if strings.TrimSpace(protocolOption) == "" {
		protocolOption = providerOption(config, "gateway_protocol", "")
	}
	protocol, err := parseCloudflareGatewayProtocol(protocolOption)
	if err != nil {
		return cloudflareGatewayConfig{}, err
	}
	if err := validateCloudflareGatewayModel(protocol, config.Model); err != nil {
		return cloudflareGatewayConfig{}, err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.cloudflare.com/client/v4/accounts/" + accountID + "/ai/v1"
	}
	return cloudflareGatewayConfig{apiKey: apiKey, model: config.Model, baseURL: baseURL, gateway: gatewayID, protocol: protocol}, nil
}

func parseCloudflareGatewayProtocol(configured string) (cloudflareGatewayProtocol, error) {
	if strings.TrimSpace(configured) == "" {
		configured = os.Getenv("GATOR_CLOUDFLARE_GATEWAY_PROTOCOL")
	}
	protocol := cloudflareGatewayProtocol(strings.TrimSpace(strings.ToLower(configured)))
	switch protocol {
	case cloudflareGatewayOpenAIResponses, cloudflareGatewayAnthropicMessages, cloudflareGatewayWorkersAIChatCompletions:
		return protocol, nil
	default:
		return "", errors.New("GATOR_CLOUDFLARE_GATEWAY_PROTOCOL is required and must be openai-responses, anthropic-messages, or workers-ai-chat-completions")
	}
}

func validateCloudflareGatewayModel(protocol cloudflareGatewayProtocol, model string) error {
	model = strings.TrimSpace(model)
	switch {
	case protocol == cloudflareGatewayOpenAIResponses && strings.HasPrefix(model, "openai/"):
		return nil
	case protocol == cloudflareGatewayAnthropicMessages && strings.HasPrefix(model, "anthropic/"):
		return nil
	case protocol == cloudflareGatewayWorkersAIChatCompletions && strings.HasPrefix(model, "@cf/"):
		return nil
	default:
		return fmt.Errorf("model %q is incompatible with Cloudflare AI Gateway protocol %q", model, protocol)
	}
}

func cloudflareGatewayEndpoint(baseURL, suffix string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, suffix) {
		return baseURL
	}
	return baseURL + suffix
}
