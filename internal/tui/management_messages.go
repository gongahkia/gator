package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/sandbox"
)

func (m Model) updateManagementSnapshot(msg managementSnapshotMsg) (tea.Model, tea.Cmd) {
	m.management.loading = false
	m.management.err = msg.err
	if msg.err != nil {
		m.notice = notice{text: "Refresh management state: " + msg.err.Error(), kind: noticeError}
		return m, nil
	}
	m.management.data = msg.snapshot
	m.config.Execution.Mode = sandbox.Mode(msg.snapshot.Settings.SandboxMode)
	m.config.Execution.Network = sandbox.Network(msg.snapshot.Settings.Network)
	if m.management.section == managementRuns && m.management.selectedRunRecord != "" {
		for index, run := range msg.snapshot.Runs {
			if run.StatePath == m.management.selectedRunRecord {
				m.management.index = index
				break
			}
		}
	}
	if count := m.managementItemCount(); count == 0 {
		m.management.index = 0
	} else if m.management.index >= count {
		m.management.index = count - 1
	}
	return m, nil
}

func (m Model) updateExtensionPrepared(msg extensionPreparedMsg) (tea.Model, tea.Cmd) {
	m.management.loading = false
	if msg.err != nil {
		m.notice = notice{text: "Stage extension source: " + msg.err.Error(), kind: noticeError}
		return m, nil
	}
	preview := msg.preview
	m.management.installPreview = &preview
	label := "Install extension " + preview.ID + " from the staged bundle sha256:" + preview.Hash
	replace := ""
	if preview.AlreadyInstalled {
		replace = "replace"
		label = "Replace installed extension " + preview.ID + " with the staged bundle sha256:" + preview.Hash
	}
	m.management.confirm = &managementConfirmation{
		action: "install-extension", id: preview.Token, value: preview.Hash, value2: replace, label: label,
	}
	m.notice = notice{text: "Review the staged bundle. Only these exact bytes are installed if you confirm.", kind: noticeInfo}
	return m, nil
}

func (m Model) updateMCPLoginDone(msg mcpLoginDoneMsg) (tea.Model, tea.Cmd) {
	if msg.server != m.management.mcpLoginServer {
		return m, nil
	}
	m.oauthLogin = nil
	m.oauthCancel = nil
	m.oauthProvider = ""
	m.management.mcpLoginServer = ""
	m.management.mcpLoginURL = ""
	if msg.err != nil {
		m.notice = notice{text: "MCP authorization stopped: " + msg.err.Error(), kind: noticeError}
		return m, nil
	}
	m.notice = notice{text: "Stored a Gator OAuth credential bound to MCP server " + msg.server + ". Its tools still require approval.", kind: noticeSuccess}
	m.management.loading = true
	return m, loadManagement(m.management.backend, m.management.selectedRunRecord)
}

func (m Model) updateManagementAction(msg managementActionMsg) (tea.Model, tea.Cmd) {
	m.management.loading = false
	if msg.err != nil {
		m.management.err = msg.err
		m.management.actionDetail = ""
		m.notice = notice{text: msg.message + ": " + msg.err.Error(), kind: noticeError}
		return m, nil
	}
	m.management.err = nil
	m.management.actionDetail = msg.detail
	if msg.checkedRun != "" {
		m.management.checkedRunRecord = msg.checkedRun
	} else if strings.HasPrefix(msg.message, "Applied") || strings.Contains(strings.ToLower(msg.message), "apply retained") {
		m.management.checkedRunRecord = ""
	}
	m.notice = notice{text: msg.message + ".", kind: noticeSuccess}
	m.management.loading = true
	return m, loadManagement(m.management.backend, m.management.selectedRunRecord)
}
