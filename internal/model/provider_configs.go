package model

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

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

// azureResponsesCredential resolves an API key using the same explicit,
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
		if found && credential.IsAPIKey() {
			return azureResponsesAuth{value: credential.Key, source: "Gator Azure OpenAI Responses API-key credential", header: "api-key"}, nil
		}
	}
	if value := strings.TrimSpace(os.Getenv("AZURE_OPENAI_API_KEY")); value != "" {
		return azureResponsesAuth{value: value, source: "AZURE_OPENAI_API_KEY", header: "api-key"}, nil
	}
	return azureResponsesAuth{}, errors.New("AZURE_OPENAI_API_KEY is required")
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
