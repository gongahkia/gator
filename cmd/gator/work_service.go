package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/orchestrator"
	"github.com/gongahkia/gator/internal/snapshot"
	"github.com/gongahkia/gator/internal/workrun"
	"io"
	"os"
)

// configuredWorkService assembles trusted adapters; execution stays in workrun.Service.
func configuredWorkService(provider, modelName, stateDir string, request *workrun.Request) (workrun.Service, error) {
	defaults, err := configuredDefaults()
	if err != nil {
		return workrun.Service{}, err
	}
	if provider == "" {
		provider = defaults.Provider
	}
	if modelName == "" {
		modelName = defaults.Model
	}
	provider, modelName, err = resolveConfiguredProvider(provider, modelName)
	if err != nil {
		return workrun.Service{}, err
	}
	backend, err := nativeWorkModel(provider, modelName, "")
	if err != nil {
		return workrun.Service{}, err
	}
	settings, err := loadSettings()
	if err != nil {
		return workrun.Service{}, err
	}
	registry, err := connector.NewRegistry(settings.Connectors)
	if err != nil {
		return workrun.Service{}, err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return workrun.Service{}, err
	}
	request.OTLPEndpoint = os.Getenv("GATOR_OTLP_ENDPOINT")
	request.Provider = provider + "/" + modelName
	request.StateDir = stateDir
	request.ConnectorPermissions = connector.PermissionSet(settings.ConnectorPermissions)
	request.SnapshotOptions = snapshot.Options{Limits: snapshot.Limits{MaxFiles: settings.Snapshots.MaxFiles, MaxTotal: settings.Snapshots.MaxTotalBytes, MaxFileBytes: settings.Snapshots.MaxFileBytes}, Excludes: settings.Snapshots.Excludes}
	executor := workrun.Executor{Model: backend, StateDir: stateDir, Connectors: connector.Runtime{Registry: registry, Credentials: credentials}}
	if native, ok := backend.(*nativeWorkBackend); ok {
		executor.Code = native.codeDelegate(stateDir)
	}
	if err := configureWorkRoles(&executor, settings, stateDir); err != nil {
		return workrun.Service{}, err
	}
	return workrun.Service{Executor: executor, Defaults: request}, nil
}

func executeConfiguredWork(ctx context.Context, provider, modelName, stateDir string, request workrun.Request) (workrun.Outcome, error) {
	service, err := configuredWorkService(provider, modelName, stateDir, &request)
	if err != nil {
		return workrun.Outcome{}, err
	}
	return service.Execute(ctx, request)
}

func configureWorkRoles(executor *workrun.Executor, settings config.Settings, state string) error {
	executor.RoleConfiguration = map[string]orchestrator.RoleConfiguration{}
	executor.RoleModels = map[string]agent.Model{}
	executor.RoleSteps = map[string]int{}
	for _, role := range settings.WorkRoles {
		executor.RoleSteps[role.Name] = role.MaxSteps
		configuration := orchestrator.RoleConfiguration{Version: role.Version, MaxSteps: role.MaxSteps}
		if role.Provider != "" {
			configuration.Provider = role.Provider + "/" + role.Model
		}
		executor.RoleConfiguration[role.Name] = configuration
		if role.Provider == "" {
			continue
		}
		model, err := nativeWorkModel(role.Provider, role.Model, "")
		if err != nil {
			return err
		}
		if role.Name == "code" {
			executor.Code = model.(*nativeWorkBackend).codeDelegate(state)
		} else {
			executor.RoleModels[role.Name] = model
		}
	}
	return nil
}

func readWorkJSON(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 1024*1024+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("expected one bounded JSON document")
	}
	return nil
}
