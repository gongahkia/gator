package modelcatalog

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) openModelCatalog() (tea.Model, tea.Cmd) {
	if m.localModels.manager == nil {
		m.notice = notice{text: "Model management is unavailable in this TUI session.", kind: noticeError}
		return m, nil
	}
	m.screen = localModelsScreen
	m.localModels.confirmation = localModelNoConfirmation
	m.localModels.dependencyHelp = false
	m.localModels.err = nil
	m.selectActiveModelCatalogEntry()
	return m.beginLocalStatus()
}

// openLocalModels remains a private compatibility bridge for TUI callers that
// predate the unified model catalog. The user-facing command is /model.
func (m Model) openLocalModels() (tea.Model, tea.Cmd) {
	return m.openModelCatalog()
}

func (m Model) beginLocalStatus() (tea.Model, tea.Cmd) {
	if m.localModels.manager == nil {
		return m, nil
	}
	m.cancelLocalOperation()
	m.localModels.generation++
	m.localModels.action = localModelRefreshing
	m.localModels.err = nil
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	m.localModels.statusCancel = cancel
	commands := []tea.Cmd{m.localModels.spinner.Tick, loadLocalModelStatus(ctx, cancel, m.localModels.manager, m.localModels.generation)}
	if m.config.ModelManagement != nil {
		commands = append(commands, loadCredentialStatuses(m.config.ModelManagement))
	}
	return m, tea.Batch(commands...)
}

func loadCredentialStatuses(backend ManagementBackend) tea.Cmd {
	return func() tea.Msg {
		statuses, err := backend.CredentialStatuses()
		return credentialStatusesMsg{statuses: statuses, err: err}
	}
}

func loadLocalModelStatus(ctx context.Context, cancel context.CancelFunc, manager LocalManager, generation uint64) tea.Cmd {
	return func() tea.Msg {
		defer cancel()
		catalog, err := manager.Status(ctx)
		return localModelStatusMsg{generation: generation, catalog: catalog, err: err}
	}
}

func (m *Model) cancelLocalOperation() {
	if m.localModels.statusCancel != nil {
		m.localModels.statusCancel()
		m.localModels.statusCancel = nil
	}
	if m.localModels.operation != nil {
		m.localModels.operation.cancel()
		m.localModels.operation = nil
	}
}

