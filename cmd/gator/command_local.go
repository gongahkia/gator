package main

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/localmodel"
)

func localClient(override string, settings config.Settings) (localmodel.Client, error) {
	runtimeURL := strings.TrimSpace(override)
	if runtimeURL == "" {
		if provider, found := configuredCustomProvider(settings, localmodel.ProviderID); found {
			var err error
			runtimeURL, err = localRuntimeURL(provider.BaseURL)
			if err != nil {
				return localmodel.Client{}, fmt.Errorf("configured local provider: %w", err)
			}
		}
	}
	return localmodel.NewClient(runtimeURL)
}

func localRuntimeURL(chatCompletionsURL string) (string, error) {
	parsed, err := url.Parse(chatCompletionsURL)
	if err != nil {
		return "", err
	}
	const suffix = "/v1/chat/completions"
	if !strings.HasSuffix(parsed.Path, suffix) {
		return "", errors.New("base URL must end in /v1/chat/completions")
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, suffix)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func installedModelNames(models []localmodel.InstalledModel) map[string]struct{} {
	names := make(map[string]struct{}, len(models))
	for _, model := range models {
		names[model.Name] = struct{}{}
	}
	return names
}

func hasInstalledModel(models []localmodel.InstalledModel, name string) bool {
	_, found := installedModelNames(models)[name]
	return found
}

func installedCatalogModels(installed []localmodel.InstalledModel) []string {
	names := installedModelNames(installed)
	models := make([]string, 0, len(names))
	for _, model := range localmodel.Catalog() {
		if _, found := names[model.OllamaModel]; found {
			models = append(models, model.OllamaModel)
		}
	}
	return models
}

func removeLocalProviderModel(values []string, target string) []string {
	result := values[:0]
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}

func safeLocalDisplay(value string) string {
	value = strings.Map(func(character rune) rune {
		if character < 0x20 || character == 0x7f {
			return -1
		}
		return character
	}, strings.TrimSpace(value))
	if len(value) > 160 {
		return value[:160] + "…"
	}
	return value
}
