package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/localmodel"
	"github.com/gongahkia/gator/internal/retrieval"
	"github.com/gongahkia/gator/internal/workapp"
)

func workRecallStatus(ctx context.Context, stateDir string) (workapp.RecallStatus, error) {
	settings, err := loadSettings()
	if err != nil {
		return workapp.RecallStatus{}, err
	}
	provider := settings.Embeddings.Provider
	if provider == "" {
		provider = "ollama"
	}
	status := workapp.RecallStatus{Provider: provider, State: "Basic recall available", Storage: "Saved memories, notes, transcripts, and Work records stay in the private local Gator state directory on this computer. Forgetting removes an item from future recall; prior transcripts and retained Work evidence may still contain it."}
	status.LearnFromChats = settings.Desktop.LearnsFromChats()
	status.Retrieval = "Basic word matching works without a download. Enhanced recall uses " + provider + "; unavailable embeddings fall back to basic matching."
	status.Chat = "Selected saved context and task sources are supplied to the chat model chosen in /model."
	if settings.Defaults.Provider == localmodel.ProviderID {
		status.Chat += " Your selected Work chat model runs through local Ollama. Connected tools may still contact their configured services."
	} else if settings.Defaults.Provider == "" {
		status.Chat += " No chat model is selected yet. Cloud models process supplied context remotely even if storage and retrieval are local."
	} else {
		status.Chat += fmt.Sprintf(" Selected: %s / %s. Cloud providers process supplied context remotely even if storage and retrieval are local; custom endpoints follow their configured destination.", settings.Defaults.Provider, settings.Defaults.Model)
	}
	if provider != "ollama" {
		status.Retrieval += " Your cloud retrieval opt-in uploads eligible saved text and search queries to " + provider + " for embedding."
		if _, err := configuredEmbedder(stateDir); err != nil {
			status.Detail = "Cloud retrieval unavailable; check the upload setting and API key. Use /model → Cloud to configure that provider's key without selecting it as the chat model. Basic matching remains available."
		} else {
			status.State, status.Detail, status.Ready = "Enhanced recall configured", "API key configured; network access and embedding responses are checked on each Work run.", true
		}
		return status, nil
	}
	status.Retrieval += " Ollama retrieval runs on this computer. It does not change the selected chat model."
	client, err := localmodel.NewClient("")
	if err != nil {
		return status, err
	}
	models, err := client.Models(ctx)
	if err != nil {
		status.Detail = "Ollama is unavailable. Choose Local model in Models to install or start it, then return here. Basic recall continues."
		return status, nil
	}
	for _, model := range models {
		if model.Name == retrieval.LocalModel {
			status.Ready, status.State, status.Detail = true, "Enhanced local recall installed", "Embedding responses are checked on each run; failures use basic word matching."
			return status, nil
		}
	}
	status.Detail = "Enhanced local recall needs a separate 622 MB model. Press I to review installation; basic recall works now."
	return status, nil
}

func configureWorkRecall(provider string, allowUpload bool) error {
	if provider != "ollama" && provider != "openai" && provider != "gemini" {
		return errors.New("unsupported recall provider")
	}
	if provider != "ollama" && !allowUpload {
		return errors.New("review and accept the cloud text upload before enabling it")
	}
	store, err := config.DefaultStore()
	if err != nil {
		return err
	}
	settings, err := store.Load()
	if err != nil {
		return err
	}
	settings.Embeddings = config.EmbeddingSettings{Provider: provider, CloudUploadApproved: provider != "ollama" && allowUpload}
	return store.Save(settings)
}

func installWorkRecall(ctx context.Context, progress func(string)) error {
	model := localmodel.Model{OllamaModel: retrieval.LocalModel, SourceURL: "https://ollama.com/library/embeddinggemma", Download: "622 MB", DownloadBytes: 622000000}
	eligibility := localmodel.Assess(model, localmodel.DetectHost())
	if !eligibility.Allowed {
		return fmt.Errorf("local recall model unavailable on this host: %s", eligibility.Reason)
	}
	client, err := localmodel.NewClient("")
	if err != nil {
		return err
	}
	return client.Pull(ctx, model, func(p localmodel.Progress) {
		if p.Status != "" {
			progress(p.Status)
		}
	})
}
