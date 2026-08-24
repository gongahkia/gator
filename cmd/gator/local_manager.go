package main

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/localmodel"
	"github.com/gongahkia/gator/internal/tui"
)

// localModelManager is the shared application layer behind the native local
// CLI and TUI. It keeps the UI independent of Ollama's HTTP API while using
// the same curated catalog, loopback validation, and provider configuration.
type localModelManager struct {
	store      config.Store
	runtimeURL string
}

func newLocalModelManager(store config.Store) *localModelManager {
	return &localModelManager{store: store}
}

func (manager *localModelManager) Status(ctx context.Context) (tui.LocalModelCatalog, error) {
	settings, err := manager.store.Load()
	if err != nil {
		return tui.LocalModelCatalog{}, err
	}
	client, err := localClient(manager.runtimeURL, settings)
	if err != nil {
		return tui.LocalModelCatalog{}, err
	}
	catalog := manager.catalog(client, nil)
	if binary, lookupErr := exec.LookPath("ollama"); lookupErr == nil {
		catalog.Executable = binary
	}
	version, err := client.Version(ctx)
	if err != nil {
		catalog.RuntimeError = err.Error()
		return catalog, nil
	}
	catalog.RuntimeVersion = version
	installed, err := client.Models(ctx)
	if err != nil {
		catalog.RuntimeError = "list installed models: " + err.Error()
		return catalog, nil
	}
	catalog.Models = manager.catalog(client, installed).Models
	return catalog, nil
}

func (manager *localModelManager) Pull(ctx context.Context, id string, report func(tui.LocalModelProgress)) (tui.LocalModelCatalog, error) {
	model, found := localmodel.Resolve(id)
	if !found {
		return tui.LocalModelCatalog{}, fmt.Errorf("%q is not in Gator's curated local-model catalog", id)
	}
	settings, err := manager.store.Load()
	if err != nil {
		return tui.LocalModelCatalog{}, err
	}
	client, err := localClient(manager.runtimeURL, settings)
	if err != nil {
		return tui.LocalModelCatalog{}, err
	}
	if _, err := client.Version(ctx); err != nil {
		return tui.LocalModelCatalog{}, err
	}
	if err := client.Pull(ctx, model, func(progress localmodel.Progress) {
		if report == nil {
			return
		}
		report(tui.LocalModelProgress{
			Status:    safeLocalDisplay(progress.Status),
			Completed: progress.Completed,
			Total:     progress.Total,
		})
	}); err != nil {
		return tui.LocalModelCatalog{}, err
	}
	return manager.Status(ctx)
}

func (manager *localModelManager) Use(ctx context.Context, id string) (tui.LocalModelUpdate, error) {
	model, found := localmodel.Resolve(id)
	if !found {
		return tui.LocalModelUpdate{}, fmt.Errorf("%q is not in Gator's curated local-model catalog", id)
	}
	settings, err := manager.store.Load()
	if err != nil {
		return tui.LocalModelUpdate{}, err
	}
	client, err := localClient(manager.runtimeURL, settings)
	if err != nil {
		return tui.LocalModelUpdate{}, err
	}
	installed, err := client.Models(ctx)
	if err != nil {
		return tui.LocalModelUpdate{}, err
	}
	if !hasInstalledModel(installed, model.OllamaModel) {
		return tui.LocalModelUpdate{}, fmt.Errorf("%s is not installed; pull it first", model.OllamaModel)
	}
	settings.CustomProviders = setCustomProvider(settings.CustomProviders, config.CustomProvider{
		ID:           localmodel.ProviderID,
		BaseURL:      client.ChatCompletionsURL(),
		Models:       installedCatalogModels(installed),
		DefaultModel: model.OllamaModel,
	})
	settings.Defaults.Provider = localmodel.ProviderID
	settings.Defaults.Model = model.OllamaModel
	if err := manager.store.Save(settings); err != nil {
		return tui.LocalModelUpdate{}, err
	}
	catalog, err := manager.Status(ctx)
	if err != nil {
		return tui.LocalModelUpdate{}, err
	}
	return tui.LocalModelUpdate{
		Catalog:         catalog,
		CustomProviders: cloneCustomProviders(settings.CustomProviders),
		Provider:        settings.Defaults.Provider,
		Model:           settings.Defaults.Model,
	}, nil
}

func (manager *localModelManager) Remove(ctx context.Context, id string) (tui.LocalModelUpdate, error) {
	model, found := localmodel.Resolve(id)
	if !found {
		return tui.LocalModelUpdate{}, fmt.Errorf("%q is not in Gator's curated local-model catalog", id)
	}
	settings, err := manager.store.Load()
	if err != nil {
		return tui.LocalModelUpdate{}, err
	}
	client, err := localClient(manager.runtimeURL, settings)
	if err != nil {
		return tui.LocalModelUpdate{}, err
	}
	if _, err := client.Version(ctx); err != nil {
		return tui.LocalModelUpdate{}, err
	}
	if err := client.Remove(ctx, model); err != nil {
		return tui.LocalModelUpdate{}, err
	}
	if localProvider, found := configuredCustomProvider(settings, localmodel.ProviderID); found {
		localProvider.Models = removeLocalProviderModel(localProvider.Models, model.OllamaModel)
		if len(localProvider.Models) == 0 {
			settings.CustomProviders = removeCustomProvider(settings.CustomProviders, localmodel.ProviderID)
			if settings.Defaults.Provider == localmodel.ProviderID {
				settings.Defaults.Provider = ""
				settings.Defaults.Model = ""
			}
		} else {
			if localProvider.DefaultModel == model.OllamaModel {
				localProvider.DefaultModel = localProvider.Models[0]
			}
			settings.CustomProviders = setCustomProvider(settings.CustomProviders, localProvider)
			if settings.Defaults.Provider == localmodel.ProviderID && settings.Defaults.Model == model.OllamaModel {
				settings.Defaults.Model = localProvider.DefaultModel
			}
		}
		if err := manager.store.Save(settings); err != nil {
			return tui.LocalModelUpdate{}, fmt.Errorf("removed %s but could not update Gator's local provider configuration: %w", model.OllamaModel, err)
		}
	}
	catalog, err := manager.Status(ctx)
	if err != nil {
		return tui.LocalModelUpdate{}, err
	}
	return tui.LocalModelUpdate{
		Catalog:         catalog,
		CustomProviders: cloneCustomProviders(settings.CustomProviders),
		Provider:        settings.Defaults.Provider,
		Model:           settings.Defaults.Model,
	}, nil
}

func (manager *localModelManager) catalog(client localmodel.Client, installed []localmodel.InstalledModel) tui.LocalModelCatalog {
	installedNames := installedModelNames(installed)
	models := make([]tui.LocalModel, 0, len(localmodel.Catalog()))
	for _, model := range localmodel.Catalog() {
		_, isInstalled := installedNames[model.OllamaModel]
		models = append(models, tui.LocalModel{
			ID:          model.ID,
			OllamaModel: model.OllamaModel,
			Name:        model.Name,
			Download:    model.Download,
			Context:     model.Context,
			Summary:     model.Summary,
			SourceURL:   model.SourceURL,
			Installed:   isInstalled,
		})
	}
	return tui.LocalModelCatalog{RuntimeURL: client.BaseURL(), Models: models}
}

func cloneCustomProviders(providers []config.CustomProvider) []config.CustomProvider {
	result := make([]config.CustomProvider, 0, len(providers))
	for _, provider := range providers {
		provider.Models = append([]string(nil), provider.Models...)
		result = append(result, provider)
	}
	return result
}

var _ tui.LocalModelManager = (*localModelManager)(nil)
