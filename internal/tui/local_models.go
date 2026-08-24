package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/config"
)

// LocalModelManager owns the local runtime and persistent settings operations
// behind the TUI's reviewed local-model catalog. Implementations return a
// catalog with RuntimeError when the runtime is unavailable so the UI can
// explain recovery without losing the curated choices.
type LocalModelManager interface {
	Status(context.Context) (LocalModelCatalog, error)
	Pull(context.Context, string, func(LocalModelProgress)) (LocalModelCatalog, error)
	Use(context.Context, string) (LocalModelUpdate, error)
	Remove(context.Context, string) (LocalModelUpdate, error)
}

// LocalModelCatalog is the display-safe state of Gator's reviewed catalog.
// It deliberately contains no model weight, credential, or endpoint secrets.
type LocalModelCatalog struct {
	RuntimeURL     string
	RuntimeVersion string
	RuntimeError   string
	Executable     string
	Models         []LocalModel
}

// LocalModel is one selectable reviewed local coding model.
type LocalModel struct {
	ID          string
	OllamaModel string
	Name        string
	Download    string
	Context     string
	Summary     string
	SourceURL   string
	Installed   bool
}

// LocalModelProgress is a bounded, display-only pull update from the local
// runtime. It must not be interpreted as an instruction.
type LocalModelProgress struct {
	Status    string
	Completed int64
	Total     int64
}

// LocalModelUpdate returns a fresh catalog plus the configuration that should
// take effect in the current TUI process after a use or removal operation.
type LocalModelUpdate struct {
	Catalog         LocalModelCatalog
	CustomProviders []config.CustomProvider
	Provider        string
	Model           string
}

type localModelAction uint8

const (
	localModelIdle localModelAction = iota
	localModelRefreshing
	localModelPulling
	localModelUsing
	localModelRemoving
)

type localModelConfirmation uint8

const (
	localModelNoConfirmation localModelConfirmation = iota
	localModelConfirmPull
	localModelConfirmRemove
)

type localModelOperation struct {
	cancel   context.CancelFunc
	progress chan LocalModelProgress
	done     chan localModelOperationDone
}

type localModelOperationDone struct {
	catalog *LocalModelCatalog
	update  *LocalModelUpdate
	err     error
}

type localModelStatusMsg struct {
	catalog LocalModelCatalog
	err     error
}

type localModelProgressMsg struct {
	progress LocalModelProgress
}

type localModelDoneMsg struct {
	done localModelOperationDone
}

type localModelsState struct {
	manager      LocalModelManager
	catalog      LocalModelCatalog
	selected     int
	action       localModelAction
	confirmation localModelConfirmation
	operation    *localModelOperation
	progress     LocalModelProgress
	spinner      spinner.Model
	err          error
}

func newLocalModelsState(manager LocalModelManager, spinner spinner.Model) localModelsState {
	return localModelsState{manager: manager, spinner: spinner}
}

func (m Model) openLocalModels() (tea.Model, tea.Cmd) {
	if m.localModels.manager == nil {
		m.notice = notice{text: "Local model management is unavailable in this TUI session.", kind: noticeError}
		return m, nil
	}
	m.screen = localModelsScreen
	m.localModels.confirmation = localModelNoConfirmation
	m.localModels.err = nil
	return m.beginLocalStatus()
}

func (m Model) beginLocalStatus() (tea.Model, tea.Cmd) {
	if m.localModels.manager == nil {
		return m, nil
	}
	m.cancelLocalOperation()
	m.localModels.action = localModelRefreshing
	m.localModels.err = nil
	return m, tea.Batch(m.localModels.spinner.Tick, loadLocalModelStatus(m.localModels.manager))
}

func loadLocalModelStatus(manager LocalModelManager) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		catalog, err := manager.Status(ctx)
		return localModelStatusMsg{catalog: catalog, err: err}
	}
}

func (m *Model) cancelLocalOperation() {
	if m.localModels.operation != nil {
		m.localModels.operation.cancel()
		m.localModels.operation = nil
	}
}

func (m Model) updateLocalModels(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.localModels.confirmation != localModelNoConfirmation {
		return m.updateLocalModelConfirmation(message)
	}
	if m.localModels.action != localModelIdle {
		switch message.String() {
		case "ctrl+c", "esc":
			m.cancelLocalOperation()
			m.localModels.action = localModelIdle
			m.localModels.err = nil
			m.notice = notice{text: "Local model operation cancellation requested.", kind: noticeInfo}
		}
		return m, nil
	}
	switch message.String() {
	case "esc", "q":
		m.screen = composeScreen
		m.notice = notice{text: "Returned to the composer. Selected local model remains available for the next run.", kind: noticeInfo}
		return m, m.focusField()
	case "r":
		return m.beginLocalStatus()
	case "up", "k", "ctrl+p":
		m.moveLocalModelSelection(-1)
	case "down", "j", "ctrl+n":
		m.moveLocalModelSelection(1)
	case "p":
		if _, found := m.selectedLocalModel(); !found {
			m.notice = notice{text: "No curated local model is available to download.", kind: noticeError}
			return m, nil
		}
		m.localModels.confirmation = localModelConfirmPull
	case "u", "enter":
		model, found := m.selectedLocalModel()
		if !found {
			m.notice = notice{text: "No curated local model is selected.", kind: noticeError}
			return m, nil
		}
		if !model.Installed {
			m.notice = notice{text: model.Name + " is not installed. Press p to review and start its download.", kind: noticeInfo}
			return m, nil
		}
		return m.beginLocalUse(model)
	case "x", "delete":
		model, found := m.selectedLocalModel()
		if !found || !model.Installed {
			m.notice = notice{text: "Select an installed curated model before removing it.", kind: noticeInfo}
			return m, nil
		}
		m.localModels.confirmation = localModelConfirmRemove
	}
	return m, nil
}

