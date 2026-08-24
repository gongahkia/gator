package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	modelprovider "github.com/gongahkia/gator/internal/model"
)

// LocalModelManager owns the local runtime and persistent settings operations
// behind the TUI's reviewed local-model catalog. Implementations return a
// catalog with RuntimeError when the runtime is unavailable so the UI can
// explain recovery without losing the curated choices.
type LocalModelManager interface {
	Status(context.Context) (LocalModelCatalog, error)
	Start(context.Context) (LocalModelCatalog, error)
	Pull(context.Context, string, func(LocalModelProgress)) (LocalModelCatalog, error)
	Use(context.Context, string) (LocalModelUpdate, error)
	Remove(context.Context, string) (LocalModelUpdate, error)
	Rename(context.Context, string, string, string) (map[string]string, error)
}

// LocalModelCatalog is the display-safe state of Gator's reviewed catalog.
// It deliberately contains no model weight, credential, or endpoint secrets.
type LocalModelCatalog struct {
	RuntimeURL     string
	RuntimeVersion string
	RuntimeError   string
	Executable     string
	HostSummary    string
	HostAdvice     []string
	Dependencies   []LocalDependency
	Models         []LocalModel
}

// LocalDependency is a display-safe prerequisite result supplied by the
// application layer. Instructions are guidance only; the TUI never runs an
// operating-system installer or package manager.
type LocalDependency struct {
	ID           string
	Name         string
	Purpose      string
	Required     bool
	Installed    bool
	HelpURL      string
	Instructions []string
}

// LocalModel is one selectable reviewed local coding model.
type LocalModel struct {
	ID            string
	OllamaModel   string
	Name          string
	DefaultName   string
	Download      string
	Context       string
	Summary       string
	SourceURL     string
	Installed     bool
	Requirement   string
	BlockedReason string
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
	ModelAliases    map[string]string
	Provider        string
	Model           string
}

type localModelAction uint8

const (
	localModelIdle localModelAction = iota
	localModelRefreshing
	localModelPulling
	localModelStarting
	localModelUsing
	localModelRemoving
	localModelRenaming
)

type localModelConfirmation uint8

const (
	localModelNoConfirmation localModelConfirmation = iota
	localModelConfirmStart
	localModelConfirmInstall
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
	aliases map[string]string
	err     error
}

type localModelStatusMsg struct {
	generation uint64
	catalog    LocalModelCatalog
	err        error
}

type localModelProgressMsg struct {
	operation *localModelOperation
	progress  LocalModelProgress
}

type localModelDoneMsg struct {
	operation *localModelOperation
	done      localModelOperationDone
}

type localModelsState struct {
	manager        LocalModelManager
	catalog        LocalModelCatalog
	selected       int
	action         localModelAction
	confirmation   localModelConfirmation
	operation      *localModelOperation
	progress       LocalModelProgress
	spinner        spinner.Model
	err            error
	generation     uint64
	section        modelCatalogSection
	cloudIndex     int
	renaming       *modelRename
	startDismissed bool
	dependencyHelp bool
}

type modelCatalogSection uint8

const (
	cloudModelSection modelCatalogSection = iota
	localModelSection
)

type cloudModelEntry struct {
	provider   string
	model      string
	name       string
	status     string
	selectable bool
	canLogin   bool
}

type modelRename struct {
	provider string
	model    string
	input    textinput.Model
}

func newLocalModelsState(manager LocalModelManager, spinner spinner.Model) localModelsState {
	return localModelsState{manager: manager, spinner: spinner}
}

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
	return m, tea.Batch(m.localModels.spinner.Tick, loadLocalModelStatus(m.localModels.manager, m.localModels.generation))
}

func loadLocalModelStatus(manager LocalModelManager, generation uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		catalog, err := manager.Status(ctx)
		return localModelStatusMsg{generation: generation, catalog: catalog, err: err}
	}
}

