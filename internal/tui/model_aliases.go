package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/config"
)

func (m Model) beginModelRename() (tea.Model, tea.Cmd) {
	provider, model, found := m.selectedModelIdentity()
	if !found {
		m.notice = notice{text: "Select a model before changing its display name.", kind: noticeError}
		return m, nil
	}
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "display name (empty restores the original)"
	input.CharLimit = 128
	input.Width = max(24, m.inlineWidth()-2)
	input.SetValue(m.modelAlias(provider, model))
	command := input.Focus()
	m.localModels.renaming = &modelRename{provider: provider, model: model, input: input}
	return m, command
}

func (m Model) updateModelRename(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.localModels.renaming == nil {
		return m, nil
	}
	switch message.String() {
	case "esc", "ctrl+c":
		m.localModels.renaming = nil
		m.notice = notice{text: "Model display-name change cancelled.", kind: noticeInfo}
		return m, nil
	case "enter":
		rename := m.localModels.renaming
		if m.localModels.manager == nil {
			m.notice = notice{text: "Model management is unavailable in this TUI session.", kind: noticeError}
			return m, nil
		}
		m.localModels.renaming = nil
		m.localModels.action = localModelRenaming
		operation := startLocalModelOperation(func(ctx context.Context, _ func(LocalModelProgress)) localModelOperationDone {
			aliases, err := m.localModels.manager.Rename(ctx, rename.provider, rename.model, rename.input.Value())
			return localModelOperationDone{aliases: aliases, err: err}
		})
		m.localModels.operation = operation
		return m, tea.Batch(m.localModels.spinner.Tick, waitForLocalModelOperation(operation))
	}
	var command tea.Cmd
	m.localModels.renaming.input, command = m.localModels.renaming.input.Update(message)
	return m, command
}

func (m Model) selectedModelIdentity() (string, string, bool) {
	if m.localModels.section == localModelSection {
		local, found := m.selectedLocalModel()
		if !found {
			return "", "", false
		}
		return "gator-local", local.OllamaModel, true
	}
	cloud, found := m.selectedCloudModel()
	if !found || !cloud.selectable {
		return "", "", false
	}
	return cloud.provider, cloud.model, true
}

func (m Model) modelAlias(provider, model string) string {
	return strings.TrimSpace(m.config.ModelAliases[config.ModelAliasKey(provider, model)])
}

func (m Model) modelDisplayName(provider, model, fallback string) string {
	if alias := m.modelAlias(provider, model); alias != "" {
		return alias
	}
	return fallback
}

func (m *Model) moveLocalModelSelection(delta int) {
	if len(m.localModels.catalog.Models) == 0 {
		m.localModels.selected = 0
		return
	}
	m.localModels.selected = (m.localModels.selected + delta + len(m.localModels.catalog.Models)) % len(m.localModels.catalog.Models)
}

func (m *Model) applyLocalModelCatalog(catalog LocalModelCatalog) {
	selectedID := ""
	if selected, found := m.selectedLocalModel(); found {
		selectedID = selected.ID
	}
	m.localModels.catalog = catalog
	m.applyModelAliases(m.config.ModelAliases)
	m.localModels.selected = 0
	for index, model := range catalog.Models {
		if model.ID == selectedID {
			m.localModels.selected = index
			break
		}
	}
}

func (m *Model) applyLocalModelUpdate(update LocalModelUpdate) {
	if update.ModelAliases != nil {
		m.applyModelAliases(update.ModelAliases)
	}
	m.applyLocalModelCatalog(update.Catalog)
	m.config.CustomProviders = append([]config.CustomProvider(nil), update.CustomProviders...)
	m.provider.SetValue(update.Provider)
	m.model.SetValue(update.Model)
	m.delegateRuntime = ""
	m.persistDraft()
	m.refreshPreflight()
}

func (m *Model) applyModelAliases(aliases map[string]string) {
	m.config.ModelAliases = cloneModelAliasMap(aliases)
	for index := range m.localModels.catalog.Models {
		model := &m.localModels.catalog.Models[index]
		fallback := model.DefaultName
		if fallback == "" {
			fallback = model.Name
		}
		model.Name = m.modelDisplayName("gator-local", model.OllamaModel, fallback)
	}
}

func cloneModelAliasMap(aliases map[string]string) map[string]string {
	result := make(map[string]string, len(aliases))
	for key, value := range aliases {
		result[key] = value
	}
	return result
}

func (m Model) localModelActionLabel() string {
	switch m.localModels.action {
	case localModelRefreshing:
		return "checking local runtime"
	case localModelPulling:
		return "downloading selected model"
	case localModelStarting:
		return "starting Ollama"
	case localModelUsing:
		return "selecting local model"
	case localModelRemoving:
		return "removing selected model"
	case localModelRenaming:
		return "saving model display name"
	case localModelDiscovering:
		return "requesting untrusted model catalog"
	default:
		return ""
	}
}

func (m Model) localModelProgressLabel() string {
	progress := m.localModels.progress
	status := strings.TrimSpace(progress.Status)
	if status == "" {
		status = "working"
	}
	if progress.Total > 0 && progress.Completed >= 0 {
		return fmt.Sprintf("%s (%d%%)", status, progress.Completed*100/progress.Total)
	}
	return status
}
