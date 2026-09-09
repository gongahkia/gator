package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/dependency"
	"github.com/gongahkia/gator/internal/localmodel"
	"github.com/gongahkia/gator/internal/tui"
)

// localModelManager is the application layer behind TUI model management.
// It keeps the UI independent of Ollama's HTTP API while using
// the same curated catalog, loopback validation, and provider configuration.
type localModelManager struct {
	store      config.Store
	runtimeURL string
	host       func() localmodel.Host
	runtimeMu  sync.Mutex
	runtime    *managedLocalRuntime
	closed     bool
	command    func(string, ...string) *exec.Cmd
	lookPath   func(string) (string, error)
}

type managedLocalRuntime struct {
	command *exec.Cmd
	done    chan struct{}

	mu      sync.Mutex
	exitErr error
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
	host := manager.localHost()
	catalog := manager.catalog(settings, client, nil, host)
	if binary, lookupErr := manager.ollamaPath(); lookupErr == nil {
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
	catalog.Models = manager.catalog(settings, client, installed, host).Models
	return catalog, nil
}

// Start launches `ollama serve` only after the TUI receives explicit user
// confirmation. The child is retained by this interactive Gator session and
// stopped by Close; an already-running Ollama process is never adopted or
// stopped.
func (manager *localModelManager) Start(ctx context.Context) (tui.LocalModelCatalog, error) {
	settings, err := manager.store.Load()
	if err != nil {
		return tui.LocalModelCatalog{}, err
	}
	client, err := localClient(manager.runtimeURL, settings)
	if err != nil {
		return tui.LocalModelCatalog{}, err
	}
	if runtimeReachable(ctx, client) {
		return manager.Status(ctx)
	}
	binary, err := manager.ollamaPath()
	if err != nil {
		return tui.LocalModelCatalog{}, errors.New("Ollama executable was not found; open installation help with i in /model's Local section")
	}
	runtime, err := manager.startRuntime(binary)
	if err != nil {
		return tui.LocalModelCatalog{}, err
	}
	readyContext, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := waitForLocalRuntime(readyContext, client, runtime); err != nil {
		_ = manager.stopRuntime(runtime)
		return tui.LocalModelCatalog{}, err
	}
	return manager.Status(ctx)
}

// Close stops only the Ollama child that this manager launched. It is called
// when the interactive TUI exits, preserving a user-managed Ollama process.
func (manager *localModelManager) Close() error {
	manager.runtimeMu.Lock()
	runtime := manager.runtime
	manager.runtime = nil
	manager.closed = true
	manager.runtimeMu.Unlock()
	return manager.stopRuntime(runtime)
}

func (manager *localModelManager) stopRuntime(runtime *managedLocalRuntime) error {
	if runtime == nil || runtime.command.Process == nil {
		return nil
	}
	select {
	case <-runtime.done:
		return nil
	default:
	}
	if err := runtime.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("stop Gator-managed Ollama runtime: %w", err)
	}
	select {
	case <-runtime.done:
		return nil
	case <-time.After(2 * time.Second):
		return errors.New("timed out stopping Gator-managed Ollama runtime")
	}
}

func (manager *localModelManager) ollamaPath() (string, error) {
	if manager.lookPath != nil {
		return manager.lookPath("ollama")
	}
	return exec.LookPath("ollama")
}

func (manager *localModelManager) startRuntime(binary string) (*managedLocalRuntime, error) {
	manager.runtimeMu.Lock()
	defer manager.runtimeMu.Unlock()
	if manager.closed {
		return nil, errors.New("local model management session is closed")
	}
	if manager.runtime != nil {
		return manager.runtime, nil
	}
	newCommand := manager.command
	if newCommand == nil {
		newCommand = exec.Command
	}
	command := newCommand(binary, "serve")
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start Ollama: %w", err)
	}
	runtime := &managedLocalRuntime{command: command, done: make(chan struct{})}
	manager.runtime = runtime
	go manager.waitForManagedRuntime(runtime)
	return runtime, nil
}

func (manager *localModelManager) waitForManagedRuntime(runtime *managedLocalRuntime) {
	err := runtime.command.Wait()
	runtime.mu.Lock()
	runtime.exitErr = err
	runtime.mu.Unlock()
	close(runtime.done)
	manager.runtimeMu.Lock()
	if manager.runtime == runtime {
		manager.runtime = nil
	}
	manager.runtimeMu.Unlock()
}

func (runtime *managedLocalRuntime) error() error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.exitErr
}

func runtimeReachable(parent context.Context, client localmodel.Client) bool {
	context, cancel := context.WithTimeout(parent, time.Second)
	defer cancel()
	_, err := client.Version(context)
	return err == nil
}

func waitForLocalRuntime(ctx context.Context, client localmodel.Client, runtime *managedLocalRuntime) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		probeContext, cancel := context.WithTimeout(ctx, time.Second)
		_, lastErr = client.Version(probeContext)
		cancel()
		if lastErr == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("wait for Ollama to start: %w", lastErr)
			}
			return fmt.Errorf("wait for Ollama to start: %w", ctx.Err())
		case <-runtime.done:
			if err := runtime.error(); err != nil {
				return fmt.Errorf("Ollama exited before becoming ready: %w", err)
			}
			return errors.New("Ollama exited before becoming ready")
		case <-ticker.C:
		}
	}
}

