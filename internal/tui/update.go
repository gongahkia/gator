package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// Update handles input and asynchronous agent events.
func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeInputs()
		m.syncTranscript(m.followTranscript)
		if m.screen == terminalScreen {
			m.refreshAttachedTerminal()
		}
	case commandApprovalMsg:
		m.pendingApproval = &pendingCommandApproval{argv: append([]string(nil), msg.argv...), reply: msg.reply}
		m.notice = notice{text: "Approve worktree command: " + strings.Join(msg.argv, " "), kind: noticeInfo}
		m.setActivity(activityAwaitingApproval, "awaiting command approval", time.Now())
		return m, waitForExecution(m.execution)
	case terminalManagerMsg:
		m.setTerminalAttachment(msg.attachment)
		return m, waitForExecution(m.execution)
	case agentEventMsg:
		m.appendEvent(msg.event)
		return m, waitForExecution(m.execution)
	case activityTickMsg:
		if (m.screen == runningScreen || m.screen == terminalScreen) && m.execution != nil {
			if m.screen == terminalScreen {
				m.refreshAttachedTerminal()
			}
			return m, waitForExecution(m.execution)
		}
		return m, nil
	case spinner.TickMsg:
		if m.screen == localModelsScreen && m.localModels.action != localModelIdle {
			var command tea.Cmd
			m.localModels.spinner, command = m.localModels.spinner.Update(msg)
			return m, command
		}
	case clipboardWriteMsg:
		return m.applyClipboardWrite(msg), nil
	case cloudModelSetupSavedMsg:
		return m.updateCloudModelSetupSaved(msg)
	case credentialStatusesMsg:
		return m.updateCredentialStatuses(msg)
	case credentialRemovedMsg:
		return m.updateCredentialRemoved(msg)
	case customProviderSavedMsg:
		return m.updateCustomProviderSaved(msg)
	case customProviderRemovedMsg:
		return m.updateCustomProviderRemoved(msg)
	case customProviderDiscoverMsg:
		return m.updateCustomProviderDiscovered(msg)
	case customProviderAppliedMsg:
		return m.updateCustomProviderApplied(msg)
	case localModelStatusMsg:
		return m.updateLocalModelStatus(msg)
	case localModelProgressMsg:
		return m.updateLocalModelProgress(msg)
	case localModelDoneMsg:
		return m.updateLocalModelDone(msg)
	case doctorSnapshotMsg:
		m.doctor.loading = false
		m.doctor.err = msg.err
		if msg.err != nil {
			m.notice = notice{text: "Refresh diagnostics: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		m.doctor.data = msg.snapshot
		return m, nil
	case reviewWebStartedMsg:
		m.applyReviewWebStarted(msg)
		return m, nil
	case managementSnapshotMsg:
		return m.updateManagementSnapshot(msg)
	case extensionPreparedMsg:
		return m.updateExtensionPrepared(msg)
	case mcpLoginDoneMsg:
		return m.updateMCPLoginDone(msg)
	case managementActionMsg:
		return m.updateManagementAction(msg)
	case executionDoneMsg:
		return m.updateExecutionDone(msg)
	case oauthLoginDoneMsg:
		return m.updateOAuthLoginDone(msg)
	case connectDoneMsg:
		return m.updateConnectDone(msg)
	case openCodeCommandDoneMsg:
		return m.updateOpenCodeCommandDone(msg)
	case updateStatusMsg:
		return m.updateStatusDone(msg)
	case delegatedRunDoneMsg:
		return m.updateDelegatedRunDone(msg)
	case diffLoadedMsg:
		return m.updateDiffLoaded(msg)
	case reviewLoadedMsg:
		return m.updateReviewLoaded(msg)
	case reviewMutationDoneMsg:
		return m.updateReviewMutationDone(msg)
	case tea.MouseMsg:
		if m.screen == reviewScreen {
			return m.updateReviewMouse(msg)
		}
		if m.screen == composeScreen || m.screen == runningScreen {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.pageTranscript(false)
				return m, nil
			case tea.MouseButtonWheelDown:
				m.pageTranscript(true)
				return m, nil
			}
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}
