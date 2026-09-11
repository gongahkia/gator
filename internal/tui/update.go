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
	case credentialStatusesMsg:
		if msg.err != nil {
			m.notice = notice{text: "Read stored credentials: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		m.applyCredentialStatuses(msg.statuses)
		return m, nil
	case credentialRemovedMsg:
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
	case customProviderSavedMsg:
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
	case customProviderRemovedMsg:
		if msg.err != nil {
			m.notice = notice{text: "Remove custom provider: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		m.applyCustomProviders(msg.providers, msg.id)
		m.notice = notice{text: "Removed custom provider " + msg.id + ". Its API key environment variable was not changed.", kind: noticeSuccess}
		return m, nil
	case customProviderDiscoverMsg:
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
	case customProviderAppliedMsg:
		if msg.err != nil {
			m.notice = notice{text: "Apply discovered models: " + msg.err.Error(), kind: noticeError}
			return m, nil
		}
		m.localModels.discovery = nil
		m.applyCustomProviders(msg.providers, msg.id)
		m.notice = notice{text: "Replaced the model catalog for " + msg.id + " with discovered IDs.", kind: noticeSuccess}
		return m, nil
	case localModelStatusMsg:
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
		m.applyLocalModelCatalog(msg.catalog)
		section := m.localModels.section
		m.selectActiveModelCatalogEntry()
		m.localModels.section = section
		if msg.catalog.RuntimeError != "" {
			if m.localModels.section == localModelSection && !m.localModels.startDismissed {
				m.requestLocalRuntimeRecovery()
			} else {
				if m.ollamaMissing() {
					m.notice = notice{text: "Ollama is not installed. Open the Local section for installation help.", kind: noticeInfo}
				} else {
					m.notice = notice{text: "Local runtime is unavailable. Open the Local section to choose whether Gator should start Ollama.", kind: noticeInfo}
				}
			}
		} else {
			m.localModels.startDismissed = false
			m.notice = notice{text: "Local model catalog refreshed.", kind: noticeSuccess}
		}
		return m, nil
	case localModelProgressMsg:
		if msg.operation == nil || msg.operation != m.localModels.operation || m.localModels.action == localModelIdle {
			return m, nil
		}
		m.localModels.progress = msg.progress
		return m, waitForLocalModelOperation(m.localModels.operation)
	case localModelDoneMsg:
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
			m.applyLocalModelUpdate(*msg.done.update)
			if action == localModelRemoving {
				m.notice = notice{text: "Local model removed and current configuration updated.", kind: noticeSuccess}
			} else {
				m.notice = notice{text: "Local model selected. Return to the composer and send a task to use it.", kind: noticeSuccess}
			}
			return m, nil
		}
		if msg.done.catalog != nil {
			m.applyLocalModelCatalog(*msg.done.catalog)
		}
		if action == localModelStarting {
			m.notice = notice{text: "Ollama is running under this Gator session. Choose a local model to download or use.", kind: noticeSuccess}
		} else {
			m.notice = notice{text: "Local model download finished. Press u to select it for Gator.", kind: noticeSuccess}
		}
		return m, nil
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
	case extensionPreparedMsg:
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
	case mcpLoginDoneMsg:
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
	case managementActionMsg:
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
	case executionDoneMsg:
		m.resolvePendingApproval(tools.CommandDeny)
		wasNewThread := m.resumeStatePath == ""
		wasCancelling := m.cancelling
		m.execution = nil
		if !m.hasBackgroundTerminalTask() {
			m.setTerminalAttachment(nil)
		}
		m.cancelling = false
		m.lastRunCancelled = wasCancelling
		m.outcome = &msg.done.outcome
		m.threadID = msg.done.outcome.ThreadID
		if msg.done.outcome.StatePath != "" {
			m.resumeStatePath = msg.done.outcome.StatePath
			m.forkStatePath = ""
		}
		m.runErr = msg.done.err
		m.finishRunActivity(msg.done.err, wasCancelling)
		m.appendCompletion(msg.done.outcome, msg.done.err)
		m.screen = composeScreen
		m.task.Reset()
		m.task.Placeholder = "Send a follow-up..."
		m.focus = taskField
		_ = m.focusField()
		m.refreshPreflight()
		if msg.done.err != nil {
			m.notice = notice{text: m.runMode.String() + " stopped: " + msg.done.err.Error(), kind: noticeError}
		} else {
			if m.runMode == gatorrun.PlanMode {
				m.notice = notice{text: "Plan ready. Continue this thread in Execute mode when you are ready to make changes.", kind: noticeSuccess}
			} else if m.hasBackgroundTerminalTask() {
				m.notice = notice{text: "Run complete. A detached terminal task is still running in this Gator session; press Ctrl+T to attach.", kind: noticeSuccess}
			} else {
				m.notice = notice{text: "Run complete. Inspect the diff and evidence before applying anything.", kind: noticeSuccess}
			}
		}
		if msg.done.outcome.StatePath != "" && wasNewThread && strings.TrimSpace(m.config.StateDir) != "" {
			if err := journal.DeleteDraft(m.config.StateDir, m.config.RepositoryPath); err != nil {
				m.draftErr = err
			}
		}
		if msg.done.err == nil && !wasCancelling && len(m.queue) > 0 {
			m.notice = notice{text: "Previous run completed. Starting the next queued instruction...", kind: noticeInfo}
			queued, command, dispatched := m.dispatchNextQueued()
			if dispatched {
				return queued, command
			}
			m = queued.(Model)
		} else if len(m.queue) > 0 {
			if wasCancelling {
				m.notice = notice{text: "Run cancelled. " + m.queueSummary() + " retained; use /queue to inspect it.", kind: noticeInfo}
			} else {
				m.notice = notice{text: "Run stopped. " + m.queueSummary() + " retained; use /queue to inspect it.", kind: noticeError}
			}
		}
		if m.quitAfterRun && msg.done.err == nil && !wasCancelling && len(m.queue) == 0 {
			m.quitAfterRun = false
			return m, tea.Quit
		}
		if msg.done.err != nil || wasCancelling {
			m.quitAfterRun = false
		}
		if msg.done.outcome.Worktree.Path == "" {
			return m, nil
		}
		return m, loadReview(msg.done.outcome)
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
