package tui

import "github.com/gongahkia/gator/internal/modelcatalog"

// ModelCatalogPanel is retained as a compatibility alias for callers that
// have not yet moved from the historical TUI package. New Work code should
// depend on modelcatalog directly.
type ModelCatalogPanel = modelcatalog.ModelCatalogPanel

// NewModelCatalogPanel adapts the historical broad Config to the focused
// model-catalog boundary. Composer, execution, review, worktree, and draft
// state deliberately do not cross this adapter.
func NewModelCatalogPanel(config Config) *modelcatalog.ModelCatalogPanel {
	catalogConfig := modelcatalog.Config{
		Provider:           config.Provider,
		Model:              config.Model,
		BaseURL:            config.BaseURL,
		StateDir:           config.StateDir,
		Theme:              config.Theme,
		CustomProviders:    config.CustomProviders,
		ModelAliases:       config.ModelAliases,
		ProviderEndpoints:  config.ProviderEndpoints,
		ProviderOptions:    config.ProviderOptions,
		LocalModels:        config.LocalModels,
		ModelManagement:    config.ModelManagement,
		SaveModelSelection: config.SaveModelSelection,
		SaveCloudModel:     config.SaveCloudModel,
	}
	if config.BeginOAuthLogin != nil {
		catalogConfig.BeginOAuthLogin = func(provider string) (modelcatalog.OAuthLogin, error) {
			return config.BeginOAuthLogin(provider)
		}
	}
	return modelcatalog.NewModelCatalogPanel(catalogConfig)
}
