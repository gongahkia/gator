package main

import (
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/modelcatalog"
	"github.com/gongahkia/gator/internal/worktui"
)

func workModelPanel(store config.Store, stateDir string, local modelcatalog.LocalManager) func() (worktui.ModelPanel, error) {
	return func() (worktui.ModelPanel, error) {
		settings, err := store.Load()
		if err != nil {
			return nil, err
		}
		defaults := &tuiManagementBackend{settings: store}
		return modelcatalog.NewModelCatalogPanel(modelcatalog.Config{
			StateDir: stateDir,
			Provider: settings.Defaults.Provider, Model: settings.Defaults.Model,
			CustomProviders: settings.CustomProviders, ModelAliases: settings.ModelAliases,
			ProviderEndpoints: settings.ProviderEndpoints, ProviderOptions: settings.ProviderOptions,
			Theme: settings.Theme, LocalModels: local,
			SaveModelSelection: defaults.SetDefaults,
			SaveCloudModel:     saveTUICloudModelConfiguration(store, stateDir),
			ModelManagement:    newTUIModelManagementBackend(store, stateDir),
			BeginOAuthLogin: func(provider string) (modelcatalog.OAuthLogin, error) {
				return beginTUIOAuthLogin(provider)
			},
		}), nil
	}
}
