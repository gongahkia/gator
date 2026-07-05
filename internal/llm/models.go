package llm

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type ModelListOptions struct {
	Query    string
	Provider string
}

func ListEndpointModels(ctx context.Context, endpoint EndpointConfig, opts ModelListOptions) ([]ModelInfo, error) {
	return EndpointHealthChecker{}.ListEndpointModels(ctx, endpoint, opts)
}

func (c EndpointHealthChecker) ListEndpointModels(ctx context.Context, endpoint EndpointConfig, opts ModelListOptions) ([]ModelInfo, error) {
	transport := strings.ToLower(strings.TrimSpace(endpoint.Transport))
	switch transport {
	case "ollama":
		return c.ListOllamaModels(ctx, endpoint.BaseURL)
	case "openai":
		return c.listOpenAIModels(ctx, endpoint)
	case "anthropic":
		return c.listAnthropicModels(ctx, endpoint)
	case "opencode-cli":
		return c.listOpenCodeModels(ctx, opts.Provider)
	case "aider-cli":
		return c.listAiderModels(ctx, opts.Query)
	case "cursor-cli":
		return c.listCursorModels(ctx)
	case "goose-cli":
		return nil, fmt.Errorf("model listing unsupported for transport %q; run goose configure or set PAW_BRAIN_PROVIDER and PAW_BRAIN_MODEL", endpoint.Transport)
	case "qwen-cli":
		return nil, fmt.Errorf("model listing unsupported for transport %q; Qwen Code exposes model switching interactively via /model, so set PAW_BRAIN_MODEL explicitly", endpoint.Transport)
	case "":
		return nil, fmt.Errorf("missing transport")
	default:
		return nil, fmt.Errorf("model listing unsupported for transport %q; set the model explicitly in config or PAW_BRAIN_MODEL", endpoint.Transport)
	}
}

func (c EndpointHealthChecker) listOpenAIModels(ctx context.Context, endpoint EndpointConfig) ([]ModelInfo, error) {
	if endpoint.BaseURL == "" {
		return nil, fmt.Errorf("missing base URL")
	}
	if requiresEndpointKey(endpoint) && endpoint.APIKey == "" {
		return nil, fmt.Errorf("missing API key for non-local OpenAI-compatible model listing")
	}
	headers := map[string]string{}
	if endpoint.APIKey != "" {
		headers["Authorization"] = "Bearer " + endpoint.APIKey
	}
	return c.getModelInfos(ctx, http.MethodGet, strings.TrimRight(endpoint.BaseURL, "/")+"/models", headers, "data")
}

func (c EndpointHealthChecker) listAnthropicModels(ctx context.Context, endpoint EndpointConfig) ([]ModelInfo, error) {
	if endpoint.BaseURL == "" {
		return nil, fmt.Errorf("missing base URL")
	}
	if endpoint.APIKey == "" {
		return nil, fmt.Errorf("missing API key for Anthropic model listing")
	}
	headers := map[string]string{
		"x-api-key":         endpoint.APIKey,
		"anthropic-version": "2023-06-01",
	}
	return c.getModelInfos(ctx, http.MethodGet, strings.TrimRight(endpoint.BaseURL, "/")+"/v1/models", headers, "data")
}

func (c EndpointHealthChecker) listOpenCodeModels(ctx context.Context, provider string) ([]ModelInfo, error) {
	args := []string{"models"}
	if provider != "" {
		args = append(args, provider)
	}
	out, err := c.runCLI(ctx, "opencode", args)
	if err != nil {
		return nil, err
	}
	return parseModelLines(out), nil
}

func (c EndpointHealthChecker) listAiderModels(ctx context.Context, query string) ([]ModelInfo, error) {
	if query == "" {
		return nil, fmt.Errorf("aider model listing requires --query because aider --list-models requires a partial model name")
	}
	out, err := c.runCLI(ctx, "aider", []string{"--list-models", query})
	if err != nil {
		return nil, err
	}
	return parseModelLines(out), nil
}

func (c EndpointHealthChecker) listCursorModels(ctx context.Context) ([]ModelInfo, error) {
	out, err := c.runCLI(ctx, "cursor-agent", []string{"models"})
	if err != nil {
		return nil, err
	}
	return parseModelLines(out), nil
}

func parseModelLines(raw string) []ModelInfo {
	var models []ModelInfo
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 {
			models = append(models, ModelInfo{ID: fields[0]})
		}
	}
	return models
}
