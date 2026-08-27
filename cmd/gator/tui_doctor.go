package main

import (
	"os"
	"runtime"
	"strings"

	"github.com/gongahkia/gator/internal/dependency"
	"github.com/gongahkia/gator/internal/localmodel"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/tui"
)

type tuiDoctorBackend struct {
	sandbox sandbox.Policy
}

func newTUIDoctorBackend(policy sandbox.Policy) tui.DoctorBackend {
	return &tuiDoctorBackend{sandbox: policy.Normalize()}
}

func (backend *tuiDoctorBackend) Snapshot(providerName string) (tui.DoctorSnapshot, error) {
	settings, err := loadSettings()
	if err != nil {
		return tui.DoctorSnapshot{}, err
	}
	custom, customProvider := configuredCustomProvider(settings, providerName)
	provider := model.Provider("")
	if !customProvider && strings.TrimSpace(providerName) != "" {
		provider, err = model.ParseProvider(providerName)
		if err != nil {
			return tui.DoctorSnapshot{}, err
		}
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return tui.DoctorSnapshot{}, err
	}
	repository, gitErr := gitRepositoryRoot(workingDirectory)
	snapshot := tui.DoctorSnapshot{
		RepositoryDetected: gitErr == nil,
		RepositoryPath:     repository,
		EffectiveSandbox:   string(backend.sandbox.Mode),
		EffectiveNetwork:   string(backend.sandbox.Network),
	}
	if customProvider {
		snapshot.Provider = custom.ID
	} else {
		snapshot.Provider = string(provider)
	}
	snapshot.AuthKind, snapshot.AuthStatus, err = inspectProviderAuth(custom, customProvider, provider)
	if err != nil {
		return tui.DoctorSnapshot{}, err
	}
	sandboxStatus, sandboxErr := sandbox.StrictAvailability()
	if sandboxErr != nil {
		sandboxStatus = "unavailable: " + sandboxErr.Error()
	}
	snapshot.Sandbox = sandboxStatus
	snapshot.WebSearchConfigured = strings.TrimSpace(os.Getenv("BRAVE_SEARCH_API_KEY")) != ""
	if snapshot.WebSearchConfigured {
		snapshot.WebSearchStatus = "configured (requires --network allow)"
	} else {
		snapshot.WebSearchStatus = "not configured (set BRAVE_SEARCH_API_KEY)"
	}
	for _, status := range dependency.Detect() {
		advice := []string(nil)
		if !status.Installed && status.Instructions != nil {
			advice = status.Instructions(runtime.GOOS)
		}
		snapshot.Dependencies = append(snapshot.Dependencies, tui.DoctorDependency{
			Name: status.Name, Installed: status.Installed, Required: status.Required,
			Purpose: status.Purpose, HelpURL: status.HelpURL, Advice: advice,
		})
	}
	host := inspectLocalModelHost()
	summary, _ := localModelHostSummary(host)
	snapshot.LocalHost = summary
	for _, catalogModel := range localmodel.Catalog() {
		eligibility := localmodel.Assess(catalogModel, host)
		snapshot.LocalModels = append(snapshot.LocalModels, tui.DoctorLocalModel{
			ID: catalogModel.ID, Allowed: eligibility.Allowed, Reason: eligibility.Reason, Needs: localModelRequirement(eligibility),
		})
	}
	suggestionDirectory := workingDirectory
	if snapshot.RepositoryDetected {
		suggestionDirectory = repository
	}
	snapshot.SuggestedVerify = suggestedVerificationCommands(suggestionDirectory)
	return snapshot, nil
}
