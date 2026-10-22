package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workrun"
)

type gatorAppController struct {
	stateDir        string
	settings        config.Store
	request         *workrun.Request
	startSupervisor func(string) error
}

func (c gatorAppController) Inspect(ctx context.Context, query workrun.AppQuery) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	settings, err := c.settings.Load()
	if err != nil {
		return nil, err
	}
	switch query.Topic {
	case "followups":
		return c.inspectFollowups(query)
	case "settings":
		selected, err := currentWorkModelStatus(c.settings, c.stateDir)
		return map[string]any{"model": selected, "theme": settings.Theme, "retrieval": settings.Embeddings, "learn_from_chats": settings.Desktop.LearnsFromChats(), "storage": "Private Gator content is stored locally. Cloud retrieval uploads eligible saved text and queries; cloud chat processes supplied context remotely.", "settings_pages": []string{"Connections", "Models", "Usage & limits", "Data & backups", "Recovery", "About & updates"}}, err
	case "models":
		type choice struct {
			Provider string   `json:"provider"`
			Models   []string `json:"models"`
		}
		choices := []choice{}
		for _, name := range model.Names() {
			provider, _ := model.ParseProvider(name)
			choices = append(choices, choice{name, model.CuratedModels(provider)})
		}
		for _, custom := range settings.CustomProviders {
			choices = append(choices, choice{custom.ID, append([]string{}, custom.Models...)})
		}
		return map[string]any{"selected": settings.Defaults, "choices": choices, "note": "Curated choices are not proof of authentication. Configure API keys in Settings → Models; new local downloads require the Local model screen."}, nil
	case "usage", "data", "recovery":
		screen, err := workControls(c.stateDir)(ctx, query.Topic, "status", nil)
		return screen.Panel, err
	case "connections":
		choices := []map[string]any{}
		for _, descriptor := range settings.Connectors {
			operations := []string{}
			for _, operation := range descriptor.Operations() {
				operations = append(operations, operation.ID)
			}
			choices = append(choices, map[string]any{"id": descriptor.ID, "name": descriptor.Name, "operations": operations, "selected_for_this_run": slices.Contains(c.request.ConnectorIDs, descriptor.ID)})
		}
		return map[string]any{"connections": choices, "note": "Manage accounts in Settings → Connections. Sign-in, live access checks and reconnect happen there; never ask for credentials in chat. Selection applies to future requests; it does not authenticate or authorize external changes."}, nil
	case "outputs":
		return c.inspectOutputs(query.ID)
	case "shortcuts":
		return map[string]any{"bindings": settings.Desktop.ResolvedShortcuts(), "defaults": config.DefaultShortcuts, "note": "Mod means Cmd on macOS and Ctrl elsewhere. Use shortcut default to reset one action. Escape and operating-system shortcuts remain reserved."}, nil
	case "voice":
		_, status, err := voiceConfiguration(settings, c.stateDir)
		return status, err
	default:
		return nil, fmt.Errorf("unknown Gator app topic %q", query.Topic)
	}
}

func (c gatorAppController) inspectOutputs(id string) (any, error) {
	store, err := workhistory.Open(c.stateDir)
	if err != nil {
		return nil, err
	}
	if id != "" {
		record, err := store.Load(id)
		if err != nil {
			return nil, err
		}
		if record.Evidence.ArtifactManifestPath == "" {
			return nil, errors.New("this run has no sealed output; inspect outputs after it finishes")
		}
		return summarizeWorkBundle(filepath.Dir(record.Evidence.ArtifactManifestPath))
	}
	records, err := store.List(20)
	if err != nil {
		return nil, err
	}
	items := []map[string]any{}
	for _, record := range records {
		if record.Evidence.ArtifactManifestPath == "" {
			continue
		}
		bundle, err := summarizeWorkBundle(filepath.Dir(record.Evidence.ArtifactManifestPath))
		if err != nil {
			items = append(items, map[string]any{"id": record.ID, "issue": err.Error()})
			continue
		}
		for i := range bundle.Artifacts {
			bundle.Artifacts[i].Preview = ""
			bundle.Artifacts[i].PreviewKind = ""
		}
		items = append(items, map[string]any{"id": record.ID, "title": record.ObjectiveSummary, "status": bundle.Status, "artifacts": bundle.Artifacts})
	}
	return map[string]any{"runs": items, "limit": 20}, nil
}

func (c gatorAppController) Prepare(ctx context.Context, change workrun.AppChange) (workrun.AppMutation, error) {
	if change.Setting == "followup" {
		return c.prepareFollowup(ctx, change)
	}
	return c.prepareSettings(ctx, change)
}

var _ workrun.AppController = gatorAppController{}
