package llm

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type HealthStatus string

const (
	HealthOK      HealthStatus = "ok"
	HealthFail    HealthStatus = "fail"
	HealthUnknown HealthStatus = "unknown"
)

type HealthCheck struct {
	Name   string       `json:"name"`
	Status HealthStatus `json:"status"`
	Detail string       `json:"detail,omitempty"`
	Action string       `json:"action,omitempty"`
}

type HealthReport struct {
	Transport string        `json:"transport"`
	BaseURL   string        `json:"base_url,omitempty"`
	Model     string        `json:"model,omitempty"`
	Checks    []HealthCheck `json:"checks"`
}

type EndpointHealthChecker struct {
	HTTPClient        *http.Client
	CLIRunner         cliRunner
	LookPath          func(string) (string, error)
	AutoPullOllama    bool
	SchemaSmokeOllama bool
}

func CheckEndpointHealth(ctx context.Context, endpoint EndpointConfig) HealthReport {
	return EndpointHealthChecker{}.Check(ctx, endpoint)
}

func (c EndpointHealthChecker) Check(ctx context.Context, endpoint EndpointConfig) HealthReport {
	endpoint.Transport = strings.ToLower(strings.TrimSpace(endpoint.Transport))
	report := HealthReport{
		Transport: endpoint.Transport,
		BaseURL:   endpoint.BaseURL,
		Model:     endpoint.Model,
	}
	switch endpoint.Transport {
	case "ollama":
		return c.checkOllama(ctx, endpoint, report)
	case "openai":
		return c.checkOpenAICompatible(ctx, endpoint, report)
	case "anthropic":
		return c.checkAnthropic(ctx, endpoint, report)
	case "codex-cli", "gemini-cli", "claude-cli", "opencode-cli", "aider-cli", "goose-cli", "qwen-cli", "cursor-cli":
		return c.checkCLI(ctx, endpoint, report)
	case "":
		report.add(HealthCheck{Name: "transport", Status: HealthFail, Detail: "missing transport"})
	default:
		report.add(HealthCheck{Name: "transport", Status: HealthFail, Detail: fmt.Sprintf("unsupported transport %q", endpoint.Transport)})
	}
	return report
}

func (c EndpointHealthChecker) checkOpenAICompatible(ctx context.Context, endpoint EndpointConfig, report HealthReport) HealthReport {
	if endpoint.BaseURL == "" {
		report.add(HealthCheck{Name: "running", Status: HealthFail, Detail: "missing base URL"})
		return report
	}
	headers := map[string]string{}
	if endpoint.APIKey != "" {
		headers["Authorization"] = "Bearer " + endpoint.APIKey
		report.add(HealthCheck{Name: "auth", Status: HealthOK, Detail: "API key configured"})
	} else if isLocalBaseURL(endpoint.BaseURL) {
		report.add(HealthCheck{Name: "auth", Status: HealthOK, Detail: "local OpenAI-compatible endpoint without API key"})
	} else {
		report.add(HealthCheck{Name: "auth", Status: HealthFail, Detail: "missing API key", Action: "set PAW_BRAIN_API_KEY or PAW_DRONE_API_KEY"})
	}
	models, err := c.getModelIDs(ctx, http.MethodGet, strings.TrimRight(endpoint.BaseURL, "/")+"/models", headers, "data")
	if err != nil {
		report.add(HealthCheck{Name: "running", Status: HealthFail, Detail: err.Error()})
	} else {
		report.add(HealthCheck{Name: "running", Status: HealthOK, Detail: "OpenAI-compatible models endpoint responded"})
		report.add(modelHealthCheck(endpoint.Model, models, "choose a model from /models"))
	}
	report.add(HealthCheck{Name: "schema", Status: HealthUnknown, Detail: "json_schema support requires a smoke chat check"})
	return report
}

func (c EndpointHealthChecker) checkAnthropic(ctx context.Context, endpoint EndpointConfig, report HealthReport) HealthReport {
	if endpoint.BaseURL == "" {
		report.add(HealthCheck{Name: "running", Status: HealthFail, Detail: "missing base URL"})
		return report
	}
	headers := map[string]string{"anthropic-version": "2023-06-01"}
	if endpoint.APIKey != "" {
		headers["x-api-key"] = endpoint.APIKey
		report.add(HealthCheck{Name: "auth", Status: HealthOK, Detail: "API key configured"})
	} else {
		report.add(HealthCheck{Name: "auth", Status: HealthFail, Detail: "missing API key", Action: "set PAW_BRAIN_API_KEY"})
	}
	models, err := c.getModelIDs(ctx, http.MethodGet, strings.TrimRight(endpoint.BaseURL, "/")+"/v1/models", headers, "data")
	if err != nil {
		report.add(HealthCheck{Name: "running", Status: HealthFail, Detail: err.Error()})
	} else {
		report.add(HealthCheck{Name: "running", Status: HealthOK, Detail: "Anthropic models endpoint responded"})
		report.add(modelHealthCheck(endpoint.Model, models, "choose a model from /v1/models"))
	}
	report.add(HealthCheck{Name: "schema", Status: HealthUnknown, Detail: "tool/schema support requires a smoke chat check"})
	return report
}

func modelHealthCheck(model string, models map[string]bool, action string) HealthCheck {
	if model == "" {
		return HealthCheck{Name: "model", Status: HealthUnknown, Detail: "no model configured"}
	}
	if models[model] {
		return HealthCheck{Name: "model", Status: HealthOK, Detail: model}
	}
	return HealthCheck{Name: "model", Status: HealthFail, Detail: "configured model not found: " + model, Action: action}
}

func defaultEndpointBaseURL(endpoint EndpointConfig, fallback string) string {
	if endpoint.BaseURL != "" {
		return strings.TrimRight(endpoint.BaseURL, "/")
	}
	return fallback
}

func (r *HealthReport) add(check HealthCheck) {
	r.Checks = append(r.Checks, check)
}
