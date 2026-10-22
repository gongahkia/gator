package main

import (
	"os"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/workapp"
)

// Read only Gator's configured providers. Discovery, credentials and downloads
// stay in model management; opening the composer never starts network activity.
func workComposerModels(store config.Store, stateDir string) ([]workapp.ComposerModel, error) {
	settings, err := store.Load()
	if err != nil {
		return nil, err
	}
	credentials, err := auth.New(stateDir)
	if err != nil {
		return nil, err
	}
	choices := []workapp.ComposerModel{}
	add := func(provider, name, unavailable string) {
		for _, choice := range choices {
			if choice.Provider == provider && choice.Model == name {
				return
			}
		}
		label := strings.TrimSpace(settings.ModelAliases[config.ModelAliasKey(provider, name)])
		if label == "" {
			label = name
		}
		choices = append(choices, workapp.ComposerModel{Provider: provider, Model: name, Label: label, Unavailable: unavailable})
	}
	for _, id := range model.Names() {
		provider, err := model.ParseProvider(id)
		if err != nil {
			continue
		}
		ready := len(remainingCredentialSources(provider)) > 0
		credential, found, readErr := credentials.Read(gatorCredentialStoreKey(provider))
		if readErr != nil {
			ready = false
		} else if found {
			ready = credential.IsAPIKey() && !credential.Expired(time.Now())
		}
		if !ready && id != settings.Defaults.Provider {
			continue
		}
		unavailable := ""
		if !ready {
			unavailable = "Set up this provider in Manage models."
		}
		for _, name := range model.CuratedModels(provider) {
			add(id, name, unavailable)
		}
		if id == settings.Defaults.Provider {
			add(id, model.EffectiveModel(provider, settings.Defaults.Model), unavailable)
		}
	}
	for _, provider := range settings.CustomProviders {
		unavailable := ""
		if provider.APIKeyEnv != "" && strings.TrimSpace(os.Getenv(provider.APIKeyEnv)) == "" {
			unavailable = "Set up this provider in Manage models."
		}
		for _, name := range provider.Models {
			add(provider.ID, name, unavailable)
		}
	}
	return choices, nil
}
