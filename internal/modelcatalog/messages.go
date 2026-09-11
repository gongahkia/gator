package modelcatalog

import tea "github.com/charmbracelet/bubbletea"

func (m Model) updateCloudModelSetupSaved(msg cloudModelSetupSavedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		if m.localModels.cloudSetup != nil {
			m.localModels.cloudSetup.saving = false
		}
		m.notice = notice{text: "Save cloud model configuration: " + msg.err.Error(), kind: noticeError}
		return m, nil
	}
	m.localModels.cloudSetup = nil
	m.provider.SetValue(msg.provider)
	m.model.SetValue(msg.model)
	m.config.BaseURL = msg.baseURL
	if m.config.ProviderEndpoints == nil {
		m.config.ProviderEndpoints = make(map[string]string)
	}
	if msg.baseURL == "" {
		delete(m.config.ProviderEndpoints, msg.provider)
	} else {
		m.config.ProviderEndpoints[msg.provider] = msg.baseURL
	}
	if m.config.ProviderOptions == nil {
		m.config.ProviderOptions = make(map[string]map[string]string)
	}
	if len(msg.options) == 0 {
		delete(m.config.ProviderOptions, msg.provider)
	} else {
		m.config.ProviderOptions[msg.provider] = cloneProviderOptions(msg.options)
	}
	m.delegateRuntime = msg.delegateRuntime
	m.persistDraft()
	m.refreshPreflight()
	m.selectActiveModelCatalogEntry()
	m.notice = notice{text: "Cloud model configuration saved. The selected model is ready when its provider reports configured.", kind: noticeSuccess}
	return m, nil
}

func (m Model) updateCredentialStatuses(msg credentialStatusesMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.notice = notice{text: "Read stored credentials: " + msg.err.Error(), kind: noticeError}
		return m, nil
	}
	m.applyCredentialStatuses(msg.statuses)
	return m, nil
}

func (m Model) updateCredentialRemoved(msg credentialRemovedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.notice = notice{text: "Remove Gator credential: " + msg.err.Error(), kind: noticeError}
		return m, nil
	}
	delete(m.localModels.credentials, msg.result.StoreKey)
	m.notice = notice{text: credentialRemovalNotice(msg.result), kind: noticeSuccess}
	if m.config.ModelManagement != nil {
		return m, loadCredentialStatuses(m.config.ModelManagement)
	}
	return m, nil
}

func (m Model) updateCustomProviderSaved(msg customProviderSavedMsg) (tea.Model, tea.Cmd) {
	if m.localModels.customSetup != nil {
		m.localModels.customSetup.saving = false
	}
	if msg.err != nil {
		m.notice = notice{text: "Save custom provider: " + msg.err.Error(), kind: noticeError}
		return m, nil
	}
	m.localModels.customSetup = nil
	m.localModels.pendingCustom = nil
	m.applyCustomProviders(msg.providers, msg.id)
	m.notice = notice{text: "Saved custom provider " + msg.id + ". The API key environment variable was not written to config.json.", kind: noticeSuccess}
	return m, nil
}

func (m Model) updateCustomProviderRemoved(msg customProviderRemovedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.notice = notice{text: "Remove custom provider: " + msg.err.Error(), kind: noticeError}
		return m, nil
	}
	m.applyCustomProviders(msg.providers, msg.id)
	m.notice = notice{text: "Removed custom provider " + msg.id + ". Its API key environment variable was not changed.", kind: noticeSuccess}
	return m, nil
}

func (m Model) updateCustomProviderDiscovered(msg customProviderDiscoverMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.localModels.generation || m.localModels.action != localModelDiscovering {
		return m, nil
	}
	m.localModels.action = localModelIdle
	if msg.err != nil {
		m.notice = notice{text: "Discover custom provider models: " + msg.err.Error(), kind: noticeError}
		return m, nil
	}
	preview := msg.preview
	m.localModels.discovery = &preview
	m.localModels.confirmation = localModelConfirmApplyDiscovery
	m.notice = notice{text: "Discovered model IDs are untrusted server data. Confirm to replace the configured catalog.", kind: noticeInfo}
	return m, nil
}

func (m Model) updateCustomProviderApplied(msg customProviderAppliedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.notice = notice{text: "Apply discovered models: " + msg.err.Error(), kind: noticeError}
		return m, nil
	}
	m.localModels.discovery = nil
	m.applyCustomProviders(msg.providers, msg.id)
	m.notice = notice{text: "Replaced the model catalog for " + msg.id + " with discovered IDs.", kind: noticeSuccess}
	return m, nil
}

func (m Model) updateLocalModelStatus(msg localModelStatusMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.localModels.generation || m.localModels.action != localModelRefreshing {
		return m, nil
	}
	m.localModels.action = localModelIdle
	m.localModels.operation = nil
	m.localModels.err = msg.err
	if msg.err != nil {
		m.notice = notice{text: "Check local models: " + msg.err.Error(), kind: noticeError}
		return m, nil
	}
	m.applyLocalCatalog(msg.catalog)
	section := m.localModels.section
	m.selectActiveModelCatalogEntry()
	m.localModels.section = section
	if msg.catalog.RuntimeError != "" {
		if m.localModels.section == localModelSection && !m.localModels.startDismissed {
			m.requestLocalRuntimeRecovery()
		} else if m.ollamaMissing() {
			m.notice = notice{text: "Ollama is not installed. Open the Local section for installation help.", kind: noticeInfo}
		} else {
			m.notice = notice{text: "Local runtime is unavailable. Open the Local section to choose whether Gator should start Ollama.", kind: noticeInfo}
		}
	} else {
		m.localModels.startDismissed = false
		m.notice = notice{text: "Local model catalog refreshed.", kind: noticeSuccess}
	}
	return m, nil
}

func (m Model) updateLocalProgress(msg localModelProgressMsg) (tea.Model, tea.Cmd) {
	if msg.operation == nil || msg.operation != m.localModels.operation || m.localModels.action == localModelIdle {
		return m, nil
	}
	m.localModels.progress = msg.progress
	return m, waitForLocalModelOperation(m.localModels.operation)
}

func (m Model) updateLocalModelDone(msg localModelDoneMsg) (tea.Model, tea.Cmd) {
	if msg.operation == nil || msg.operation != m.localModels.operation {
		return m, nil
	}
	m.localModels.operation = nil
	action := m.localModels.action
	m.localModels.action = localModelIdle
	m.localModels.err = msg.done.err
	if msg.done.err != nil {
		m.notice = notice{text: "Local model operation stopped: " + msg.done.err.Error(), kind: noticeError}
		return m, nil
	}
	if msg.done.aliases != nil {
		m.applyModelAliases(msg.done.aliases)
		m.notice = notice{text: "Model display name saved. Provider model IDs are unchanged.", kind: noticeSuccess}
		return m, nil
	}
	if msg.done.update != nil {
		m.applyLocalUpdate(*msg.done.update)
		if action == localModelRemoving {
			m.notice = notice{text: "Local model removed and current configuration updated.", kind: noticeSuccess}
		} else {
			m.notice = notice{text: "Local model selected. Return to the composer and send a task to use it.", kind: noticeSuccess}
		}
		return m, nil
	}
	if msg.done.catalog != nil {
		m.applyLocalCatalog(*msg.done.catalog)
	}
	if action == localModelStarting {
		m.notice = notice{text: "Ollama is running under this Gator session. Choose a local model to download or use.", kind: noticeSuccess}
	} else {
		m.notice = notice{text: "Local model download finished. Press u to select it for Gator.", kind: noticeSuccess}
	}
	return m, nil
}
