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
