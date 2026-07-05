package llm

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

type EndpointConfig struct {
	Transport string
	BaseURL   string
	APIKey    string
	Provider  string
	Model     string
}

type FactoryConfig struct {
	Brain       EndpointConfig
	Drone       EndpointConfig
	CallTimeout time.Duration
}

func NewBrainClient(cfg FactoryConfig) (Client, error) {
	endpoint := endpointFromEnv(cfg.Brain, "PAW_BRAIN_")
	if endpoint.Transport == "" {
		endpoint.Transport = "ollama"
	}
	if endpoint.BaseURL == "" && strings.EqualFold(endpoint.Transport, "ollama") {
		endpoint.BaseURL = "http://localhost:11434"
	}
	if endpoint.Model == "" && strings.EqualFold(endpoint.Transport, "ollama") {
		endpoint.Model = "gpt-oss:20b"
	}
	if requiresEndpointKey(endpoint) && endpoint.APIKey == "" {
		return missingKeyClient{envKey: "PAW_BRAIN_API_KEY"}, nil
	}
	return newClient(endpoint, callTimeout(cfg.CallTimeout))
}

func NewDroneClient(cfg FactoryConfig) (Client, error) {
	endpoint := endpointFromEnv(cfg.Drone, "PAW_DRONE_")
	if endpoint.Transport == "" {
		endpoint.Transport = "ollama"
	}
	if endpoint.BaseURL == "" && strings.EqualFold(endpoint.Transport, "ollama") {
		endpoint.BaseURL = "http://localhost:11434"
	}
	if endpoint.Model == "" && strings.EqualFold(endpoint.Transport, "ollama") {
		endpoint.Model = "qwen3:8b"
	}
	return newClient(endpoint, callTimeout(cfg.CallTimeout))
}

func newClient(endpoint EndpointConfig, timeout time.Duration) (Client, error) {
	var client Client
	switch strings.ToLower(endpoint.Transport) {
	case "openai":
		client = NewOpenAIClient(endpoint.BaseURL, endpoint.APIKey, endpoint.Model)
	case "anthropic":
		client = NewAnthropicClient(endpoint.BaseURL, endpoint.APIKey, endpoint.Model)
	case "ollama":
		client = NewOllamaClient(endpoint.BaseURL, endpoint.Model)
	case "codex-cli":
		client = NewCodexCLIClient(endpoint.Model)
	case "gemini-cli":
		client = NewGeminiCLIClient(endpoint.Model)
	case "claude-cli":
		client = NewClaudeCLIClient(endpoint.Model)
	case "opencode-cli":
		client = NewOpenCodeCLIClient(endpoint.Model)
	case "aider-cli":
		client = NewAiderCLIClient(endpoint.Model)
	case "goose-cli":
		client = NewGooseCLIClient(endpoint.Provider, endpoint.Model)
	case "qwen-cli":
		client = NewQwenCLIClient(endpoint.Model)
	default:
		return nil, fmt.Errorf("unsupported llm transport %q", endpoint.Transport)
	}
	if timeout > 0 {
		client = NewRetryClient(client, timeout)
	}
	return client, nil
}

func requiresEndpointKey(endpoint EndpointConfig) bool {
	switch strings.ToLower(endpoint.Transport) {
	case "openai":
		return !isLocalBaseURL(endpoint.BaseURL)
	case "anthropic":
		return true
	default:
		return false
	}
}

func isLocalBaseURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func endpointFromEnv(cfg EndpointConfig, prefix string) EndpointConfig {
	if v := os.Getenv(prefix + "TRANSPORT"); v != "" {
		cfg.Transport = v
	}
	if v := os.Getenv(prefix + "BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv(prefix + "API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv(prefix + "PROVIDER"); v != "" {
		cfg.Provider = v
	}
	if v := os.Getenv(prefix + "MODEL"); v != "" {
		cfg.Model = v
	}
	return cfg
}

func callTimeout(current time.Duration) time.Duration {
	if v := os.Getenv("PAW_CALL_TIMEOUT"); v != "" {
		timeout, err := time.ParseDuration(v)
		if err == nil {
			return timeout
		}
	}
	return current
}

type missingKeyClient struct {
	envKey string
}

func (c missingKeyClient) Chat(context.Context, ChatRequest) (*ChatResponse, error) {
	return nil, fmt.Errorf("missing required %s", c.envKey)
}
