package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/model"
	gatorrun "github.com/gongahkia/gator/internal/run"
)

func providerFromEnvironment() (model.Provider, error) {
	provider := os.Getenv("GATOR_PROVIDER")
	if provider == "" {
		provider = string(model.OpenAI)
	}
	parsed, err := model.ParseProvider(provider)
	if err != nil {
		return "", fmt.Errorf("GATOR_PROVIDER: %w", err)
	}
	return parsed, nil
}

func modelFromEnvironment(provider model.Provider) string {
	if model := os.Getenv("GATOR_MODEL"); model != "" {
		return model
	}
	return model.DefaultModel(provider)
}

func newExecutor(providerName, modelName, baseURL string) (gatorrun.Executor, error) {
	credentials, err := gatorCredentials()
	if err != nil {
		return gatorrun.Executor{}, err
	}
	backend, err := model.New(model.Config{Provider: model.Provider(providerName), Model: modelName, BaseURL: baseURL, Credentials: &credentials})
	if err != nil {
		return gatorrun.Executor{}, err
	}
	return gatorrun.Executor{Model: backend.Model}, nil
}

func gatorCredentials() (auth.Store, error) {
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return auth.Store{}, err
	}
	return auth.New(stateDir)
}

func displayModel(value string) string {
	if strings.TrimSpace(value) == "" {
		return "provider default"
	}
	return value
}
