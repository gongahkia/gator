package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
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
	HTTPClient *http.Client
	CLIRunner  cliRunner
	LookPath   func(string) (string, error)
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
	case "codex-cli", "gemini-cli", "claude-cli", "opencode-cli":
		return c.checkCLI(ctx, endpoint, report)
	case "":
		report.add(HealthCheck{Name: "transport", Status: HealthFail, Detail: "missing transport"})
	default:
		report.add(HealthCheck{Name: "transport", Status: HealthFail, Detail: fmt.Sprintf("unsupported transport %q", endpoint.Transport)})
	}
	return report
}

func (c EndpointHealthChecker) checkOllama(ctx context.Context, endpoint EndpointConfig, report HealthReport) HealthReport {
	baseURL := defaultEndpointBaseURL(endpoint, "http://localhost:11434")
	report.BaseURL = baseURL
	models, err := c.getModelIDs(ctx, http.MethodGet, baseURL+"/api/tags", nil, "models")
	if err != nil {
		report.add(HealthCheck{Name: "running", Status: HealthFail, Detail: err.Error(), Action: "start Ollama with `ollama serve`"})
	} else {
		report.add(HealthCheck{Name: "running", Status: HealthOK, Detail: "Ollama metadata endpoint responded"})
		report.add(modelHealthCheck(endpoint.Model, models, "ollama pull "+endpoint.Model))
	}
	report.add(HealthCheck{Name: "auth", Status: HealthOK, Detail: "no API key required for local Ollama"})
	report.add(HealthCheck{Name: "schema", Status: HealthUnknown, Detail: "schema support requires a smoke chat check"})
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

func (c EndpointHealthChecker) checkCLI(ctx context.Context, endpoint EndpointConfig, report HealthReport) HealthReport {
	bin := cliBinary(endpoint.Transport)
	lookPath := c.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	path, err := lookPath(bin)
	if err != nil {
		report.add(HealthCheck{Name: "installed", Status: HealthFail, Detail: err.Error(), Action: "install " + bin})
		report.add(HealthCheck{Name: "auth", Status: HealthUnknown, Detail: "not checked because binary is missing"})
		report.add(HealthCheck{Name: "model", Status: HealthUnknown, Detail: "not checked because binary is missing"})
		report.add(cliSchemaCheck(endpoint.Transport))
		return report
	}
	report.add(HealthCheck{Name: "installed", Status: HealthOK, Detail: path})
	version, err := c.runCLI(ctx, bin, []string{"--version"})
	if err != nil {
		report.add(HealthCheck{Name: "running", Status: HealthFail, Detail: err.Error()})
	} else {
		report.add(HealthCheck{Name: "running", Status: HealthOK, Detail: strings.TrimSpace(version)})
	}
	report.add(HealthCheck{Name: "auth", Status: HealthUnknown, Detail: "not checked without a model call"})
	if endpoint.Model == "" {
		report.add(HealthCheck{Name: "model", Status: HealthUnknown, Detail: "no model configured"})
	} else {
		report.add(HealthCheck{Name: "model", Status: HealthUnknown, Detail: "model availability is not checked by version probe"})
	}
	report.add(cliSchemaCheck(endpoint.Transport))
	return report
}

func (c EndpointHealthChecker) getModelIDs(ctx context.Context, method, url string, headers map[string]string, field string) (map[string]bool, error) {
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, &statusError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	var items []map[string]any
	if err := json.Unmarshal(doc[field], &items); err != nil {
		return nil, fmt.Errorf("models response missing %q array: %w", field, err)
	}
	models := map[string]bool{}
	for _, item := range items {
		if id, ok := item["id"].(string); ok && id != "" {
			models[id] = true
		}
		if name, ok := item["name"].(string); ok && name != "" {
			models[name] = true
		}
		if model, ok := item["model"].(string); ok && model != "" {
			models[model] = true
		}
	}
	return models, nil
}

func (c EndpointHealthChecker) runCLI(ctx context.Context, command string, args []string) (string, error) {
	runner := c.CLIRunner
	if runner == nil {
		runner = execCLIRunner{}
	}
	result, err := runner.Run(ctx, cliInvocation{Command: command, Args: args})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(result.Stdout) != "" {
		return result.Stdout, nil
	}
	return result.Stderr, nil
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

func cliSchemaCheck(transport string) HealthCheck {
	switch transport {
	case "codex-cli", "claude-cli":
		return HealthCheck{Name: "schema", Status: HealthOK, Detail: "native schema flag configured"}
	case "gemini-cli", "opencode-cli":
		return HealthCheck{Name: "schema", Status: HealthUnknown, Detail: "schema is prompt-enforced, not CLI-enforced"}
	default:
		return HealthCheck{Name: "schema", Status: HealthUnknown}
	}
}

func cliBinary(transport string) string {
	switch transport {
	case "codex-cli":
		return "codex"
	case "gemini-cli":
		return "gemini"
	case "claude-cli":
		return "claude"
	case "opencode-cli":
		return "opencode"
	default:
		return transport
	}
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
