package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/journal"
)

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
	m.refreshPreflight()
	return m, nil
}

func (m Model) updateConnectDone(msg connectDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.notice = notice{text: "Provider-owned login stopped: " + msg.err.Error(), kind: noticeError}
		return m, nil
	}
	runtime := delegatedRuntimeForProvider(msg.provider)
	if runtime == "" || m.config.NewDelegateCommand == nil {
		m.commandOutput = "Provider-owned login completed for " + msg.provider + ". Its credential remains in that vendor CLI's store. Use the matching gator delegate runtime for harness-owned runs."
		m.notice = notice{text: "Provider-owned login completed. See the next action below.", kind: noticeSuccess}
		return m, nil
	}
	m.returnToComposer()
	m.delegateRuntime = runtime
	m.provider.SetValue(msg.provider)
	if _, modelName, _, err := m.resolveProviderAndModel(msg.provider, ""); err == nil {
		m.model.SetValue(modelName)
	}
	m.commandOutput = delegatedRuntimeLabel(runtime) + " is ready. Send a task to run it in a fresh isolated worktree. The vendor CLI keeps its own login, tools, approvals, and session state."
	m.notice = notice{text: "Provider-owned login completed. Harness mode is ready for the next task.", kind: noticeSuccess}
	m.persistDraft()
	m.refreshPreflight()
	return m, nil
}

func (m Model) updateOpenCodeCommandDone(msg openCodeCommandDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.notice = notice{text: "OpenCode " + msg.action + " stopped: " + msg.err.Error(), kind: noticeError}
		return m, nil
	}
	if msg.action == "login" {
		m.returnToComposer()
		m.delegateRuntime = "opencode"
		m.provider.SetValue("opencode")
		m.commandOutput = "OpenCode login completed for " + msg.provider + ". Select a harness model with /opencode use PROVIDER/MODEL, then send an Execute-mode task."
		m.notice = notice{text: "OpenCode provider login completed. The installed CLI retains its credential.", kind: noticeSuccess}
		m.persistDraft()
		m.refreshPreflight()
		return m, m.focusField()
	}
	m.commandOutput = "OpenCode authentication status completed in the terminal. Use /opencode login PROVIDER [METHOD] to change a provider connection."
	m.notice = notice{text: "OpenCode status completed.", kind: noticeSuccess}
	return m, nil
}

func (m Model) updateStatusDone(msg updateStatusMsg) (tea.Model, tea.Cmd) {
	m.updateChecking = false
	if msg.err != nil {
		m.commandOutput = m.versionStatus() + "\n\nUpdate check failed: " + msg.err.Error()
		m.notice = notice{text: "Update check failed: " + msg.err.Error(), kind: noticeError}
		return m, nil
	}
	current := strings.TrimSpace(msg.status.Current)
	if current == "" {
		current = strings.TrimSpace(m.config.Build.Version)
	}
	if current == "" {
		current = "dev"
	}
	latest := strings.TrimSpace(msg.status.Latest)
	if msg.status.Available {
		m.commandOutput = "Gator " + latest + " is available; this TUI is running " + current + ".\n\nExit the TUI and run 'gator update' to download, verify, and replace the executable."
		m.notice = notice{text: "A newer Gator release is available. The running TUI was not changed.", kind: noticeInfo}
		return m, nil
	}
	m.commandOutput = "Gator " + current + " is up to date."
	if latest != "" && latest != current {
		m.commandOutput += " Latest published release: " + latest + "."
	}
	m.notice = notice{text: "No update was installed; the running build is current.", kind: noticeSuccess}
	return m, nil
}

func (m Model) updateDelegatedRunDone(msg delegatedRunDoneMsg) (tea.Model, tea.Cmd) {
	m.task.Reset()
	m.task.Placeholder = "Send another delegated task..."
	m.focus = taskField
	_ = m.focusField()
	if msg.err != nil {
		m.commandOutput = delegatedRuntimeLabel(msg.runtime) + " stopped."
		if output := delegatedOutputTail(msg.output); output != "" {
			m.commandOutput += "\n\nTerminal output:\n" + output
		}
		m.notice = notice{text: "Delegated run stopped: " + msg.err.Error(), kind: noticeError}
	} else {
		m.commandOutput = delegatedRuntimeLabel(msg.runtime) + " completed. Review its retained worktree before applying changes."
		if output := delegatedOutputTail(msg.output); output != "" {
			m.commandOutput += "\n\nTerminal output:\n" + output
		}
		m.notice = notice{text: "Delegated run complete. Review the retained worktree before applying changes.", kind: noticeSuccess}
		if strings.TrimSpace(m.config.StateDir) != "" {
			if err := journal.DeleteDraft(m.config.StateDir, m.config.RepositoryPath); err != nil {
				m.draftErr = err
			}
		}
	}
	m.refreshPreflight()
	return m, nil
}

func delegatedOutputTail(value string) string {
	const limit = 6 * 1024
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return "… earlier terminal output omitted …\n" + value[len(value)-limit:]
}
