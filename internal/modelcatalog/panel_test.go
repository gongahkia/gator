package modelcatalog

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestPanelPersistsSelectionAndIgnoresAnotherVisitMessages(t *testing.T) {
	savedProvider, savedModel := "", ""
	configuration := Config{
		Provider:    "openai",
		Model:       "current",
		LocalModels: &fakeLocalManager{},
		SaveModelSelection: func(provider, model string) error {
			savedProvider, savedModel = provider, model
			return nil
		},
	}
	panel := NewModelCatalogPanel(configuration)
	panel.Init()
	panel.model.localModels.action = localModelIdle
	next, _ := panel.model.useCloudModel(cloudModelEntry{provider: "openai", model: "selected", name: "selected", selectable: true})
	panel.model = next.(Model)
	if savedProvider != "openai" || savedModel != "selected" {
		t.Fatal("cloud selection was not persisted")
	}
	panel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !panel.Closed() {
		t.Fatal("Esc did not close the panel")
	}

	reopened := NewModelCatalogPanel(configuration)
	defer reopened.Close()
	reopened.Init()
	reopened.Update(catalogPanelMessage{panel: panel, message: localModelStatusMsg{generation: 1, catalog: LocalCatalog{RuntimeVersion: "stale"}}})
	if reopened.model.localModels.catalog.RuntimeVersion == "stale" {
		t.Fatal("closed panel's asynchronous response reached a new visit")
	}
}

func TestPanelCloseCancelsPendingRefresh(t *testing.T) {
	manager := &refreshCancellationManager{entered: make(chan struct{}), cancelled: make(chan struct{})}
	panel := NewModelCatalogPanel(Config{LocalModels: manager})
	batch := panel.Init()().(tea.BatchMsg)
	done := make(chan struct{})
	go func() { defer close(done); batch[1]() }()
	<-manager.entered
	panel.Close()
	<-manager.cancelled
	<-done
	panel.Close()
}

func TestPanelConfirmationRemainsVisibleInShortTerminal(t *testing.T) {
	panel := NewModelCatalogPanel(Config{LocalModels: &fakeLocalManager{}})
	defer panel.Close()
	panel.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	panel.model.localModels.section = localModelSection
	panel.model.localModels.catalog = LocalCatalog{RuntimeVersion: "fixture", Models: []LocalModel{{
		ID: "fixture", Name: "Reviewed model", Download: "398 MB", SourceURL: "https://ollama.com/library/qwen2.5-coder",
	}}}
	panel.model.localModels.confirmation = localModelConfirmPull
	view := panel.View()
	for _, required := range []string{"Confirm download", "398 MB", "https://ollama.com/library/qwen2.5-coder", "enter/y download", "esc/n cancel"} {
		if !strings.Contains(view, required) {
			t.Fatalf("confirmation hid %q:\n%s", required, view)
		}
	}
}

func TestPanelUsesWorkVisualLanguage(t *testing.T) {
	panel := NewModelCatalogPanel(Config{Provider: "gemini", Model: "gemini-3.5-flash", LocalModels: &fakeLocalManager{}})
	defer panel.Close()
	panel.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	panel.model.localModels.action = localModelIdle
	panel.model.localModels.section = cloudModelSection
	panel.model.notice = notice{}

	view := panel.View()
	plain := ansi.Strip(view)
	for _, required := range []string{"Models", "Cloud", "Local", "gemini", "↑/↓ choose", "esc back", "f1 help"} {
		if !strings.Contains(plain, required) {
			t.Fatalf("work-native model panel omitted %q:\n%s", required, plain)
		}
	}
	for _, legacy := range []string{"🐊 Gator  models · gator", "Tab: Cloud / Local", "Readiness is credential/configuration state only", "Cloud models"} {
		if strings.Contains(plain, legacy) {
			t.Fatalf("model panel retained legacy chrome %q:\n%s", legacy, plain)
		}
	}
}

type fakeLocalManager struct{ catalog LocalCatalog }

func (m *fakeLocalManager) Status(context.Context) (LocalCatalog, error) { return m.catalog, nil }
func (m *fakeLocalManager) Start(context.Context) (LocalCatalog, error)  { return m.catalog, nil }
func (m *fakeLocalManager) Pull(_ context.Context, _ string, report func(LocalProgress)) (LocalCatalog, error) {
	if report != nil {
		report(LocalProgress{Status: "complete"})
	}
	return m.catalog, nil
}
func (m *fakeLocalManager) Use(context.Context, string) (LocalUpdate, error) {
	return LocalUpdate{Catalog: m.catalog}, nil
}
func (m *fakeLocalManager) Remove(context.Context, string) (LocalUpdate, error) {
	return LocalUpdate{Catalog: m.catalog}, nil
}
func (m *fakeLocalManager) Rename(context.Context, string, string, string) (map[string]string, error) {
	return nil, nil
}

type refreshCancellationManager struct {
	fakeLocalManager
	entered   chan struct{}
	cancelled chan struct{}
}

func (m *refreshCancellationManager) Status(ctx context.Context) (LocalCatalog, error) {
	close(m.entered)
	<-ctx.Done()
	close(m.cancelled)
	return LocalCatalog{}, ctx.Err()
}