func (m *Model) cancelLocalOperation() {
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
		m.screen = composeScreen
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
			m.notice = notice{text: "Cloud models are managed by their provider. Gator only removes local model weights here.", kind: noticeInfo}
			return m, nil
		}
		if m.localModels.catalog.RuntimeError != "" {
			m.notice = notice{text: "The local runtime is unavailable. Start Ollama, then press r to refresh.", kind: noticeError}
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
			m.notice = notice{text: "Start Ollama yourself with 'ollama serve' or 'gator local serve', then press r to refresh.", kind: noticeInfo}
			return m, nil
		case "enter", "y":
			m.localModels.confirmation = localModelNoConfirmation
			m.localModels.startDismissed = false
			return m.beginLocalStart()
		}
		return m, nil
	}
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
	m.localModels.progress = LocalModelProgress{Status: "starting Ollama"}
	m.localModels.err = nil
	operation := startLocalModelOperation(func(ctx context.Context, _ func(LocalModelProgress)) localModelOperationDone {
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

func (m Model) cloudModels() []cloudModelEntry {
	entries := make([]cloudModelEntry, 0, len(modelprovider.Names())+len(m.config.CustomProviders))
	credentialStore, credentialStoreErr := auth.New(m.config.StateDir)
	for _, providerName := range modelprovider.Names() {
		provider, err := modelprovider.ParseProvider(providerName)
		if err != nil || provider == modelprovider.Claude {
			continue
		}
		modelName := modelprovider.DefaultModel(provider)
		if providerName == strings.TrimSpace(m.provider.Value()) && strings.TrimSpace(m.model.Value()) != "" {
			modelName = strings.TrimSpace(m.model.Value())
		}
		name := providerName
		selectable := modelName != ""
		if selectable {
			name += " · " + m.modelDisplayName(providerName, modelName, modelName)
		} else {
			name += " · account model required"
		}
		status := cloudProviderStatus(provider, credentialStore, credentialStoreErr)
		entries = append(entries, cloudModelEntry{
			provider:   providerName,
			model:      modelName,
			name:       name,
			status:     status,
			selectable: selectable,
			canLogin:   modelprovider.SupportsOAuthLogin(provider),
		})
	}
	for _, provider := range m.config.CustomProviders {
		for _, modelName := range provider.Models {
			name := provider.ID + " · " + m.modelDisplayName(provider.ID, modelName, modelName)
			status := "configured custom endpoint"
			if provider.APIKeyEnv == "" {
				status += " · no API key"
			} else if strings.TrimSpace(os.Getenv(provider.APIKeyEnv)) != "" {
				status += " · API key set"
			} else {
				status += " · requires " + provider.APIKeyEnv
			}
			entries = append(entries, cloudModelEntry{provider: provider.ID, model: modelName, name: name, status: status, selectable: true})
		}
	}
	return entries
}

func cloudProviderStatus(provider modelprovider.Provider, store auth.Store, storeErr error) string {
	if storeErr == nil {
		credential, found, err := store.Read(string(provider))
		if err == nil && found {
			if credential.Expired(time.Now()) {
				return "stored credential expired"
			}
			if credential.IsOAuth() {
				return "signed in"
			}
			return "Gator credential stored"
		}
	}
	if modelprovider.AmbientCredentialAvailable(provider) {
		if source := modelprovider.AmbientCredentialSource(provider); source != "" {
			return "ambient credentials: " + source
		}
		return "ambient credentials configured"
	}
	if environment := modelprovider.APIKeyEnvironment(provider); environment != "" && strings.TrimSpace(os.Getenv(environment)) != "" {
		return "API key set: " + environment
	}
	if modelprovider.SupportsOAuthLogin(provider) {
		return "sign-in available"
	}
	return "requires " + modelprovider.CredentialHint(provider)
}

func (m Model) selectedCloudModel() (cloudModelEntry, bool) {
	entries := m.cloudModels()
	if len(entries) == 0 {
		return cloudModelEntry{}, false
	}
	index := min(max(0, m.localModels.cloudIndex), len(entries)-1)
	return entries[index], true
}

func (m *Model) selectActiveModelCatalogEntry() {
	if strings.EqualFold(strings.TrimSpace(m.provider.Value()), "gator-local") {
		m.localModels.section = localModelSection
		for index, local := range m.localModels.catalog.Models {
			if local.OllamaModel == strings.TrimSpace(m.model.Value()) {
				m.localModels.selected = index
				break
			}
		}
		return
	}
	m.localModels.section = cloudModelSection
	for index, cloud := range m.cloudModels() {
		if cloud.provider == strings.TrimSpace(m.provider.Value()) && cloud.model == strings.TrimSpace(m.model.Value()) {
			m.localModels.cloudIndex = index
			return
		}
	}
}

func (m *Model) toggleModelCatalogSection() {
	if m.localModels.section == cloudModelSection {
		m.localModels.section = localModelSection
		return
	}
	m.localModels.section = cloudModelSection
}

func (m *Model) moveModelCatalogSelection(delta int) {
	if m.localModels.section == localModelSection {
		m.moveLocalModelSelection(delta)
		return
	}
	entries := m.cloudModels()
	if len(entries) == 0 {
		m.localModels.cloudIndex = 0
		return
	}
	m.localModels.cloudIndex = (m.localModels.cloudIndex + delta + len(entries)) % len(entries)
}

func (m Model) useCloudModel(cloud cloudModelEntry) (tea.Model, tea.Cmd) {
	if !cloud.selectable {
		m.notice = notice{text: "This provider requires an account-specific deployment or model ID. Use the provider field to enter it before a run.", kind: noticeInfo}
		return m, nil
	}
	m.provider.SetValue(cloud.provider)
	m.model.SetValue(cloud.model)
	m.delegateRuntime = ""
	m.persistDraft()
	m.refreshPreflight()
	m.notice = notice{text: "Selected " + cloud.name + ". Return to the composer and send a task when its readiness is configured.", kind: noticeSuccess}
	return m, nil
}

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
