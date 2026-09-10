package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/gongahkia/gator/internal/journal"
)

func TestWorkModelPanelKeepsLegacyDraftAndIgnoresClosedPanelMessages(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	if err := journal.SaveDraft(state, journal.Draft{Repository: source, Task: "private Code draft", Provider: "anthropic", Model: "legacy"}); err != nil {
		t.Fatal(err)
	}
	savedProvider, savedModel := "", ""
	configuration := Config{RepositoryPath: source, StateDir: state, Provider: "openai", Model: "current", LocalModels: &fakeLocalModelManager{}, SaveModelSelection: func(provider, model string) error {
		savedProvider, savedModel = provider, model
		return nil
	}}
	panel := NewModelCatalogPanel(configuration)
	if panel.model.task.Value() != "" || panel.model.provider.Value() != "openai" {
		t.Fatal("Work model panel loaded the historical Code draft")
	}
	panel.Init()
	panel.model.localModels.action = localModelIdle
	next, _ := panel.model.useCloudModel(cloudModelEntry{provider: "openai", model: "selected", name: "selected", selectable: true})
	panel.model = next.(Model)
	if savedProvider != "openai" || savedModel != "selected" {
		t.Fatal("cloud selection was not persisted for Work")
	}
	panel.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !panel.Closed() {
		t.Fatal("Esc did not close the panel")
	}
	draft, found, err := journal.LoadDraft(state, source)
	if err != nil || !found || draft.Task != "private Code draft" || draft.Provider != "anthropic" || draft.Model != "legacy" {
		t.Fatalf("model panel changed Code draft: %+v %v", draft, err)
	}
	reopened := NewModelCatalogPanel(configuration)
	defer reopened.Close()
	reopened.Init()
	reopened.Update(catalogPanelMessage{panel: panel, message: localModelStatusMsg{generation: 1, catalog: LocalModelCatalog{RuntimeVersion: "stale"}}})
	if reopened.model.localModels.catalog.RuntimeVersion == "stale" {
		t.Fatal("closed panel's asynchronous response reached a new visit")
	}
}

func TestWorkModelPanelCloseCancelsPendingRefresh(t *testing.T) {
	manager := &refreshCancellationManager{entered: make(chan struct{}), cancelled: make(chan struct{})}
	panel := NewModelCatalogPanel(Config{LocalModels: manager})
	batch := panel.Init()().(tea.BatchMsg)
	done := make(chan struct{})
	go func() { defer close(done); batch[1]() }()
	<-manager.entered
	panel.Close()
	<-manager.cancelled
	<-done
}

type refreshCancellationManager struct {
	fakeLocalModelManager
	entered, cancelled chan struct{}
}

func TestWorkModelConfirmationRemainsVisibleInShortTerminal(t *testing.T) {
	panel := NewModelCatalogPanel(Config{LocalModels: &fakeLocalModelManager{}})
	defer panel.Close()
	panel.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	panel.model.localModels.section = localModelSection
	panel.model.localModels.catalog = LocalModelCatalog{Executable: "/usr/bin/ollama", RuntimeURL: "http://127.0.0.1:11434", RuntimeVersion: "fixture", HostSummary: strings.Repeat("host capacity ", 8)}
	for range 8 {
		panel.model.localModels.catalog.Models = append(panel.model.localModels.catalog.Models, LocalModel{ID: "fixture", Name: "Reviewed model", Download: "398 MB", Summary: "A reviewed local model for this fixture", SourceURL: "https://ollama.com/library/qwen2.5-coder"})
	}
	panel.model.localModels.confirmation = localModelConfirmPull
	view := panel.View()
	for _, required := range []string{"Confirm download", "398 MB", "https://ollama.com/library/qwen2.5-coder", "enter/y download", "esc/n cancel"} {
		if !strings.Contains(view, required) {
			t.Fatalf("confirmation hid %q:\n%s", required, view)
		}
	}
}

func TestWorkModelPanelUsesTheWorkVisualLanguage(t *testing.T) {
	panel := NewModelCatalogPanel(Config{RepositoryPath: "/work/gator", Provider: "gemini", Model: "gemini-3.5-flash", LocalModels: &fakeLocalModelManager{}})
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
			t.Fatalf("work-native model panel retained legacy chrome %q:\n%s", legacy, plain)
		}
	}
	if strings.Contains(view, "38;5;212") {
		t.Fatalf("work-native model panel retained the old pink accent:\n%q", view)
	}

	panel.Update(tea.KeyMsg{Type: tea.KeyF1})
	help := ansi.Strip(panel.View())
	if !strings.Contains(help, "Browse") || !strings.Contains(help, "Cloud") || !strings.Contains(help, "Local") || strings.Contains(help, "🐊 Gator  models") {
		t.Fatalf("model help did not use the focused Work layout:\n%s", help)
	}
}

func (m *refreshCancellationManager) Status(ctx context.Context) (LocalModelCatalog, error) {
	close(m.entered)
	<-ctx.Done()
	close(m.cancelled)
	return LocalModelCatalog{}, ctx.Err()
}