func (manager *localModelManager) Pull(ctx context.Context, id string, report func(tui.LocalModelProgress)) (tui.LocalModelCatalog, error) {
	model, found := localmodel.Resolve(id)
	if !found {
		return tui.LocalModelCatalog{}, fmt.Errorf("%q is not in Gator's curated local-model catalog", id)
	}
	if eligibility := localmodel.Assess(model, manager.localHost()); !eligibility.Allowed {
		return tui.LocalModelCatalog{}, fmt.Errorf("%s is disabled on this host: %s", model.Name, eligibility.Reason)
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
	if eligibility := localmodel.Assess(model, manager.localHost()); !eligibility.Allowed {
		return tui.LocalModelUpdate{}, fmt.Errorf("%s is disabled on this host: %s", model.Name, eligibility.Reason)
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
		ModelAliases:    cloneModelAliases(settings.ModelAliases),
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
		ModelAliases:    cloneModelAliases(settings.ModelAliases),
		Provider:        settings.Defaults.Provider,
		Model:           settings.Defaults.Model,
	}, nil
}

func (manager *localModelManager) Rename(ctx context.Context, provider, model, alias string) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	alias = strings.TrimSpace(alias)
	if provider == "" || model == "" {
		return nil, fmt.Errorf("provider and model are required to rename a display label")
	}
	settings, err := manager.store.Load()
	if err != nil {
		return nil, err
	}
	if settings.ModelAliases == nil {
		settings.ModelAliases = make(map[string]string)
	}
	key := config.ModelAliasKey(provider, model)
	if alias == "" {
		delete(settings.ModelAliases, key)
	} else {
		settings.ModelAliases[key] = alias
	}
	if err := manager.store.Save(settings); err != nil {
		return nil, err
	}
	return cloneModelAliases(settings.ModelAliases), nil
}

func (manager *localModelManager) catalog(settings config.Settings, client localmodel.Client, installed []localmodel.InstalledModel, host localmodel.Host) tui.LocalModelCatalog {
	installedNames := installedModelNames(installed)
	models := make([]tui.LocalModel, 0, len(localmodel.Catalog()))
	for _, model := range localmodel.Catalog() {
		_, isInstalled := installedNames[model.OllamaModel]
		eligibility := localmodel.Assess(model, host)
		models = append(models, tui.LocalModel{
			ID:            model.ID,
			OllamaModel:   model.OllamaModel,
			Name:          modelDisplayName(settings, localmodel.ProviderID, model.OllamaModel, model.Name),
			DefaultName:   model.Name,
			Download:      model.Download,
			Context:       model.Context,
			Summary:       model.Summary,
			SourceURL:     model.SourceURL,
			Installed:     isInstalled,
			Requirement:   localModelRequirement(eligibility),
			BlockedReason: eligibility.Reason,
		})
	}
	hostSummary, hostAdvice := localModelHostSummary(host)
	return tui.LocalModelCatalog{RuntimeURL: client.BaseURL(), HostSummary: hostSummary, HostAdvice: hostAdvice, Dependencies: localModelDependencies(manager), Models: models}
}

func localModelDependencies(manager *localModelManager) []tui.LocalDependency {
	statuses := dependency.Detect()
	dependencies := make([]tui.LocalDependency, 0, len(statuses))
	for _, status := range statuses {
		installed := status.Installed
		if status.ID == "ollama" {
			_, err := manager.ollamaPath()
			installed = err == nil
		}
		instructions := []string(nil)
		if status.Instructions != nil {
			instructions = status.Instructions(runtime.GOOS)
		}
		dependencies = append(dependencies, tui.LocalDependency{
			ID:           status.ID,
			Name:         status.Name,
			Purpose:      status.Purpose,
			Required:     status.Required,
			Installed:    installed,
			HelpURL:      status.HelpURL,
			Instructions: instructions,
		})
	}
	return dependencies
}

func (manager *localModelManager) localHost() localmodel.Host {
	if manager.host != nil {
		return manager.host()
	}
	return inspectLocalModelHost()
}

func cloneCustomProviders(providers []config.CustomProvider) []config.CustomProvider {
	result := make([]config.CustomProvider, 0, len(providers))
	for _, provider := range providers {
		provider.Models = append([]string(nil), provider.Models...)
		result = append(result, provider)
	}
	return result
}

func cloneModelAliases(aliases map[string]string) map[string]string {
	result := make(map[string]string, len(aliases))
	for key, value := range aliases {
		result[key] = value
	}
	return result
}

func modelDisplayName(settings config.Settings, provider, model, fallback string) string {
	if alias := strings.TrimSpace(settings.ModelAliases[config.ModelAliasKey(provider, model)]); alias != "" {
		return alias
	}
	return fallback
}

var _ tui.LocalModelManager = (*localModelManager)(nil)
