package worktui

import (
	"strings"

	"github.com/gongahkia/gator/internal/localmodel"
)

// explainRunFailure keeps provider failures useful without hiding the original
// technical detail. Local-runtime failures receive a concrete recovery path
// because the Work model cannot service a request until that runtime is up.
func explainRunFailure(runError string, status ModelStatus) string {
	detail := strings.TrimSpace(runError)
	lower := strings.ToLower(detail)
	provider := strings.TrimSpace(status.Provider)
	model := strings.TrimSpace(status.Model)
	selected := provider
	if model != "" {
		selected += "/" + model
	}
	if selected == "" {
		selected = "the selected model"
	}

	if provider == localmodel.ProviderID || strings.Contains(lower, "custom provider gator-local") {
		switch {
		case strings.Contains(lower, "connection refused"):
			return "Local Ollama connection refused for " + selected + " at 127.0.0.1:11434.\n" +
				"Detail: " + detail + "\n" +
				"Recovery: open /model, choose Local, and press s. When it shows Connected, press r, then resend. Or run: ollama serve\n" +
				"Check: ollama ps; curl -fsS http://127.0.0.1:11434/api/version"
		case strings.Contains(lower, "deadline exceeded"), strings.Contains(lower, "timeout"):
			return "Local Ollama did not respond for " + selected + " at 127.0.0.1:11434.\n" +
				"Detail: " + detail + "\n" +
				"Recovery: confirm Ollama is running and the machine has enough memory for the selected model. Check: ollama ps"
		}
	}

	if strings.Contains(lower, "connection refused") {
		return "Model endpoint connection refused for " + selected + ".\n" +
			"Detail: " + detail + "\n" +
			"Recovery: open /model to verify the selected provider and endpoint, then make sure its service is running and reachable."
	}
	if strings.Contains(lower, "unknown provider") || strings.Contains(lower, "no direct gator") {
		return "The selected model provider is no longer available.\n" +
			"Detail: " + detail + "\n" +
			"Recovery: open /model and choose Cloud API key or Local model before sending the request again."
	}
	if strings.Contains(lower, "unauthorized") || strings.Contains(lower, "forbidden") || strings.Contains(lower, "status 401") || strings.Contains(lower, "status 403") {
		return "Model authentication failed for " + selected + ".\n" +
			"Detail: " + detail + "\n" +
			"Recovery: open /model, choose Cloud API key, and update its API key, then resend your request."
	}
	return "I couldn't finish that run: " + detail
}
