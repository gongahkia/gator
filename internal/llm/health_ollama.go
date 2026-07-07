package llm

import "context"

func (c EndpointHealthChecker) checkOllama(ctx context.Context, endpoint EndpointConfig, report HealthReport) HealthReport {
	baseURL := defaultEndpointBaseURL(endpoint, "http://localhost:11434")
	report.BaseURL = baseURL
	models, err := c.ListOllamaModels(ctx, baseURL)
	modelCheck := HealthCheck{Name: "model", Status: HealthUnknown, Detail: "not checked because Ollama metadata failed"}
	if err != nil {
		report.add(HealthCheck{Name: "running", Status: HealthFail, Detail: err.Error(), Action: "start Ollama with `ollama serve`"})
	} else {
		report.add(HealthCheck{Name: "running", Status: HealthOK, Detail: "Ollama metadata endpoint responded"})
		modelCheck = c.ollamaModelHealth(ctx, baseURL, endpoint.Model, models)
		report.add(modelCheck)
	}
	report.add(HealthCheck{Name: "auth", Status: HealthOK, Detail: "no API key required for local Ollama"})
	report.add(c.ollamaSchemaHealth(ctx, baseURL, endpoint.Model, modelCheck))
	return report
}

func (c EndpointHealthChecker) ollamaModelHealth(ctx context.Context, baseURL, model string, models []ModelInfo) HealthCheck {
	check := modelHealthCheck(model, modelSet(models), "ollama pull "+model)
	if check.Status != HealthFail || !c.AutoPullOllama || model == "" {
		return check
	}
	if err := c.PullOllamaModel(ctx, baseURL, model); err != nil {
		check.Detail = "auto-pull failed for " + model + ": " + err.Error()
		return check
	}
	return HealthCheck{Name: "model", Status: HealthOK, Detail: "pulled " + model}
}

func (c EndpointHealthChecker) ollamaSchemaHealth(ctx context.Context, baseURL, model string, modelCheck HealthCheck) HealthCheck {
	if !c.SchemaSmokeOllama {
		return HealthCheck{Name: "schema", Status: HealthUnknown, Detail: "schema support requires a smoke chat check"}
	}
	if model == "" {
		return HealthCheck{Name: "schema", Status: HealthUnknown, Detail: "not checked because no model is configured"}
	}
	if modelCheck.Status != HealthOK {
		return HealthCheck{Name: "schema", Status: HealthUnknown, Detail: "not checked because model is unavailable"}
	}
	if err := c.SmokeCheckOllamaSchema(ctx, baseURL, model); err != nil {
		return HealthCheck{Name: "schema", Status: HealthFail, Detail: err.Error()}
	}
	return HealthCheck{Name: "schema", Status: HealthOK, Detail: "Ollama JSON Schema smoke check passed"}
}
