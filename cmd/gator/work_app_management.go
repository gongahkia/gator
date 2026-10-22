package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"

	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/delivery"
	"github.com/gongahkia/gator/internal/lifecycle"
	"github.com/gongahkia/gator/internal/workapp"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

type appScreenResult struct {
	Family     string
	Screen     workapp.ControlScreen
	FormAction string
}

func (c gatorAppController) prepareManagement(ctx context.Context, change workrun.AppChange) (workrun.AppMutation, error) {
	switch change.Setting {
	case "connections":
		var v struct {
			IDs []string `json:"ids"`
		}
		if err := workrun.DecodeAppArguments(change.Value, &v); err != nil {
			return workrun.AppMutation{}, err
		}
		if v.IDs == nil {
			return workrun.AppMutation{}, errors.New("provide ids, using [] to clear the selection")
		}
		if change.ConversationID == "" {
			return workrun.AppMutation{}, errors.New("a saved conversation is required")
		}
		settings, err := c.settings.Load()
		if err != nil {
			return workrun.AppMutation{}, err
		}
		seen := map[string]bool{}
		for _, id := range v.IDs {
			found := false
			for _, descriptor := range settings.Connectors {
				found = found || descriptor.ID == id
			}
			if !found || seen[id] {
				return workrun.AppMutation{}, fmt.Errorf("connection %q is missing or repeated; inspect connections", id)
			}
			seen[id] = true
		}
		store, err := worksession.Open(c.stateDir)
		if err != nil {
			return workrun.AppMutation{}, err
		}
		prior, err := store.LoadPreferences(change.ConversationID)
		if err != nil {
			return workrun.AppMutation{}, err
		}
		return workrun.AppMutation{Summary: "Connection selection saved for future messages in this conversation.", Preview: map[string]any{"ids": v.IDs, "effect": "Does not sign in, change permissions, or enable these tools in the current run."}, Apply: func(context.Context) (any, error) {
			current, err := c.settings.Load()
			if err != nil {
				return nil, err
			}
			for _, id := range v.IDs {
				found := false
				for _, descriptor := range current.Connectors {
					found = found || descriptor.ID == id
				}
				if !found {
					return nil, errors.New("a connection was removed; inspect connections again")
				}
			}
			return v, store.SavePreferences(change.ConversationID, worksession.Preferences{ConnectorIDs: &v.IDs}, &prior)
		}}, nil
	case "data_action":
		return c.prepareDataAction(change)
	case "recovery":
		var v struct {
			Action string `json:"action"`
		}
		if err := workrun.DecodeAppArguments(change.Value, &v); err != nil {
			return workrun.AppMutation{}, err
		}
		if !slices.Contains([]string{"start", "stop", "enable_login", "disable_login"}, v.Action) {
			return workrun.AppMutation{}, errors.New("recovery action must be start, stop, enable_login or disable_login")
		}
		servicePath, err := recoveryServicePath()
		if err != nil {
			return workrun.AppMutation{}, err
		}
		installed, owned, digest, err := recoveryServiceReview(servicePath, c.stateDir, runtime.GOOS)
		if err != nil {
			return workrun.AppMutation{}, err
		}
		if installed && !owned {
			return workrun.AppMutation{}, errors.New("the background service belongs to another Gator profile")
		}
		return workrun.AppMutation{Review: true, AfterRun: true, Summary: "Background service change completed: " + strings.ReplaceAll(v.Action, "_", " ") + ".", Preview: map[string]any{"action": v.Action, "effect": "Changes Gator's background process or per-user start-at-login service; saved checks remain."}, Apply: func(ctx context.Context) (any, error) {
			action := v.Action
			if action == "stop" {
				return managedJobCommand(ctx, c.stateDir, "stop")
			}
			args := []string(nil)
			if action == "enable_login" {
				action = "install"
				args = []string{"", digest}
			}
			if action == "disable_login" {
				action = "remove"
				args = []string{"", digest}
			}
			screen, err := recoveryControl(ctx, c.stateDir, action, args)
			return screen.Panel, err
		}}, nil
	case "output_action":
		var v struct {
			Action string `json:"action"`
			ID     string `json:"id"`
			Path   string `json:"path"`
		}
		if err := workrun.DecodeAppArguments(change.Value, &v); err != nil {
			return workrun.AppMutation{}, err
		}
		if v.Action != "save" || v.ID == "" || !filepath.IsAbs(v.Path) {
			return workrun.AppMutation{}, errors.New("saving outputs requires action save, a retained run id, and an absolute destination folder path")
		}
		store, err := workhistory.Open(c.stateDir)
		if err != nil {
			return workrun.AppMutation{}, err
		}
		record, err := store.Load(v.ID)
		if err != nil {
			return workrun.AppMutation{}, err
		}
		if record.Evidence.ArtifactManifestPath == "" {
			return workrun.AppMutation{}, errors.New("the run has no sealed output")
		}
		request := workapp.BundleActionRequest{Action: "save", BundlePath: filepath.Dir(record.Evidence.ArtifactManifestPath), Target: v.Path}
		bundle, err := artifact.OpenBundle(request.BundlePath)
		if err != nil {
			return workrun.AppMutation{}, err
		}
		preview, err := workTUIBundleAction(c.stateDir)(request)
		if err != nil {
			return workrun.AppMutation{}, err
		}
		return workrun.AppMutation{Review: true, AfterRun: true, Summary: "Verified outputs saved to " + v.Path + ".", Preview: map[string]any{"action": "save_outputs", "id": v.ID, "destination": v.Path, "plan": preview}, Apply: func(ctx context.Context) (any, error) {
			current, err := artifact.OpenBundle(request.BundlePath)
			if err != nil {
				return nil, err
			}
			before, _ := json.Marshal(bundle.Manifest)
			after, _ := json.Marshal(current.Manifest)
			if !reflect.DeepEqual(before, after) {
				return nil, errors.New("output changed during review; inspect it again")
			}
			if err := artifact.VerifyBundle(current); err != nil {
				return nil, err
			}
			deliveries, err := delivery.Open(c.stateDir)
			if err != nil {
				return nil, err
			}
			return deliveries.DeliverArtifacts(ctx, current, request.Target, false)
		}}, nil
	}
	return workrun.AppMutation{}, errors.New("unsupported app management operation")
}

