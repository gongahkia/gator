package modelcatalog

import (
	"context"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
)

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
	progress chan LocalProgress
	done     chan localModelOperationDone
}

type localModelOperationDone struct {
	catalog *LocalCatalog
	update  *LocalUpdate
	aliases map[string]string
	err     error
}

type localModelStatusMsg struct {
	generation uint64
	catalog    LocalCatalog
	err        error
}

type localModelProgressMsg struct {
	operation *localModelOperation
	progress  LocalProgress
}

type localModelDoneMsg struct {
	operation *localModelOperation
	done      localModelOperationDone
}

type localModelsState struct {
	manager        LocalManager
	catalog        LocalCatalog
	selected       int
	action         localModelAction
	confirmation   localModelConfirmation
	operation      *localModelOperation
	statusCancel   context.CancelFunc
	progress       LocalProgress
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

func newLocalModelsState(manager LocalManager, spinner spinner.Model) localModelsState {
	return localModelsState{manager: manager, spinner: spinner}
}
