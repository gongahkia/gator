package model

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/model/bedrock"
	"github.com/gongahkia/gator/internal/model/chatcompletions"
)

func bedrockConfig(config Config) (chatcompletions.Config, error) {
	apiKey, err := bearerOrAPIKey(config, AmazonBedrock, "AWS_BEARER_TOKEN_BEDROCK")
	if err != nil {
		return chatcompletions.Config{}, err
	}
	if strings.TrimSpace(config.Model) == "" {
		return chatcompletions.Config{}, fmt.Errorf("--model is required for provider %q", AmazonBedrock)
	}
	baseURL := strings.TrimSpace(config.BaseURL)
	if strings.TrimSpace(apiKey) != "" {
		if baseURL == "" {
			region := providerOption(config, "region", "AWS_REGION")
			if region == "" {
				region = "us-east-1"
			}
			baseURL = "https://bedrock-mantle." + region + ".api.aws/v1/chat/completions"
		}
		return chatcompletions.Config{
			APIKey:       apiKey,
			APIKeyEnv:    "AWS_BEARER_TOKEN_BEDROCK",
			BaseURL:      baseURL,
			Model:        config.Model,
			ProviderName: "Amazon Bedrock",
			Client:       config.Client,
		}, nil
	}
	signer, err := bedrock.LoadProfile(
		context.Background(),
		config.Client,
		providerOption(config, "region", "AWS_REGION"),
		providerOption(config, "profile", "AWS_PROFILE"),
	)
	if err != nil {
		return chatcompletions.Config{}, fmt.Errorf("load standard AWS credentials: %w", err)
	}
	if baseURL == "" {
		baseURL = "https://bedrock-runtime." + signer.Region() + ".amazonaws.com/v1/chat/completions"
	}
	return chatcompletions.Config{
		APIKeyEnv:     "AWS_BEARER_TOKEN_BEDROCK or standard AWS credentials",
		BaseURL:       baseURL,
		Model:         config.Model,
		ProviderName:  "Amazon Bedrock",
		RequestSigner: signer.SignRequest,
		Client:        config.Client,
	}, nil
}

func azureResponsesURL(configured string) (string, error) {
	baseURL := strings.TrimSpace(configured)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("AZURE_OPENAI_BASE_URL"))
	}
	if baseURL == "" {
		resource := strings.TrimSpace(os.Getenv("AZURE_OPENAI_RESOURCE_NAME"))
		if resource == "" {
			return "", errors.New("--base-url, AZURE_OPENAI_BASE_URL, or AZURE_OPENAI_RESOURCE_NAME is required for provider \"azure-openai-responses\"")
		}
		baseURL = "https://" + resource + ".openai.azure.com"
	}
	endpoint, err := url.Parse(baseURL)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return "", fmt.Errorf("invalid Azure OpenAI Responses base URL %q", baseURL)
	}
	path := strings.TrimRight(endpoint.Path, "/")
	switch {
	case strings.HasSuffix(path, "/responses"):
	case strings.HasSuffix(path, "/openai/v1"):
		path += "/responses"
	default:
		path += "/openai/v1/responses"
	}
	endpoint.Path = path
	query := endpoint.Query()
	if query.Get("api-version") == "" {
		apiVersion := strings.TrimSpace(os.Getenv("AZURE_OPENAI_API_VERSION"))
		if apiVersion == "" {
			apiVersion = "v1"
		}
		query.Set("api-version", apiVersion)
	}
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

type azureResponsesAuth struct {
	value  string
	source string
	header string
	prefix string
}

// azureResponsesCredential resolves API-key authentication before an ambient
// Entra token. A Gator-owned provider credential retains the normal explicit,
// stored, then environment precedence used by the rest of the CLI.
func azureResponsesCredential(config Config) (azureResponsesAuth, error) {
	if value := strings.TrimSpace(config.APIKey); value != "" {
		return azureResponsesAuth{value: value, source: "AZURE_OPENAI_API_KEY", header: "api-key"}, nil
	}
	if config.Credentials != nil {
		credential, found, err := config.Credentials.Read(string(AzureOpenAIResponses))
		if err != nil {
			return azureResponsesAuth{}, fmt.Errorf("read Gator credential for %q: %w", AzureOpenAIResponses, err)
		}
		if found {
			switch {
			case credential.IsAPIKey():
				return azureResponsesAuth{value: credential.Key, source: "Gator Azure OpenAI Responses API-key credential", header: "api-key"}, nil
			case credential.IsBearerToken():
				if credential.Expired(time.Now()) {
					return azureResponsesAuth{}, errors.New("Gator Azure OpenAI Responses bearer token expired; replace it with 'gator login azure-openai-responses --bearer-token' or set AZURE_OPENAI_AUTH_TOKEN")
				}
				return azureResponsesAuth{value: credential.Access, source: "Gator Azure OpenAI Responses bearer-token credential", header: "Authorization", prefix: "Bearer "}, nil
			}
		}
	}
	if value := strings.TrimSpace(os.Getenv("AZURE_OPENAI_API_KEY")); value != "" {
		return azureResponsesAuth{value: value, source: "AZURE_OPENAI_API_KEY", header: "api-key"}, nil
	}
	if value := strings.TrimSpace(os.Getenv("AZURE_OPENAI_AUTH_TOKEN")); value != "" {
		return azureResponsesAuth{value: value, source: "AZURE_OPENAI_AUTH_TOKEN", header: "Authorization", prefix: "Bearer "}, nil
	}
	return azureResponsesAuth{}, errors.New("AZURE_OPENAI_API_KEY or AZURE_OPENAI_AUTH_TOKEN is required")
}

func miniMaxMessagesURL(configured string, china bool) string {
	baseURL := strings.TrimRight(strings.TrimSpace(configured), "/")
	if baseURL == "" {
		if china {
			baseURL = "https://api.minimaxi.com/anthropic"
		} else {
			baseURL = "https://api.minimax.io/anthropic"
		}
	}
	if strings.HasSuffix(baseURL, "/v1/messages") {
		return baseURL
	}
	return baseURL + "/v1/messages"
}

// openCodeBackend uses the checked-in OpenCode model catalog rather than
// guessing a protocol from a model-family prefix. Gator never launches
// OpenCode or reads its credential store.