func (c gatorAppController) prepareDataAction(change workrun.AppChange) (workrun.AppMutation, error) {
	var v struct {
		Action string `json:"action"`
		Path   string `json:"path"`
		Scope  string `json:"scope"`
	}
	if err := workrun.DecodeAppArguments(change.Value, &v); err != nil {
		return workrun.AppMutation{}, err
	}
	if !slices.Contains([]string{"backup", "verify", "cleanup", "delete", "restore", "relink"}, v.Action) {
		return workrun.AppMutation{}, errors.New("data action must be backup, verify, cleanup, delete, restore or relink")
	}
	if v.Action == "backup" || v.Action == "verify" {
		if !filepath.IsAbs(v.Path) || len(v.Path) > 4096 {
			return workrun.AppMutation{}, errors.New("provide an absolute backup archive path")
		}
		return workrun.AppMutation{Review: true, AfterRun: true, Summary: map[string]string{"backup": "Private backup exported to ", "verify": "Private backup verified: "}[v.Action] + v.Path + ".", Preview: map[string]any{"action": v.Action, "path": v.Path, "effect": "Backups contain private retained content and are not encrypted. Credentials and original project files are excluded."}, Apply: func(ctx context.Context) (any, error) {
			store, err := lifecycle.Open(c.stateDir)
			if err != nil {
				return nil, err
			}
			if v.Action == "verify" {
				return lifecycle.VerifyBackup(ctx, v.Path)
			}
			return store.Export(ctx, v.Path)
		}}, nil
	}
	if v.Action == "delete" && !slices.Contains([]string{"knowledge", "history", "snapshots", "all"}, v.Scope) {
		return workrun.AppMutation{}, errors.New("delete requires scope knowledge, history, snapshots or all")
	}
	return workrun.AppMutation{AfterRun: true, Summary: "Opened the data review. No content has been deleted, restored or relinked.", Preview: map[string]any{"action": v.Action, "scope": v.Scope, "effect": "Opens the canonical data review after this reply; final confirmation remains in the review screen."}, Apply: func(ctx context.Context) (any, error) {
		action := "status"
		args := []string(nil)
		if v.Action == "cleanup" {
			action = "cleanup"
		}
		if v.Action == "delete" {
			action = "plan"
			args = []string{v.Scope}
		}
		screen, err := dataControl(ctx, c.stateDir, action, args)
		formAction := ""
		if v.Action == "restore" {
			formAction = "restore-plan"
		}
		if v.Action == "relink" {
			formAction = "relink-plan"
		}
		if formAction != "" && v.Path != "" {
			for i := range screen.Actions {
				if screen.Actions[i].Action == formAction && len(screen.Actions[i].Fields) > 0 {
					screen.Actions[i].Fields[0].Value = v.Path
				}
			}
		}
		return appScreenResult{Family: "data", Screen: screen, FormAction: formAction}, err
	}}, nil
}