func (m Model) updateLocalModels(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if message.String() == "ctrl+c" && m.oauthLogin != nil {
		m.oauthCancel()
		m.oauthLogin.Cancel()
		m.notice = notice{text: "OAuth login cancellation requested.", kind: noticeInfo}
		return m, nil
	}
	if m.localModels.cloudSetup != nil {
		return m.updateCloudModelSetup(message)
	}
	if m.localModels.confirmation != localModelNoConfirmation {
		return m.updateLocalModelConfirmation(message)
	}
	if m.localModels.customSetup != nil {
		return m.updateCustomProviderSetup(message)
	}
	if m.localModels.renaming != nil {
		return m.updateModelRename(message)
	}
	if m.localModels.dependencyHelp {
		switch message.String() {
		case "esc", "i":
			m.localModels.dependencyHelp = false
			m.notice = notice{text: "Installation help closed.", kind: noticeInfo}
		}
		return m, nil
	}
	if m.localModels.confirmation != localModelNoConfirmation {
		return m.updateLocalModelConfirmation(message)
	}
	if m.localModels.action != localModelIdle && m.localModels.action != localModelRefreshing {
		switch message.String() {
		case "ctrl+c", "esc":
			m.cancelLocalOperation()
			m.localModels.generation++
			m.localModels.action = localModelIdle
			m.localModels.err = nil
			m.notice = notice{text: "Local model operation cancellation requested.", kind: noticeInfo}
		}
		return m, nil
	}
	switch message.String() {
	case "esc", "q":
		if m.localModels.action == localModelRefreshing {
			m.localModels.generation++
			m.localModels.action = localModelIdle
		}
		m.screen = closedScreen
		m.notice = notice{text: "Returned to the composer. The selected model remains available for the next run.", kind: noticeInfo}
		return m, m.focusField()
	case "r":
		return m.beginLocalStatus()
	case "s":
		if m.localModels.section != localModelSection || m.localModels.catalog.RuntimeError == "" {
			return m, nil
		}
		m.requestLocalRuntimeRecovery()
	case "i":
		if m.localModels.section != localModelSection {
			m.notice = notice{text: "Installation help is available from the Local section.", kind: noticeInfo}
			return m, nil
		}
		if len(m.localMissingDependencies()) == 0 {
			m.notice = notice{text: "No missing local dependencies were detected.", kind: noticeSuccess}
			return m, nil
		}
		m.localModels.dependencyHelp = true
	case "tab", "left", "right":
		m.toggleModelCatalogSection()
		if m.localModels.section == localModelSection && m.localModels.catalog.RuntimeError != "" && !m.localModels.startDismissed {
			m.localModels.confirmation = m.localRuntimeConfirmation()
		}
	case "up", "k", "ctrl+p":
		m.moveModelCatalogSelection(-1)
	case "down", "j", "ctrl+n":
		m.moveModelCatalogSelection(1)
	case "e":
		return m.beginModelRename()
	case "n":
		if m.localModels.section != cloudModelSection {
			m.notice = notice{text: "Custom providers are created from the Cloud section.", kind: noticeInfo}
			return m, nil
		}
		return m.beginCustomProviderCreate()
	case "c":
		if m.localModels.section != cloudModelSection {
			m.notice = notice{text: "Cloud configuration is available from the Cloud section.", kind: noticeInfo}
			return m, nil
		}
		return m.beginCustomProviderEdit()
	case "g":
		return m.beginCustomProviderDiscover()
	case "d":
		return m.beginRemoveCloudCredential()
	case "l":
		if m.localModels.section != cloudModelSection {
			m.notice = notice{text: "Cloud sign-in is available from the Cloud section.", kind: noticeInfo}
			return m, nil
		}
		cloud, found := m.selectedCloudModel()
		if !found {
			m.notice = notice{text: "No cloud model is selected.", kind: noticeError}
			return m, nil
		}
		if !cloud.canLogin {
			m.notice = notice{text: "This provider uses " + cloud.status + ".", kind: noticeInfo}
			return m, nil
		}
		return m.startOAuthLogin(cloud.provider)
	case "p":
		if m.localModels.section != localModelSection {
			m.notice = notice{text: "Downloads are available only for reviewed local models.", kind: noticeInfo}
			return m, nil
		}
		if m.localModels.catalog.RuntimeError != "" {
			m.requestLocalRuntimeRecovery()
			return m, nil
		}
		model, found := m.selectedLocalModel()
		if !found {
			m.notice = notice{text: "No curated local model is available to download.", kind: noticeError}
			return m, nil
		}
		if model.BlockedReason != "" {
			m.notice = notice{text: model.Name + " is disabled on this host: " + model.BlockedReason, kind: noticeError}
			return m, nil
		}
		m.localModels.confirmation = localModelConfirmPull
	case "u", "enter":
		if m.localModels.section == cloudModelSection {
			cloud, found := m.selectedCloudModel()
			if !found {
				m.notice = notice{text: "No cloud model is selected.", kind: noticeError}
				return m, nil
			}
			return m.useCloudModel(cloud)
		}
		if m.localModels.catalog.RuntimeError != "" {
			m.requestLocalRuntimeRecovery()
			return m, nil
		}
		model, found := m.selectedLocalModel()
		if !found {
			m.notice = notice{text: "No curated local model is selected.", kind: noticeError}
			return m, nil
		}
		if !model.Installed {
			m.notice = notice{text: model.Name + " is not installed. Press p to review and start its download.", kind: noticeInfo}
			return m, nil
		}
		if model.BlockedReason != "" {
			m.notice = notice{text: model.Name + " is disabled on this host: " + model.BlockedReason, kind: noticeError}
			return m, nil
		}
		return m.beginLocalUse(model)
	case "x", "delete":
		if m.localModels.section != localModelSection {
			return m.beginRemoveCustomProvider()
		}
		if m.localModels.catalog.RuntimeError != "" {
			if m.ollamaMissing() {
				m.notice = notice{text: "Ollama is not installed. Press i for installation help, then r to refresh.", kind: noticeError}
			} else {
				m.notice = notice{text: "The local runtime is unavailable. Start Ollama, then press r to refresh.", kind: noticeError}
			}
			return m, nil
		}
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
	if m.localModels.confirmation == localModelConfirmInstall {
		switch message.String() {
		case "esc", "n", "ctrl+c":
			m.localModels.confirmation = localModelNoConfirmation
			m.localModels.startDismissed = true
			m.notice = notice{text: "Install Ollama yourself from https://ollama.com/download, then press r to refresh.", kind: noticeInfo}
		case "enter", "y":
			m.localModels.confirmation = localModelNoConfirmation
			m.localModels.startDismissed = true
			m.localModels.dependencyHelp = true
			m.notice = notice{text: "Installation help is open. Gator does not run system installers or package managers.", kind: noticeInfo}
		}
		return m, nil
	}
	if m.localModels.confirmation == localModelConfirmStart {
		switch message.String() {
		case "esc", "n", "ctrl+c":
			m.localModels.confirmation = localModelNoConfirmation
			m.localModels.startDismissed = true
			m.notice = notice{text: "Runtime start declined. Press s to start it with Gator later, or r to refresh.", kind: noticeInfo}
			return m, nil
		case "enter", "y":
			m.localModels.confirmation = localModelNoConfirmation
			m.localModels.startDismissed = false
			return m.beginLocalStart()
		}
		return m, nil
	}
	switch message.String() {
	case "esc", "n", "ctrl+c":
		confirmation := m.localModels.confirmation
		m.localModels.confirmation = localModelNoConfirmation
		m.localModels.pendingCustom = nil
		m.localModels.discovery = nil
		switch confirmation {
		case localModelConfirmRemoveCredential:
			m.notice = notice{text: "Credential removal cancelled. The stored Gator credential was not changed.", kind: noticeInfo}
		case localModelConfirmRemoveCustom:
			m.notice = notice{text: "Custom provider removal cancelled. Configuration was not changed.", kind: noticeInfo}
		case localModelConfirmSaveCustom:
			m.notice = notice{text: "Custom provider save cancelled. Configuration was not written.", kind: noticeInfo}
		case localModelConfirmApplyDiscovery:
			m.notice = notice{text: "Model catalog replacement cancelled. Discovered IDs were not saved.", kind: noticeInfo}
		default:
			m.notice = notice{text: "Local model operation cancelled before it started.", kind: noticeInfo}
		}
		return m, nil
	case "enter", "y":
		confirmation := m.localModels.confirmation
		m.localModels.confirmation = localModelNoConfirmation
		switch confirmation {
		case localModelConfirmPull:
			model, found := m.selectedLocalModel()
			if !found {
				m.notice = notice{text: "The selected local model is no longer available.", kind: noticeError}
				return m, nil
			}
			return m.beginLocalPull(model)
		case localModelConfirmRemove:
			model, found := m.selectedLocalModel()
			if !found {
				m.notice = notice{text: "The selected local model is no longer available.", kind: noticeError}
				return m, nil
			}
			return m.beginLocalRemove(model)
		case localModelConfirmRemoveCredential:
			return m.removeCloudCredential()
		case localModelConfirmRemoveCustom:
			return m.removeCustomProvider()
		case localModelConfirmSaveCustom:
			return m.saveReviewedCustomProvider()
		case localModelConfirmApplyDiscovery:
			return m.applyCustomProviderDiscovery()
		}
	}
	return m, nil
}

func (m *Model) requestLocalRuntimeRecovery() {
	m.localModels.confirmation = m.localRuntimeConfirmation()
	m.localModels.startDismissed = false
	if m.localModels.confirmation == localModelConfirmInstall {
		m.notice = notice{text: "Ollama is not installed. Choose whether to open installation help.", kind: noticeInfo}
		return
	}
	m.notice = notice{text: "The local runtime is unavailable. Choose whether Gator should start Ollama.", kind: noticeInfo}
}

func (m Model) localRuntimeConfirmation() localModelConfirmation {
	if m.ollamaMissing() {
		return localModelConfirmInstall
	}
	return localModelConfirmStart
}

func (m Model) ollamaMissing() bool {
	for _, dependency := range m.localModels.catalog.Dependencies {
		if dependency.ID == "ollama" {
			return !dependency.Installed
		}
	}
	return m.localModels.catalog.Executable == ""
}

func (m Model) localMissingDependencies() []LocalDependency {
	missing := make([]LocalDependency, 0, len(m.localModels.catalog.Dependencies))
	foundOllama := false
	for _, dependency := range m.localModels.catalog.Dependencies {
		if dependency.ID == "ollama" {
			foundOllama = true
		}
		if !dependency.Installed {
			missing = append(missing, dependency)
		}
	}
	if !foundOllama && m.localModels.catalog.Executable == "" {
		missing = append(missing, LocalDependency{
			ID:           "ollama",
			Name:         "Ollama",
			Purpose:      "reviewed local coding models",
			HelpURL:      "https://ollama.com/download",
			Instructions: []string{"Install Ollama from its official download page, then return to Gator and refresh the Local model section."},
		})
	}
	return missing
}

func (m Model) beginLocalStart() (tea.Model, tea.Cmd) {
	if m.localModels.manager == nil {
		return m, nil
	}
	m.cancelLocalOperation()
	m.localModels.action = localModelStarting
	m.localModels.progress = LocalProgress{Status: "starting Ollama"}
	m.localModels.err = nil
	operation := startLocalModelOperation(func(ctx context.Context, _ func(LocalProgress)) localModelOperationDone {
		catalog, err := m.localModels.manager.Start(ctx)
		return localModelOperationDone{catalog: &catalog, err: err}
	})
	m.localModels.operation = operation
	return m, tea.Batch(m.localModels.spinner.Tick, waitForLocalModelOperation(operation))
}

func (m Model) beginLocalPull(model LocalModel) (tea.Model, tea.Cmd) {
	if m.localModels.manager == nil {
		return m, nil
	}
	m.cancelLocalOperation()
	m.localModels.action = localModelPulling
	m.localModels.progress = LocalProgress{Status: "starting download"}
	m.localModels.err = nil
	operation := startLocalModelOperation(func(ctx context.Context, report func(LocalProgress)) localModelOperationDone {
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
	operation := startLocalModelOperation(func(ctx context.Context, _ func(LocalProgress)) localModelOperationDone {
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
	operation := startLocalModelOperation(func(ctx context.Context, _ func(LocalProgress)) localModelOperationDone {
		update, err := m.localModels.manager.Remove(ctx, model.ID)
		return localModelOperationDone{update: &update, err: err}
	})
	m.localModels.operation = operation
	return m, tea.Batch(m.localModels.spinner.Tick, waitForLocalModelOperation(operation))
}

func startLocalModelOperation(run func(context.Context, func(LocalProgress)) localModelOperationDone) *localModelOperation {
	ctx, cancel := context.WithCancel(context.Background())
	operation := &localModelOperation{
		cancel:   cancel,
		progress: make(chan LocalProgress, 16),
		done:     make(chan localModelOperationDone, 1),
	}
	go func() {
		defer cancel()
		defer close(operation.progress)
		done := run(ctx, func(update LocalProgress) {
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
				return localModelProgressMsg{operation: operation, progress: progress}
			}
			return localModelDoneMsg{operation: operation, done: <-operation.done}
		case done := <-operation.done:
			return localModelDoneMsg{operation: operation, done: done}
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
