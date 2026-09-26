package model

import (
	"fmt"
	"os"
	"strings"

)

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
	}
	return os.Getenv(environment), nil
}

func providerOption(config Config, name, environment string) string {
	if value := strings.TrimSpace(config.ProviderOptions[name]); value != "" {
		return value
	}
	if environment == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(environment))
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


func requireKey(value, environment string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", environment)
	}
	return nil
}
