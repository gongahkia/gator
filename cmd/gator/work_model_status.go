package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/localmodel"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/worktui"
)

// currentWorkModelStatus resolves only display-safe readiness metadata. It
// never returns credential values or reads credential stores owned by another
// application.
func currentWorkModelStatus(settingsStore config.Store, stateDir string) (worktui.ModelStatus, error) {
	settings, err := settingsStore.Load()
	if err != nil {
		return worktui.ModelStatus{}, err
	}
	status := worktui.ModelStatus{
		Provider: strings.TrimSpace(settings.Defaults.Provider),
		Model:    strings.TrimSpace(settings.Defaults.Model),
	}
	if status.Provider == "" {
		status.Access = "not configured"
		return status, nil
	}

	if custom, found := configuredCustomProvider(settings, status.Provider); found {
		status.Access = customProviderAccess(custom, status.Model)
		return status, nil
	}
	provider, err := model.ParseProvider(status.Provider)
	if err != nil {
		return status, err
	}
	credentials, err := auth.New(stateDir)
	if err != nil {
		return status, err
	}
	credential, found, err := credentials.Read(gatorCredentialStoreKey(provider))
	if err != nil {
		return status, err
	}
	if found {
		kind := strings.ToLower(credentialKindLabel(credential))
		if credential.Expired(time.Now()) {
			status.Access = "stored " + kind + " expired"
		} else if credential.IsOAuth() {
			status.Access = "logged in (Gator OAuth credential)"
		} else {
			status.Access = "authenticated with Gator " + kind
		}
		return status, nil
	}
	if sources := remainingCredentialSources(provider); len(sources) > 0 {
		status.Access = "authenticated via " + strings.Join(sources, ", ")
		return status, nil
	}
	if model.RequiresOAuthLogin(provider) {
		status.Access = "not logged in"
	} else {
		status.Access = "not authenticated; requires " + model.CredentialHint(provider)
	}
	return status, nil
}

func customProviderAccess(provider config.CustomProvider, selectedModel string) string {
	if provider.ID == localmodel.ProviderID {
		if selectedModel != "" && customProviderSupportsModel(provider, selectedModel) {
			return "local model configured"
		}
		return "local model selection is incomplete"
	}
	if provider.APIKeyEnv == "" {
		return "custom endpoint configured (keyless)"
	}
	if strings.TrimSpace(os.Getenv(provider.APIKeyEnv)) != "" {
		return "authenticated via " + provider.APIKeyEnv
	}
	return fmt.Sprintf("custom endpoint configured; %s is not set", provider.APIKeyEnv)
}
