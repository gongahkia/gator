package modelcatalog

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/localmodel"
	modelprovider "github.com/gongahkia/gator/internal/model"
)

func gatorCredentialCacheKey(provider string) string {
	if provider == string(modelprovider.Claude) {
		return string(modelprovider.Anthropic)
	}
	return provider
}

func (m Model) storedCredential(provider string) StoredCredentialStatus {
	key := gatorCredentialCacheKey(provider)
	if status, ok := m.localModels.credentials[key]; ok {
		return status
	}
	store, err := auth.New(m.config.StateDir)
	if err != nil {
		return StoredCredentialStatus{Provider: provider, StoreKey: key}
	}
	credential, found, err := store.Read(key)
	if err != nil || !found {
		return StoredCredentialStatus{Provider: provider, StoreKey: key}
	}
	return StoredCredentialStatus{
		Provider: provider,
		StoreKey: key,
		Present:  true,
		Kind:     credentialKindName(credential),
		Expired:  credential.Expired(time.Now()),
	}
}

func credentialKindName(credential auth.Credential) string {
	switch {
	case credential.IsOAuth():
		return "OAuth credential"
	case credential.IsBearerToken():
		return "bearer token"
	default:
		return "API key"
	}
}

func (m *Model) applyCredentialStatuses(statuses []StoredCredentialStatus) {
	m.localModels.credentials = make(map[string]StoredCredentialStatus, len(statuses))
	for _, status := range statuses {
		m.localModels.credentials[status.StoreKey] = status
	}
}

func (m Model) beginRemoveCloudCredential() (tea.Model, tea.Cmd) {
	if m.localModels.section != cloudModelSection {
		m.notice = notice{text: "Stored cloud credentials are removed from the Cloud section.", kind: noticeInfo}
		return m, nil
	}
	cloud, found := m.selectedCloudModel()
	if !found {
		m.notice = notice{text: "Select a cloud provider before removing its Gator credential.", kind: noticeError}
		return m, nil
	}
	if cloud.custom {
		m.notice = notice{text: "Custom providers read an environment variable at run time. Gator does not store their API keys.", kind: noticeInfo}
		return m, nil
	}
	if m.config.ModelManagement == nil {
		m.notice = notice{text: "Credential management is unavailable in this TUI session.", kind: noticeError}
		return m, nil
	}
	status := m.storedCredential(cloud.provider)
	if !status.Present {
		m.notice = notice{text: "No Gator credential is stored for " + cloud.provider + ". Environment variables and vendor CLI logins are unchanged.", kind: noticeInfo}
		return m, nil
	}
	m.localModels.confirmation = localModelConfirmRemoveCredential
	m.notice = notice{text: "Confirm removal of the stored Gator credential. Ambient environment credentials are not unset.", kind: noticeInfo}
	return m, nil
}

func (m Model) removeCloudCredential() (tea.Model, tea.Cmd) {
	cloud, found := m.selectedCloudModel()
	if !found || m.config.ModelManagement == nil {
		m.notice = notice{text: "Credential management is unavailable in this TUI session.", kind: noticeError}
		return m, nil
	}
	provider := cloud.provider
	return m, func() tea.Msg {
		result, err := m.config.ModelManagement.RemoveCredential(provider)
		return credentialRemovedMsg{result: result, err: err}
	}
}

func (m Model) beginRemoveCustomProvider() (tea.Model, tea.Cmd) {
	cloud, found := m.selectedCloudModel()
	if !found || !cloud.custom {
		m.notice = notice{text: "Cloud models are managed by their provider. Gator only removes custom endpoint metadata or local model weights here.", kind: noticeInfo}
		return m, nil
	}
	if cloud.provider == localmodel.ProviderID {
		m.notice = notice{text: "Gator-managed local Ollama is removed from the Local section.", kind: noticeInfo}
		return m, nil
	}
	if m.config.ModelManagement == nil {
		m.notice = notice{text: "Custom provider management is unavailable in this TUI session.", kind: noticeError}
		return m, nil
	}
	m.localModels.confirmation = localModelConfirmRemoveCustom
	m.notice = notice{text: "Confirm removal of custom provider metadata. The API key environment variable is not changed.", kind: noticeInfo}
	return m, nil
}

