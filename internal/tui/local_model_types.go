package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/gongahkia/gator/internal/config"
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
	localModelDiscovering
)

type localModelConfirmation uint8

const (
	localModelNoConfirmation localModelConfirmation = iota
	localModelConfirmStart
	localModelConfirmInstall
	localModelConfirmPull
	localModelConfirmRemove
	localModelConfirmRemoveCredential
	localModelConfirmRemoveCustom
	localModelConfirmSaveCustom
	localModelConfirmApplyDiscovery
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
	statusCancel   context.CancelFunc
	progress       LocalModelProgress
	spinner        spinner.Model
	err            error
	generation     uint64
	section        modelCatalogSection
	cloudIndex     int
	renaming       *modelRename
	cloudSetup     *cloudModelSetupForm
	customSetup    *customProviderSetupForm
	pendingCustom  *CustomProviderSetup
	discovery      *CustomProviderDiscovery
	credentials    map[string]StoredCredentialStatus
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
	custom     bool
}

type modelRename struct {
	provider string
	model    string
	input    textinput.Model
}

func newLocalModelsState(manager LocalModelManager, spinner spinner.Model) localModelsState {
	return localModelsState{manager: manager, spinner: spinner}
}
