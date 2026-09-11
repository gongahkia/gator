package modelcatalog

import (
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/auth"
	modelprovider "github.com/gongahkia/gator/internal/model"
)

func (m Model) cloudModels() []cloudModelEntry {
	entries := make([]cloudModelEntry, 0, len(modelprovider.Names())+len(m.config.CustomProviders))
	credentialStore, credentialStoreErr := auth.New(m.config.StateDir)
	for _, providerName := range modelprovider.Names() {
		provider, err := modelprovider.ParseProvider(providerName)
		if err != nil {
			continue
		}
		if provider == modelprovider.Claude {
			if m.catalogOnly {
				continue
			}
			entries = append(entries, claudeCodeHarnessEntry(credentialStore, credentialStoreErr, m.localModels.credentials))
			continue
		}

		models := modelprovider.CuratedModels(provider)
		if providerName == strings.TrimSpace(m.provider.Value()) {
			models = prependModelIfMissing(models, strings.TrimSpace(m.model.Value()))
		}
		status := cloudProviderStatus(provider, credentialStore, credentialStoreErr, m.localModels.credentials)
		if len(models) == 0 {
			entries = append(entries, cloudModelEntry{
				provider: providerName,
				name:     providerName + " · account model required",
				status:   status,
				canLogin: modelprovider.SupportsOAuthLogin(provider),
			})
			continue
		}
		for _, modelName := range models {
			entries = append(entries, cloudModelEntry{
				provider:   providerName,
				model:      modelName,
				name:       providerName + " · " + m.modelDisplayName(providerName, modelName, modelName),
				status:     status,
				selectable: true,
				canLogin:   modelprovider.SupportsOAuthLogin(provider),
			})
		}
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
			entries = append(entries, cloudModelEntry{provider: provider.ID, model: modelName, name: name, status: status, selectable: true, custom: true})
		}
	}
	return entries
}

func prependModelIfMissing(models []string, model string) []string {
	if model == "" {
		return models
	}
	for _, existing := range models {
		if existing == model {
			return models
		}
	}
	return append([]string{model}, models...)
}

func claudeCodeHarnessEntry(store auth.Store, storeErr error, cached map[string]StoredCredentialStatus) cloudModelEntry {
	status := "requires ANTHROPIC_API_KEY"
	if cachedStatus, ok := cached[string(modelprovider.Anthropic)]; ok && cachedStatus.Present {
		status = "Anthropic API key stored by Gator"
	} else if storeErr == nil {
		credential, found, err := store.Read(string(modelprovider.Anthropic))
		if err == nil && found && credential.IsAPIKey() {
			status = "Anthropic API key stored by Gator"
		}
	}
	if modelprovider.AmbientCredentialAvailable(modelprovider.Anthropic) {
		status = "API key set: ANTHROPIC_API_KEY"
	}
	return cloudModelEntry{
		provider: string(modelprovider.Claude),
		name:     "Claude Code · API-key harness",
		status:   status,
		canLogin: true,
	}
}

func cloudProviderStatus(provider modelprovider.Provider, store auth.Store, storeErr error, cached map[string]StoredCredentialStatus) string {
	if status, ok := cached[gatorCredentialCacheKey(string(provider))]; ok && status.Present {
		if status.Expired {
			return "stored credential expired"
		}
		if status.Kind == "OAuth credential" {
			return "signed in"
		}
		return "Gator credential stored"
	}
	if storeErr == nil {
		credential, found, err := store.Read(gatorCredentialCacheKey(string(provider)))
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
		if cloud.provider == string(modelprovider.Claude) {
			m.notice = notice{text: "Claude Code is a harness. Press l to provide an Anthropic API key, then send a task through the Claude Code harness.", kind: noticeInfo}
			return m, nil
		}
		m.notice = notice{text: "This provider requires an account-specific deployment or model ID. Press c to configure it.", kind: noticeInfo}
		return m, nil
	}
	if m.config.SaveModelSelection != nil {
		if err := m.config.SaveModelSelection(cloud.provider, cloud.model); err != nil {
			m.notice = notice{text: "Select model: " + err.Error(), kind: noticeError}
			return m, nil
		}
	}
	m.provider.SetValue(cloud.provider)
	m.model.SetValue(cloud.model)
	if custom, found := m.customProvider(cloud.provider); found {
		m.config.BaseURL = custom.BaseURL
	} else {
		m.config.BaseURL = strings.TrimSpace(m.config.ProviderEndpoints[strings.ToLower(strings.TrimSpace(cloud.provider))])
	}
	m.delegateRuntime = ""
	m.persistDraft()
	m.refreshPreflight()
	m.notice = notice{text: "Selected " + cloud.name + ". Return to the composer and send a task when its readiness is configured.", kind: noticeSuccess}
	return m, nil
}
