package modelcatalog

import (
	"context"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	modelprovider "github.com/gongahkia/gator/internal/model"
)

type oauthLoginDoneMsg struct {
	provider string
	err      error
}

func (m Model) startOAuthLogin(providerName string) (tea.Model, tea.Cmd) {
	if custom, found := m.customProvider(providerName); found {
		if custom.APIKeyEnv == "" {
			m.notice = notice{text: "Custom provider " + custom.ID + " is configured without an API key.", kind: noticeInfo}
		} else {
			m.notice = notice{text: "Set " + custom.APIKeyEnv + " before running custom provider " + custom.ID + ".", kind: noticeInfo}
		}
		return m, nil
	}
	provider, err := modelprovider.ParseProvider(providerName)
	if err != nil {
		m.notice = notice{text: err.Error(), kind: noticeError}
		return m, nil
	}
	if !modelprovider.SupportsOAuthLogin(provider) {
		m.notice = notice{text: "Provider " + string(provider) + " uses " + modelprovider.CredentialHint(provider) + ".", kind: noticeInfo}
		return m, nil
	}
	if m.oauthLogin != nil {
		m.notice = notice{text: "OAuth login is already waiting for a browser callback. Press Ctrl+C to cancel it.", kind: noticeInfo}
		return m, nil
	}
	if clientIDEnvironment := oauthClientIDEnvironment(string(provider)); clientIDEnvironment != "" && strings.TrimSpace(os.Getenv(clientIDEnvironment)) == "" {
		m.notice = notice{text: "This sign-in requires Gator's OAuth client configuration. Select a provider with an available sign-in, or press c to configure an API credential.", kind: noticeInfo}
		return m, nil
	}
	if provider == modelprovider.Claude {
		m.notice = notice{text: "Claude Code uses an Anthropic API key configured with c. Gator does not import Claude.ai subscription credentials.", kind: noticeInfo}
		return m, nil
	}
	if m.config.BeginOAuthLogin == nil {
		m.notice = notice{text: "OAuth login is not configured for this Gator build.", kind: noticeError}
		return m, nil
	}
	login, err := m.config.BeginOAuthLogin(string(provider))
	if err != nil {
		m.notice = notice{text: err.Error(), kind: noticeError}
		return m, nil
	}
	loginContext, cancel := context.WithCancel(context.Background())
	m.oauthLogin = login
	m.oauthCancel = cancel
	m.oauthProvider = string(provider)
	m.commandOutput = "Open this URL to sign Gator in:\n" + login.URL()
	m.notice = notice{text: "Waiting for the browser callback. Press Ctrl+C to cancel OAuth login.", kind: noticeInfo}
	return m, func() tea.Msg {
		return oauthLoginDoneMsg{provider: string(provider), err: login.Complete(loginContext)}
	}
}

func (m Model) updateOAuthLoginDone(msg oauthLoginDoneMsg) (tea.Model, tea.Cmd) {
	if msg.provider != m.oauthProvider {
		return m, nil
	}
	m.oauthLogin = nil
	m.oauthCancel = nil
	m.oauthProvider = ""
	if msg.err != nil {
		m.notice = notice{text: "OAuth login stopped: " + msg.err.Error(), kind: noticeError}
		return m, nil
	}
	m.commandOutput = "Signed in to " + msg.provider + "."
	m.notice = notice{text: "OAuth credential stored. The provider is ready for a Gator-owned run.", kind: noticeSuccess}
	return m, nil
}

func oauthClientIDEnvironment(provider string) string {
	switch provider {
	case string(modelprovider.Codex):
		return "GATOR_CODEX_OAUTH_CLIENT_ID"
	case string(modelprovider.Copilot):
		return "GATOR_COPILOT_OAUTH_CLIENT_ID"
	case string(modelprovider.KimiCoding):
		return "GATOR_KIMI_CODE_OAUTH_CLIENT_ID"
	case string(modelprovider.XAI):
		return "GATOR_XAI_OAUTH_CLIENT_ID"
	case string(modelprovider.Radius):
		return "GATOR_RADIUS_OAUTH_CLIENT_ID"
	default:
		return ""
	}
}
