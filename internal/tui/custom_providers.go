package tui

import (
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/config"
	modelprovider "github.com/gongahkia/gator/internal/model"
)

func (m Model) customProvider(name string) (config.CustomProvider, bool) {
	for _, provider := range m.config.CustomProviders {
		if strings.EqualFold(provider.ID, strings.TrimSpace(name)) {
			return provider, true
		}
	}
	return config.CustomProvider{}, false
}

// resolveProviderAndModel keeps the TUI's validation aligned with the CLI
// factory without making the UI depend on a network request.
func (m Model) resolveProviderAndModel(providerName, requestedModel string) (string, string, bool, error) {
	if custom, found := m.customProvider(providerName); found {
		modelName := strings.TrimSpace(requestedModel)
		if modelName == "" {
			modelName = custom.DefaultModel
		}
		for _, configured := range custom.Models {
			if configured == modelName {
				return custom.ID, modelName, true, nil
			}
		}
		return "", "", false, fmt.Errorf("model %q is not configured for custom provider %q", modelName, custom.ID)
	}
	provider, err := modelprovider.ParseProvider(providerName)
	if err != nil {
		return "", "", false, err
	}
	return string(provider), modelprovider.EffectiveModel(provider, requestedModel), false, nil
}

func customProviderOptions(providers []config.CustomProvider) []dropdownOption {
	options := make([]dropdownOption, 0, len(providers))
	for _, provider := range providers {
		description := "custom OpenAI-compatible Chat Completions"
		if provider.APIKeyEnv == "" {
			description += " · no API key"
		} else {
			description += " · API key from " + provider.APIKeyEnv
		}
		options = append(options, dropdownOption{value: provider.ID, label: provider.ID, description: description})
	}
	return options
}
