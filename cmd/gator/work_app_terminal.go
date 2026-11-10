package main

import (
	"context"
	"errors"
	"strings"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/workrun"
)

func (c gatorAppController) prepareTerminalPreferences(change workrun.AppChange) (workrun.AppMutation, error) {
	var patch config.TerminalPreferencesPatch
	if change.Setting == "theme" {
		var value struct {
			Theme *string `json:"theme"`
		}
		if err := workrun.DecodeAppArguments(change.Value, &value); err != nil {
			return workrun.AppMutation{}, err
		}
		patch.Theme = value.Theme
	} else if err := workrun.DecodeAppArguments(change.Value, &patch); err != nil {
		return workrun.AppMutation{}, err
	}
	if patch.Empty() {
		return workrun.AppMutation{}, errors.New("choose a terminal preference to change")
	}
	if patch.WorkDetailsSections != nil {
		return workrun.AppMutation{}, errors.New("use /work-details to inspect task information")
	}
	settings, err := c.settings.Load()
	if err != nil {
		return workrun.AppMutation{}, err
	}
	if settings.TUI.Preferences == nil && c.request != nil && c.request.ForegroundApp {
		return workrun.AppMutation{}, errors.New("terminal preferences have not finished importing; open /terminal-settings to retry or review default display settings")
	}
	before := settings.TerminalPreferences()
	after, err := patch.Apply(before)
	if err != nil {
		return workrun.AppMutation{}, err
	}
	changes := []string{}
	if patch.Theme != nil {
		changes = append(changes, "appearance: "+after.Theme)
	}
	if patch.SidebarCollapsed != nil {
		if after.SidebarCollapsed {
			changes = append(changes, "navigation rail hidden")
		} else {
			changes = append(changes, "navigation rail shown when space permits")
		}
	}
	if patch.GroupChatsByFolder != nil {
		if after.GroupChatsByFolder {
			changes = append(changes, "recent chats grouped by folder")
		} else {
			changes = append(changes, "flat recent chats")
		}
	}
	if patch.WorkDetailsSections != nil {
		changes = append(changes, "work detail sections updated")
	}
	return workrun.AppMutation{Summary: "Terminal preferences saved: " + strings.Join(changes, "; ") + ".", Preview: map[string]any{"before": before, "after": after, "scope": "Terminal display only; task permissions, models, approvals and drafts are unchanged."}, Apply: func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return c.settings.UpdateTerminalPreferences(patch, &before)
	}}, nil
}