func (m Model) updateLocalModelConfirmation(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	model, found := m.selectedLocalModel()
	if !found {
		m.localModels.confirmation = localModelNoConfirmation
		m.notice = notice{text: "The selected local model is no longer available.", kind: noticeError}
		return m, nil
	}
	switch message.String() {
	case "esc", "n", "ctrl+c":
		m.localModels.confirmation = localModelNoConfirmation
		m.notice = notice{text: "Local model operation cancelled before it started.", kind: noticeInfo}
		return m, nil
	case "enter", "y":
		confirmation := m.localModels.confirmation
		m.localModels.confirmation = localModelNoConfirmation
		switch confirmation {
		case localModelConfirmPull:
			return m.beginLocalPull(model)
		case localModelConfirmRemove:
			return m.beginLocalRemove(model)
		}
	}
	return m, nil
}

func (m Model) beginLocalPull(model LocalModel) (tea.Model, tea.Cmd) {
	if m.localModels.manager == nil {
		return m, nil
	}
	m.cancelLocalOperation()
	m.localModels.action = localModelPulling
	m.localModels.progress = LocalModelProgress{Status: "starting download"}
	m.localModels.err = nil
	operation := startLocalModelOperation(func(ctx context.Context, report func(LocalModelProgress)) localModelOperationDone {
		catalog, err := m.localModels.manager.Pull(ctx, model.ID, report)
		return localModelOperationDone{catalog: &catalog, err: err}
	})
	m.localModels.operation = operation
	return m, tea.Batch(m.localModels.spinner.Tick, waitForLocalModelOperation(operation))
}

func (m Model) beginLocalUse(model LocalModel) (tea.Model, tea.Cmd) {
	if m.localModels.manager == nil {
		return m, nil
	}
	m.cancelLocalOperation()
	m.localModels.action = localModelUsing
	m.localModels.err = nil
	operation := startLocalModelOperation(func(ctx context.Context, _ func(LocalModelProgress)) localModelOperationDone {
		update, err := m.localModels.manager.Use(ctx, model.ID)
		return localModelOperationDone{update: &update, err: err}
	})
	m.localModels.operation = operation
	return m, tea.Batch(m.localModels.spinner.Tick, waitForLocalModelOperation(operation))
}

func (m Model) beginLocalRemove(model LocalModel) (tea.Model, tea.Cmd) {
	if m.localModels.manager == nil {
		return m, nil
	}
	m.cancelLocalOperation()
	m.localModels.action = localModelRemoving
	m.localModels.err = nil
	operation := startLocalModelOperation(func(ctx context.Context, _ func(LocalModelProgress)) localModelOperationDone {
		update, err := m.localModels.manager.Remove(ctx, model.ID)
		return localModelOperationDone{update: &update, err: err}
	})
	m.localModels.operation = operation
	return m, tea.Batch(m.localModels.spinner.Tick, waitForLocalModelOperation(operation))
}

func startLocalModelOperation(run func(context.Context, func(LocalModelProgress)) localModelOperationDone) *localModelOperation {
	ctx, cancel := context.WithCancel(context.Background())
	operation := &localModelOperation{
		cancel:   cancel,
		progress: make(chan LocalModelProgress, 16),
		done:     make(chan localModelOperationDone, 1),
	}
	go func() {
		defer close(operation.progress)
		done := run(ctx, func(update LocalModelProgress) {
			select {
			case operation.progress <- update:
			case <-ctx.Done():
			}
		})
		operation.done <- done
	}()
	return operation
}

func waitForLocalModelOperation(operation *localModelOperation) tea.Cmd {
	return func() tea.Msg {
		select {
		case progress, open := <-operation.progress:
			if open {
				return localModelProgressMsg{progress: progress}
			}
			return localModelDoneMsg{done: <-operation.done}
		case done := <-operation.done:
			return localModelDoneMsg{done: done}
		}
	}
}

func (m Model) selectedLocalModel() (LocalModel, bool) {
	if len(m.localModels.catalog.Models) == 0 {
		return LocalModel{}, false
	}
	index := min(max(0, m.localModels.selected), len(m.localModels.catalog.Models)-1)
	return m.localModels.catalog.Models[index], true
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
	m.localModels.selected = 0
	for index, model := range catalog.Models {
		if model.ID == selectedID {
			m.localModels.selected = index
			break
		}
	}
}

func (m *Model) applyLocalModelUpdate(update LocalModelUpdate) {
	m.applyLocalModelCatalog(update.Catalog)
	m.config.CustomProviders = append([]config.CustomProvider(nil), update.CustomProviders...)
	m.provider.SetValue(update.Provider)
	m.model.SetValue(update.Model)
	m.delegateRuntime = ""
	m.persistDraft()
	m.refreshPreflight()
}

func (m Model) localModelActionLabel() string {
	switch m.localModels.action {
	case localModelRefreshing:
		return "checking local runtime"
	case localModelPulling:
		return "downloading selected model"
	case localModelUsing:
		return "selecting local model"
	case localModelRemoving:
		return "removing selected model"
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
