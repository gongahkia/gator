package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/diffview"
	"github.com/gongahkia/gator/internal/review"
)

func (m Model) updateDiffLoaded(msg diffLoadedMsg) (tea.Model, tea.Cmd) {
	m.diff, m.diffTruncated, m.diffErr = msg.diff, msg.truncated, msg.err
	m.focusedDiffInfo = diffview.Focus(msg.diff)
	m.focusedDiff = m.focusedDiffInfo.Text
	m.diffOffset = 0
	if msg.err == nil {
		m.diffStats = summarizeDiff(msg.diff)
	}
	return m, nil
}

func (m Model) updateReviewLoaded(msg reviewLoadedMsg) (tea.Model, tea.Cmd) {
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
}

func (m Model) updateReviewMutationDone(msg reviewMutationDoneMsg) (tea.Model, tea.Cmd) {
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
}
