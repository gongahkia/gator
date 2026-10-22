package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/modelcatalog"
	"github.com/gongahkia/gator/internal/workapp"
)

func workModelPanel(store config.Store, stateDir string, local modelcatalog.LocalManager) func() (workapp.ModelPanel, error) {
	return func() (workapp.ModelPanel, error) {
		settings, err := store.Load()
		if err != nil {
			return nil, err
		}
		defaults := workModelDefaults{settings: store}
		return modelcatalog.NewModelCatalogPanel(modelcatalog.Config{
			StateDir: stateDir,
			Provider: settings.Defaults.Provider, Model: settings.Defaults.Model,
			CustomProviders: settings.CustomProviders, ModelAliases: settings.ModelAliases,
			ProviderEndpoints: settings.ProviderEndpoints, ProviderOptions: settings.ProviderOptions,
			Theme: settings.Theme, LocalModels: local,
			SaveModelSelection: defaults.SetDefaults,
			SaveCloudModel:     saveTUICloudModelConfiguration(store, stateDir),
			ModelManagement:    newTUIModelManagementBackend(store, stateDir),
		}), nil
	}
}

// workModelDefaults is the small settings callback needed by the Work TUI's
// model picker. The retired Code TUI management backend carried this alongside
// unrelated run/session administration.
type workModelDefaults struct{ settings config.Store }

func (defaults workModelDefaults) SetDefaults(provider, modelName string) error {
	provider = strings.TrimSpace(provider)
	modelName = strings.TrimSpace(modelName)
	if provider == "" || len(provider) > 128 || strings.ContainsAny(provider, "\r\n") {
		return errors.New("default provider is invalid")
	}
	if len(modelName) > 512 || strings.ContainsAny(modelName, "\r\n") {
		return errors.New("default model is invalid")
	}
	return defaults.settings.Update(func(settings *config.Settings) error {
		if custom, found := configuredCustomProvider(*settings, provider); found {
			if modelName == "" {
				modelName = custom.DefaultModel
			}
			if !customProviderSupportsModel(custom, modelName) {
				return fmt.Errorf("model %q is not configured for custom provider %q", modelName, custom.ID)
			}
			provider = custom.ID
		} else {
			parsed, err := model.ParseProvider(provider)
			if err != nil {
				return err
			}
			provider = string(parsed)
			modelName = model.EffectiveModel(parsed, modelName)
		}
		settings.Defaults.Provider = provider
		settings.Defaults.Model = modelName
		return nil
	})
}
