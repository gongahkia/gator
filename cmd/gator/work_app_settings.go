package main

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/workrun"
)

func (c gatorAppController) prepareSettings(ctx context.Context, change workrun.AppChange) (workrun.AppMutation, error) {
	if err := ctx.Err(); err != nil {
		return workrun.AppMutation{}, err
	}
	prior, err := c.settings.Load()
	if err != nil {
		return workrun.AppMutation{}, err
	}
	switch change.Setting {
	case "connections", "data_action", "recovery", "output_action":
		return c.prepareManagement(ctx, change)
	case "shortcuts", "voice":
		return c.prepareDesktopPreferences(change)
	case "usage_limits", "usage_pause":
		return c.prepareUsage(change)
	case "retention":
		return c.prepareRetention(change)
	case "retrieval":
		var next struct {
			Provider string `json:"provider"`
		}
		if err := workrun.DecodeAppArguments(change.Value, &next); err != nil {
			return workrun.AppMutation{}, err
		}
		if !slices.Contains([]string{"ollama", "openai", "gemini"}, next.Provider) {
			return workrun.AppMutation{}, errors.New("retrieval provider must be ollama, openai or gemini")
		}
		embeddings := config.EmbeddingSettings{Provider: next.Provider, CloudUploadApproved: next.Provider != "ollama"}
		effect := "Uses local Ollama embeddings when installed, with basic matching as fallback. No model is downloaded and the chat model stays the same."
		if embeddings.CloudUploadApproved {
			effect = "Uploads eligible saved memory, note text, and search queries to " + next.Provider + " for embedding on subsequent runs. Requires its configured API key. Local storage and the chat model stay the same."
		}
		return workrun.AppMutation{Review: true, AfterRun: true, Summary: "Retrieval preference saved: " + next.Provider + ".", Preview: map[string]any{"before": prior.Embeddings, "after": embeddings, "effect": effect}, Apply: func(context.Context) (any, error) {
			err := c.settings.Update(func(current *config.Settings) error {
				if current.Embeddings != prior.Embeddings {
					return errors.New("retrieval settings changed; inspect settings and try again")
				}
				current.Embeddings = embeddings
				return nil
			})
			return embeddings, err
		}}, nil
	case "model":
		var next config.Defaults
		if err := workrun.DecodeAppArguments(change.Value, &next); err != nil {
			return workrun.AppMutation{}, err
		}
		if !knownAppModel(prior, next) {
			return workrun.AppMutation{}, errors.New("choose an existing provider/model from gator_app_inspect models; configure new providers and downloads in Models")
		}
		return workrun.AppMutation{Preview: map[string]any{"setting": "model", "before": prior.Defaults, "after": next}, Apply: func(ctx context.Context) (any, error) {
			err := c.settings.Update(func(current *config.Settings) error {
				if current.Defaults != prior.Defaults || !knownAppModel(*current, next) {
					return errors.New("model settings changed; inspect models and try again")
				}
				current.Defaults = next
				return nil
			})
			if err != nil {
				return nil, err
			}
			return c.Inspect(ctx, workrun.AppQuery{Topic: "settings"})
		}}, nil
	case "theme":
		var next struct {
			Theme string `json:"theme"`
		}
		if err := workrun.DecodeAppArguments(change.Value, &next); err != nil {
			return workrun.AppMutation{}, err
		}
		if !slices.Contains([]string{"gator", "mono", "contrast"}, next.Theme) {
			return workrun.AppMutation{}, errors.New("theme must be gator, mono or contrast")
		}
		return workrun.AppMutation{Preview: map[string]any{"setting": "theme", "before": prior.Theme, "after": next.Theme}, Apply: func(context.Context) (any, error) {
			err := c.settings.Update(func(current *config.Settings) error {
				if current.Theme != prior.Theme {
					return errors.New("theme changed; inspect settings and try again")
				}
				current.Theme = next.Theme
				return nil
			})
			return next, err
		}}, nil
	default:
		return workrun.AppMutation{}, fmt.Errorf("app setting %q is not available through chat", change.Setting)
	}
}

func knownAppModel(settings config.Settings, selected config.Defaults) bool {
	if selected.Provider == "" || selected.Model == "" {
		return false
	}
	if custom, found := configuredCustomProvider(settings, selected.Provider); found {
		return slices.Contains(custom.Models, selected.Model)
	}
	provider, err := model.ParseProvider(selected.Provider)
	return err == nil && (slices.Contains(model.CuratedModels(provider), selected.Model) || selected == settings.Defaults)
}
