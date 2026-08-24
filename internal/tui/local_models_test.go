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
			Download:    "4.7 GB",
			Context:     "32K",
			Summary:     "smallest reviewed coding option",
			SourceURL:   "https://ollama.example/qwen",
		}},
	}}
	model := New(Config{RepositoryPath: t.TempDir(), LocalModels: manager})
	model.width, model.height = 100, 42
	model.resizeInputs()
	model.task.SetValue("/local")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("/local did not start a status refresh")
	}
	model = next.(Model)
	if model.screen != localModelsScreen || model.localModels.action != localModelRefreshing {
		t.Fatalf("/local screen state = screen:%v action:%v", model.screen, model.localModels.action)
	}

	next, _ = model.Update(localModelStatusMsg{generation: model.localModels.generation, catalog: manager.catalog})
	model = next.(Model)
	if !strings.Contains(model.View(), "Qwen2.5-Coder 7B") || !strings.Contains(model.View(), "Connected · Ollama test") {
		t.Fatalf("local model view = %q", model.View())
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
		Models:       []LocalModel{{ID: "qwen2.5-coder-7b", Name: "Qwen2.5-Coder 7B", Download: "4.7 GB", Context: "32K"}},
	}}
	model := New(Config{LocalModels: manager})
	model.width, model.height = 100, 40
	model.screen = localModelsScreen
	model.localModels.action = localModelRefreshing
	model.localModels.generation = 1
	next, _ := model.Update(localModelStatusMsg{generation: 1, catalog: manager.catalog})
	updated := next.(Model)
	view := updated.View()
	if !strings.Contains(view, "Unavailable") || !strings.Contains(view, "gator local serve") || !strings.Contains(view, "Qwen2.5-Coder 7B") {
		t.Fatalf("unavailable local model view = %q", view)
	}
	next, command := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if command != nil || next.(Model).localModels.confirmation != localModelNoConfirmation || !strings.Contains(next.(Model).notice.text, "unavailable") {
		t.Fatalf("pull while unavailable = %#v command:%#v", next.(Model), command)
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
	pulled  bool
	used    bool
	removed bool
}

func (manager *fakeLocalModelManager) Status(context.Context) (LocalModelCatalog, error) {
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
