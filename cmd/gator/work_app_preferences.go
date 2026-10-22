package main

import (
	"context"
	"errors"
	"strings"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/workrun"
)

func (c gatorAppController) prepareDesktopPreferences(change workrun.AppChange) (workrun.AppMutation, error) {
	prior, err := c.settings.Load()
	if err != nil {
		return workrun.AppMutation{}, err
	}
	if change.Setting == "shortcuts" {
		var v struct {
			Action   string `json:"action"`
			Shortcut string `json:"shortcut"`
		}
		if err := workrun.DecodeAppArguments(change.Value, &v); err != nil {
			return workrun.AppMutation{}, err
		}
		if _, ok := config.DefaultShortcuts[v.Action]; !ok {
			return workrun.AppMutation{}, errors.New("choose an action from gator_app_inspect shortcuts")
		}
		reset := strings.EqualFold(v.Shortcut, "default")
		next := config.DefaultShortcuts[v.Action]
		if !reset {
			next, err = config.NormalizeShortcut(v.Shortcut)
			if err != nil {
				return workrun.AppMutation{}, err
			}
		}
		expected := prior.Desktop.ResolvedShortcuts()[v.Action]
		return workrun.AppMutation{Preview: map[string]any{"action": v.Action, "before": expected, "after": next}, Apply: func(context.Context) (any, error) {
			err := c.settings.Update(func(s *config.Settings) error {
				if s.Desktop.ResolvedShortcuts()[v.Action] != expected {
					return errors.New("shortcut changed; inspect shortcuts and try again")
				}
				if s.Desktop.Shortcuts == nil {
					s.Desktop.Shortcuts = map[string]string{}
				}
				if reset {
					delete(s.Desktop.Shortcuts, v.Action)
				} else {
					s.Desktop.Shortcuts[v.Action] = next
				}
				return s.Desktop.Validate()
			})
			return map[string]string{"action": v.Action, "shortcut": next}, err
		}}, nil
	}
	var v struct {
		Spoken   *bool    `json:"spoken_replies"`
		Provider *string  `json:"provider"`
		Model    *string  `json:"model"`
		Language *string  `json:"language"`
		Rate     *float64 `json:"rate"`
	}
	if err := workrun.DecodeAppArguments(change.Value, &v); err != nil {
		return workrun.AppMutation{}, err
	}
	if v.Spoken == nil && v.Provider == nil && v.Model == nil && v.Language == nil && v.Rate == nil {
		return workrun.AppMutation{}, errors.New("provide a voice preference to change")
	}
	next := prior.Desktop.Voice
	if v.Spoken != nil {
		next.SpokenReplies = *v.Spoken
	}
	if v.Provider != nil {
		next.Provider = *v.Provider
		if v.Model == nil && next.Provider != prior.Desktop.Voice.Provider {
			next.Model = ""
		}
	}
	if v.Model != nil {
		next.Model = *v.Model
	}
	if v.Language != nil {
		next.Language = *v.Language
	}
	if v.Rate != nil {
		next.Rate = *v.Rate
	}
	check := config.DesktopSettings{Voice: next}
	if err := check.Validate(); err != nil {
		return workrun.AppMutation{}, err
	}
	return workrun.AppMutation{Preview: map[string]any{"before": prior.Desktop.Voice, "after": next, "note": "Microphone capture always requires the user's explicit action. Dictation becomes a reviewable draft."}, Apply: func(context.Context) (any, error) {
		err := c.settings.Update(func(s *config.Settings) error {
			if s.Desktop.Voice != prior.Desktop.Voice {
				return errors.New("voice preferences changed; inspect and try again")
			}
			s.Desktop.Voice = next
			return nil
		})
		return next, err
	}}, nil
}
