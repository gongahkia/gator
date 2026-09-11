package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/diffview"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/review"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/tools"
)

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
	case connectDoneMsg:
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
	case openCodeCommandDoneMsg:
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
	case updateStatusMsg:
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
	case delegatedRunDoneMsg:
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
	case diffLoadedMsg:
		m.diff, m.diffTruncated, m.diffErr = msg.diff, msg.truncated, msg.err
		m.focusedDiffInfo = diffview.Focus(msg.diff)
		m.focusedDiff = m.focusedDiffInfo.Text
		m.diffOffset = 0
		if msg.err == nil {
			m.diffStats = summarizeDiff(msg.diff)
		}
		return m, nil
	case reviewLoadedMsg:
		m.reviewLoaded = msg.err == nil
		m.diffErr = msg.err
		if msg.err != nil {
			return m, nil
		}
		m.reviewSnapshot = msg.snapshot
		m.reviewScope = review.All
		m.reviewPane = reviewFilesPane
		m.reviewFileIndex = 0
		m.reviewHunkIndex = 0
		m.reviewLineIndex = 0
		m.reviewRangeFrom = -1
		m.reviewMutation = nil
		m.reviewRequestOn = false
		m.normalizeReviewSelection()
		return m, nil
	case reviewMutationDoneMsg:
		m.reviewMutation = nil
		if msg.err != nil {
			m.notice = notice{text: "Update review selection: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		m.notice = notice{text: "Review selection updated in the retained worktree.", kind: noticeSuccess}
		if m.outcome == nil {
			return m, nil
		}
		return m, loadReview(*m.outcome)
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

func delegatedOutputTail(value string) string {
	const limit = 6 * 1024
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return "… earlier terminal output omitted …\n" + value[len(value)-limit:]
}
