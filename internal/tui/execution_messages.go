package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/journal"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/tools"
)

func (m Model) updateExecutionDone(msg executionDoneMsg) (tea.Model, tea.Cmd) {
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
	} else if m.runMode == gatorrun.PlanMode {
		m.notice = notice{text: "Plan ready. Continue this thread in Execute mode when you are ready to make changes.", kind: noticeSuccess}
	} else if m.hasBackgroundTerminalTask() {
		m.notice = notice{text: "Run complete. A detached terminal task is still running in this Gator session; press Ctrl+T to attach.", kind: noticeSuccess}
	} else {
		m.notice = notice{text: "Run complete. Inspect the diff and evidence before applying anything.", kind: noticeSuccess}
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
}
