package modelcatalog

import (
	"strings"
)

func (m Model) inlineWidth() int {
	if m.width <= 0 {
		return 68
	}
	return min(72, max(16, m.width-4))
}

func (m Model) fieldView(label, hint, value string) string {
	sections := []string{labelStyle.Render(label)}
	if hint != "" {
		sections = append(sections, dimStyle.Render(compact(hint, m.inlineWidth())))
	}
	if value != "" {
		sections = append(sections, value)
	}
	return strings.Join(sections, "\n")
}

func (m Model) visibleRange(total, selected, limit int) (int, int) {
	if total == 0 || limit <= 0 {
		return 0, 0
	}
	limit = min(limit, total)
	selected = min(max(0, selected), total-1)
	start := selected - limit/2
	start = max(0, min(start, total-limit))
	return start, start + limit
}

func (m Model) modelCatalogConfirmationView() string {
	switch m.localModels.confirmation {
	case localModelConfirmStart:
		return m.fieldView("Start Ollama?", "Gator can start 'ollama serve' as a child of this TUI and stops it when Gator exits. You can instead start it yourself.", "Enter/y  Start with Gator (default)\nn/esc  I'll start it myself")
	case localModelConfirmInstall:
		return m.fieldView("Install Ollama?", "Ollama is not installed. Gator can show the official source and platform advice; it never runs a system installer or package manager.", "Enter/y  Open installation help (default)\nn/esc  I'll install it myself")
	case localModelConfirmPull:
		if selected, found := m.selectedLocalModel(); found {
			return m.fieldView("Confirm download", "Model weights and upstream terms remain governed by the linked source.", selected.Name+" · approximately "+selected.Download+"\n"+selected.SourceURL)
		}
	case localModelConfirmRemove:
		if selected, found := m.selectedLocalModel(); found {
			return m.fieldView("Confirm removal", "This deletes local model data from the selected Ollama runtime.", selected.Name+" · "+selected.OllamaModel)
		}
	case localModelConfirmRemoveCredential:
		if cloud, found := m.selectedCloudModel(); found {
			status := m.storedCredential(cloud.provider)
			detail := cloud.provider + " · stored " + status.Kind + "\nOnly Gator's private credential file is changed. Environment variables, AWS/ADC, one-run keys, and vendor CLI logins remain."
			if cloud.provider == "claude" {
				detail = "Claude Code · stored Anthropic API key\nThis removes the Gator-owned Anthropic key. ANTHROPIC_API_KEY in the environment is not unset."
			}
			return m.fieldView("Remove stored Gator credential?", "This does not revoke the upstream key and does not log you out of vendor CLIs.", detail)
		}
	case localModelConfirmRemoveCustom:
		if cloud, found := m.selectedCloudModel(); found {
			return m.fieldView("Remove custom provider?", "This deletes endpoint metadata from Gator config.json. The named API key environment variable is not changed or unset.", cloud.provider)
		}
	case localModelConfirmSaveCustom:
		if m.localModels.pendingCustom != nil {
			return m.fieldView("Save custom provider?", "Review the destination before writing non-secret metadata. No API key is stored.", customProviderReviewText(*m.localModels.pendingCustom))
		}
	case localModelConfirmApplyDiscovery:
		if m.localModels.discovery != nil {
			preview := *m.localModels.discovery
			limit := min(12, len(preview.Models))
			return m.fieldView("Replace configured models?", "These IDs came from an untrusted /models response. Confirming overwrites the configured catalog.", preview.ID+" default "+preview.DefaultModel+"\n"+strings.Join(preview.Models[:limit], "\n"))
		}
	}
	return ""
}

func (m Model) localDependencyHelpView() string {
	missing := m.localMissingDependencies()
	if len(missing) == 0 {
		return ""
	}
	lines := make([]string, 0, len(missing)*3)
	for _, dependency := range missing {
		role := "optional"
		if dependency.Required {
			role = "required"
		}
		lines = append(lines, dependency.Name+" · "+role+" for "+dependency.Purpose)
		if dependency.HelpURL != "" {
			lines = append(lines, "  Official source: "+dependency.HelpURL)
		}
		for _, instruction := range dependency.Instructions {
			lines = append(lines, "  "+instruction)
		}
	}
	return m.fieldView("Installation help", "Gator detected these prerequisites as missing. It provides checked-in guidance but does not execute system installers or package managers.", strings.Join(lines, "\n"))
}

func compact(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	if limit == 1 {
		return "…"
	}
	return value[:limit-1] + "…"
}