func (m Model) removeCustomProvider() (tea.Model, tea.Cmd) {
	cloud, found := m.selectedCloudModel()
	if !found || m.config.ModelManagement == nil {
		m.notice = notice{text: "Custom provider management is unavailable in this TUI session.", kind: noticeError}
		return m, nil
	}
	id := cloud.provider
	return m, func() tea.Msg {
		providers, err := m.config.ModelManagement.RemoveCustomProvider(id)
		return customProviderRemovedMsg{providers: providers, id: id, err: err}
	}
}

func (m Model) beginCustomProviderDiscover() (tea.Model, tea.Cmd) {
	if m.localModels.section != cloudModelSection {
		m.notice = notice{text: "Model discovery is available from the Cloud section.", kind: noticeInfo}
		return m, nil
	}
	cloud, found := m.selectedCloudModel()
	if !found || !cloud.custom {
		m.notice = notice{text: "Select a custom provider before requesting its /models catalog.", kind: noticeInfo}
		return m, nil
	}
	if cloud.provider == localmodel.ProviderID {
		m.notice = notice{text: "Gator-managed local Ollama uses the reviewed Local catalog, not /models discovery.", kind: noticeInfo}
		return m, nil
	}
	if m.config.ModelManagement == nil {
		m.notice = notice{text: "Custom provider management is unavailable in this TUI session.", kind: noticeError}
		return m, nil
	}
	m.localModels.generation++
	generation := m.localModels.generation
	m.localModels.action = localModelDiscovering
	id := cloud.provider
	return m, tea.Batch(m.localModels.spinner.Tick, func() tea.Msg {
		preview, err := m.config.ModelManagement.DiscoverCustomProvider(id)
		return customProviderDiscoverMsg{generation: generation, preview: preview, err: err}
	})
}

func (m Model) applyCustomProviderDiscovery() (tea.Model, tea.Cmd) {
	if m.localModels.discovery == nil || m.config.ModelManagement == nil {
		m.notice = notice{text: "No discovered model catalog is waiting to be applied.", kind: noticeError}
		return m, nil
	}
	preview := *m.localModels.discovery
	return m, func() tea.Msg {
		providers, err := m.config.ModelManagement.ApplyCustomProviderDiscovery(preview.ID, preview.Models)
		return customProviderAppliedMsg{providers: providers, id: preview.ID, err: err}
	}
}

func (m *Model) applyCustomProviders(providers []config.CustomProvider, selectID string) {
	m.config.CustomProviders = append([]config.CustomProvider(nil), providers...)
	for index := range m.config.CustomProviders {
		m.config.CustomProviders[index].Models = append([]string(nil), providers[index].Models...)
	}
	if selectID == "" {
		return
	}
	if custom, found := m.customProvider(selectID); found {
		m.provider.SetValue(custom.ID)
		m.model.SetValue(custom.DefaultModel)
		m.config.BaseURL = custom.BaseURL
		m.delegateRuntime = ""
		m.persistDraft()
		m.refreshPreflight()
		m.selectActiveModelCatalogEntry()
		return
	}
	if strings.EqualFold(strings.TrimSpace(m.provider.Value()), selectID) {
		m.provider.SetValue(string(modelprovider.OpenAI))
		m.model.SetValue(modelprovider.DefaultModel(modelprovider.OpenAI))
		m.config.BaseURL = strings.TrimSpace(m.config.ProviderEndpoints[string(modelprovider.OpenAI)])
		m.delegateRuntime = ""
		m.persistDraft()
		m.refreshPreflight()
		m.selectActiveModelCatalogEntry()
	}
}

func credentialRemovalNotice(result CredentialRemovalResult) string {
	if result.Removed {
		text := "Removed the stored Gator " + result.Kind + " for " + result.Provider + ". Environment variables, AWS/ADC, and vendor CLI credentials were not changed."
		if len(result.RemainingSources) > 0 {
			return text + " This process can still authenticate via " + strings.Join(result.RemainingSources, ", ") + "."
		}
		return text
	}
	return "No Gator credential was stored for " + result.Provider + "."
}
