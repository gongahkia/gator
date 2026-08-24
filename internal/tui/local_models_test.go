package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/config"
)

func TestLocalModelsAreAvailableThroughTheTUIAndConfigureTheNextRun(t *testing.T) {
	manager := &fakeLocalModelManager{catalog: LocalModelCatalog{
		RuntimeURL:     "http://127.0.0.1:11434",
		RuntimeVersion: "test",
		Executable:     "/usr/bin/ollama",
		Models: []LocalModel{{
			ID:          "qwen2.5-coder-7b",
			OllamaModel: "qwen2.5-coder:7b",
			Name:        "Qwen2.5-Coder 7B",
			DefaultName: "Qwen2.5-Coder 7B",
			Download:    "4.7 GB",
			Context:     "32K",
			Summary:     "smallest reviewed coding option",
			SourceURL:   "https://ollama.example/qwen",
		}},
	}}
	model := New(Config{RepositoryPath: t.TempDir(), LocalModels: manager})
	model.width, model.height = 100, 42
	model.resizeInputs()
	model.task.SetValue("/model")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("/model did not start a status refresh")
	}
	model = next.(Model)
	if model.screen != localModelsScreen || model.localModels.action != localModelRefreshing {
		t.Fatalf("/model screen state = screen:%v action:%v", model.screen, model.localModels.action)
	}

	next, _ = model.Update(localModelStatusMsg{generation: model.localModels.generation, catalog: manager.catalog})
	model = next.(Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = next.(Model)
	if !strings.Contains(model.View(), "Qwen2.5-Coder 7B") || !strings.Contains(model.View(), "Connected · Ollama test") {
		t.Fatalf("local model view = %q", model.View())
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	model = next.(Model)
	if model.localModels.renaming == nil {
		t.Fatal("rename did not open a display-name editor")
	}
	model.localModels.renaming.input.SetValue("desk Qwen")
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = finishLocalModelOperation(t, next.(Model))
	if got := model.localModels.catalog.Models[0].Name; got != "desk Qwen" || model.config.ModelAliases[config.ModelAliasKey("gator-local", "qwen2.5-coder:7b")] != "desk Qwen" {
		t.Fatalf("renamed local model = %#v aliases:%#v", model.localModels.catalog.Models[0], model.config.ModelAliases)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	model = next.(Model)
	if model.localModels.confirmation != localModelConfirmPull {
		t.Fatalf("pull confirmation = %v", model.localModels.confirmation)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model = next.(Model)
	if model.localModels.action != localModelPulling || model.localModels.operation == nil {
		t.Fatalf("pull state = action:%v operation:%#v", model.localModels.action, model.localModels.operation)
	}
	model = finishLocalModelOperation(t, model)
	if !manager.pulled || !model.localModels.catalog.Models[0].Installed {
		t.Fatalf("pull state = manager:%#v catalog:%#v", manager, model.localModels.catalog)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.localModels.action != localModelUsing || model.localModels.operation == nil {
		t.Fatalf("use state = action:%v operation:%#v", model.localModels.action, model.localModels.operation)
	}
	model = finishLocalModelOperation(t, model)
	if !manager.used || model.provider.Value() != "gator-local" || model.model.Value() != "qwen2.5-coder:7b" {
		t.Fatalf("local selection = used:%t provider:%q model:%q", manager.used, model.provider.Value(), model.model.Value())
	}
	if provider, modelName, custom, err := model.resolveProviderAndModel(model.provider.Value(), model.model.Value()); err != nil || !custom || provider != "gator-local" || modelName != "qwen2.5-coder:7b" {
		t.Fatalf("selected local runtime = provider:%q model:%q custom:%t err:%v", provider, modelName, custom, err)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	model = next.(Model)
	if model.localModels.confirmation != localModelConfirmRemove {
		t.Fatalf("remove confirmation = %v", model.localModels.confirmation)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model = next.(Model)
	model = finishLocalModelOperation(t, model)
	if !manager.removed || model.provider.Value() != "" || model.model.Value() != "" {
		t.Fatalf("remove state = removed:%t provider:%q model:%q", manager.removed, model.provider.Value(), model.model.Value())
	}
}

func TestLocalModelsShowsUnavailableRuntimeWithoutHidingCatalog(t *testing.T) {
	manager := &fakeLocalModelManager{catalog: LocalModelCatalog{
		RuntimeURL:   "http://127.0.0.1:11434",
		RuntimeError: "connect to local Ollama runtime: connection refused",
		Executable:   "/usr/bin/ollama",
		Models:       []LocalModel{{ID: "qwen2.5-coder-7b", Name: "Qwen2.5-Coder 7B", Download: "4.7 GB", Context: "32K"}},
	}}
	model := New(Config{LocalModels: manager})
	model.width, model.height = 100, 40
	model.screen = localModelsScreen
	model.localModels.action = localModelRefreshing
	model.localModels.generation = 1
	next, _ := model.Update(localModelStatusMsg{generation: 1, catalog: manager.catalog})
	updated := next.(Model)
	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyTab})
	updated = next.(Model)
	view := updated.View()
	if updated.localModels.confirmation != localModelConfirmStart || !strings.Contains(view, "Unavailable") || !strings.Contains(view, "Start Ollama?") || !strings.Contains(view, "Qwen2.5-Coder 7B") {
		t.Fatalf("unavailable local model view = %q", view)
	}
	next, command := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if command != nil || next.(Model).localModels.confirmation != localModelNoConfirmation || !next.(Model).localModels.startDismissed || !strings.Contains(next.(Model).notice.text, "Start Ollama yourself") {
		t.Fatalf("manual local runtime choice = confirmation:%v dismissed:%t notice:%q command:%#v", next.(Model).localModels.confirmation, next.(Model).localModels.startDismissed, next.(Model).notice.text, command)
	}
}

func TestLocalModelsStartOllamaWithGatorByDefault(t *testing.T) {
	manager := &fakeLocalModelManager{catalog: LocalModelCatalog{
		RuntimeURL:   "http://127.0.0.1:11434",
		RuntimeError: "connect to local Ollama runtime: connection refused",
		Executable:   "/usr/bin/ollama",
		Models:       []LocalModel{{ID: "qwen2.5-coder-7b", Name: "Qwen2.5-Coder 7B"}},
	}}
	model := New(Config{LocalModels: manager})
	model.width, model.height = 100, 40
	model.screen = localModelsScreen
	model.localModels.action = localModelRefreshing
	model.localModels.generation = 1
	next, _ := model.Update(localModelStatusMsg{generation: 1, catalog: manager.catalog})
	model = next.(Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = next.(Model)
	if model.localModels.confirmation != localModelConfirmStart {
		t.Fatalf("start prompt = %#v", model.localModels)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.localModels.action != localModelStarting || model.localModels.operation == nil {
		t.Fatalf("start operation = %#v", model.localModels)
	}
	model = finishLocalModelOperation(t, model)
	if !manager.started || model.localModels.catalog.RuntimeError != "" || !strings.Contains(model.notice.text, "running under this Gator session") {
		t.Fatalf("started local runtime = manager:%#v catalog:%#v notice:%q", manager, model.localModels.catalog, model.notice.text)
	}
}

func TestLocalModelsOfferInstallationHelpWhenOllamaIsMissing(t *testing.T) {
	manager := &fakeLocalModelManager{catalog: LocalModelCatalog{
		RuntimeURL:   "http://127.0.0.1:11434",
		RuntimeError: "connect to local Ollama runtime: connection refused",
		Dependencies: []LocalDependency{
			{
				ID:           "ollama",
				Name:         "Ollama",
				Purpose:      "reviewed local coding models",
				HelpURL:      "https://ollama.com/download",
				Instructions: []string{"Install Ollama from its official download page, then refresh this screen."},
			},
			{
				ID:           "bubblewrap",
				Name:         "Bubblewrap",
				Purpose:      "strict Linux sandboxing",
				Required:     true,
				HelpURL:      "https://github.com/containers/bubblewrap",
				Instructions: []string{"Install the bubblewrap package."},
			},
		},
		Models: []LocalModel{{ID: "qwen2.5-coder-7b", Name: "Qwen2.5-Coder 7B"}},
	}}
	model := New(Config{LocalModels: manager})
	model.width, model.height = 100, 40
	model.screen = localModelsScreen
	model.localModels.action = localModelRefreshing
	model.localModels.generation = 1
	next, _ := model.Update(localModelStatusMsg{generation: 1, catalog: manager.catalog})
	model = next.(Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = next.(Model)
	if model.localModels.confirmation != localModelConfirmInstall || !strings.Contains(model.View(), "Install Ollama?") {
		t.Fatalf("install prompt = %#v view:%q", model.localModels, model.View())
	}
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if command != nil || manager.started || !model.localModels.dependencyHelp {
		t.Fatalf("installation help state = command:%#v manager:%#v state:%#v", command, manager, model.localModels)
	}
	view := model.View()
	if !strings.Contains(view, "https://ollama.com/download") || !strings.Contains(view, "Bubblewrap") || !strings.Contains(model.notice.text, "does not run system installers") {
		t.Fatalf("installation help view = %q", view)
	}
}

func TestLocalModelsDisableIneligibleCatalogEntriesBeforePullOrUse(t *testing.T) {
	manager := &fakeLocalModelManager{catalog: LocalModelCatalog{
		RuntimeURL:     "http://127.0.0.1:11434",
		RuntimeVersion: "test",
		Models: []LocalModel{{
			ID:            "qwen2.5-coder-7b",
			OllamaModel:   "qwen2.5-coder:7b",
			Name:          "Qwen2.5-Coder 7B",
			Installed:     true,
			Requirement:   "needs 8.8 GiB RAM / 5.3 GiB disk",
			BlockedReason: "requires 8.8 GiB currently available RAM under Gator's guardrail; detected 6.0 GiB",
		}},
	}}
	model := New(Config{LocalModels: manager})
	model.width, model.height = 100, 40
	model.screen = localModelsScreen
	model.localModels.section = localModelSection
	model.localModels.catalog = manager.catalog

	next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	updated := next.(Model)
	if command != nil || updated.localModels.confirmation != localModelNoConfirmation || !strings.Contains(updated.notice.text, "disabled on this host") || manager.pulled {
		t.Fatalf("blocked pull = %#v command:%#v manager:%#v", updated, command, manager)
	}
	next, command = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated = next.(Model)
	if command != nil || updated.localModels.operation != nil || !strings.Contains(updated.notice.text, "disabled on this host") || manager.used {
		t.Fatalf("blocked use = %#v command:%#v manager:%#v", updated, command, manager)
	}
	if view := updated.View(); !strings.Contains(view, "disabled") || !strings.Contains(view, "needs 8.8 GiB RAM") {
		t.Fatalf("blocked local model view = %q", view)
	}
}

func TestModelCatalogShowsCloudReadinessAndSelectsCloudModel(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	manager := &fakeLocalModelManager{}
	model := New(Config{LocalModels: manager})
	model.width, model.height = 100, 40
	next, _ := model.openModelCatalog()
	model = next.(Model)
	for index, entry := range model.cloudModels() {
		if entry.provider == "openai" {
			model.localModels.cloudIndex = index
			break
		}
	}
	if !strings.Contains(model.View(), "API key set: OPENAI_API_KEY") {
		t.Fatalf("cloud readiness view = %q", model.View())
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.provider.Value() != "openai" || model.model.Value() == "" {
		t.Fatalf("cloud selection = provider:%q model:%q", model.provider.Value(), model.model.Value())
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	model = next.(Model)
	if model.localModels.renaming == nil {
		t.Fatal("cloud rename did not open a display-name editor")
	}
	model.localModels.renaming.input.SetValue("work OpenAI")
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = finishLocalModelOperation(t, next.(Model))
	if model.modelAlias("openai", model.model.Value()) != "work OpenAI" || !strings.Contains(model.View(), "work OpenAI") {
		t.Fatalf("cloud alias = %#v view:%q", model.config.ModelAliases, model.View())
	}
}

func finishLocalModelOperation(t *testing.T, model Model) Model {
	t.Helper()
	for range 8 {
		if model.localModels.operation == nil {
			return model
		}
		message := waitForLocalModelOperation(model.localModels.operation)()
		next, _ := model.Update(message)
		model = next.(Model)
	}
	t.Fatalf("local model operation did not finish: %#v", model.localModels)
	return Model{}
}

type fakeLocalModelManager struct {
	catalog LocalModelCatalog
	started bool
	pulled  bool
	used    bool
	removed bool
}

func (manager *fakeLocalModelManager) Status(context.Context) (LocalModelCatalog, error) {
	return manager.catalog, nil
}

func (manager *fakeLocalModelManager) Start(context.Context) (LocalModelCatalog, error) {
	manager.started = true
	manager.catalog.RuntimeError = ""
	if manager.catalog.RuntimeVersion == "" {
		manager.catalog.RuntimeVersion = "started"
	}
	return manager.catalog, nil
}

func (manager *fakeLocalModelManager) Pull(_ context.Context, id string, report func(LocalModelProgress)) (LocalModelCatalog, error) {
	if id != "qwen2.5-coder-7b" {
		return LocalModelCatalog{}, context.Canceled
	}
	if report != nil {
		report(LocalModelProgress{Status: "pulling layers", Completed: 5, Total: 10})
	}
	manager.pulled = true
	manager.catalog.Models[0].Installed = true
	return manager.catalog, nil
}

func (manager *fakeLocalModelManager) Use(_ context.Context, id string) (LocalModelUpdate, error) {
	if id != "qwen2.5-coder-7b" || !manager.catalog.Models[0].Installed {
		return LocalModelUpdate{}, context.Canceled
	}
	manager.used = true
	return LocalModelUpdate{
		Catalog: manager.catalog,
		CustomProviders: []config.CustomProvider{{
			ID: "gator-local", BaseURL: "http://127.0.0.1:11434/v1/chat/completions", Models: []string{"qwen2.5-coder:7b"}, DefaultModel: "qwen2.5-coder:7b",
		}},
		Provider: "gator-local",
		Model:    "qwen2.5-coder:7b",
	}, nil
}

func (manager *fakeLocalModelManager) Remove(_ context.Context, id string) (LocalModelUpdate, error) {
	if id != "qwen2.5-coder-7b" {
		return LocalModelUpdate{}, context.Canceled
	}
	manager.removed = true
	manager.catalog.Models[0].Installed = false
	return LocalModelUpdate{Catalog: manager.catalog}, nil
}

func (manager *fakeLocalModelManager) Rename(_ context.Context, provider, model, alias string) (map[string]string, error) {
	return map[string]string{config.ModelAliasKey(provider, model): alias}, nil
}
