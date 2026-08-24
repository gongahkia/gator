package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/review"
)

const maximumReviewRequestBytes = 12 * 1024

type reviewPane uint8

const (
	reviewFilesPane reviewPane = iota
	reviewHunksPane
	reviewLinesPane
)

type reviewMutation struct {
	file  review.File
	hunk  review.Hunk
	scope review.Scope
	whole bool
}

type reviewRange struct {
	side      string
	startLine int
	endLine   int
	before    []string
	after     []string
}

func (m Model) currentReviewSet() review.ChangeSet {
	return m.reviewSnapshot.ChangeSet(m.reviewScope)
}

func (m *Model) normalizeReviewSelection() {
	set := m.currentReviewSet()
	if len(set.Files) == 0 {
		m.reviewFileIndex, m.reviewHunkIndex, m.reviewLineIndex, m.reviewRangeFrom = 0, 0, 0, -1
		return
	}
	m.reviewFileIndex = min(max(0, m.reviewFileIndex), len(set.Files)-1)
	file := set.Files[m.reviewFileIndex]
	if len(file.Hunks) == 0 {
		m.reviewHunkIndex, m.reviewLineIndex, m.reviewRangeFrom = 0, 0, -1
		return
	}
	m.reviewHunkIndex = min(max(0, m.reviewHunkIndex), len(file.Hunks)-1)
	lines := selectableReviewLines(file.Hunks[m.reviewHunkIndex])
	if len(lines) == 0 {
		m.reviewLineIndex, m.reviewRangeFrom = 0, -1
		return
	}
	m.reviewLineIndex = min(max(0, m.reviewLineIndex), len(lines)-1)
	if m.reviewRangeFrom >= len(lines) {
		m.reviewRangeFrom = len(lines) - 1
	}
}

func (m Model) selectedReviewFile() (review.File, bool) {
	set := m.currentReviewSet()
	if m.reviewFileIndex < 0 || m.reviewFileIndex >= len(set.Files) {
		return review.File{}, false
	}
	return set.Files[m.reviewFileIndex], true
}

func (m Model) selectedReviewHunk() (review.File, review.Hunk, bool) {
	file, ok := m.selectedReviewFile()
	if !ok || m.reviewHunkIndex < 0 || m.reviewHunkIndex >= len(file.Hunks) {
		return review.File{}, review.Hunk{}, false
	}
	return file, file.Hunks[m.reviewHunkIndex], true
}

func selectableReviewLines(hunk review.Hunk) []int {
	indices := make([]int, 0, len(hunk.Lines))
	for index, line := range hunk.Lines {
		if line.Kind != "meta" {
			indices = append(indices, index)
		}
	}
	return indices
}

func (m *Model) setReviewScope(scope review.Scope) {
	if m.reviewScope == scope {
		return
	}
	m.reviewScope = scope
	m.reviewFileIndex, m.reviewHunkIndex, m.reviewLineIndex, m.reviewRangeFrom = 0, 0, 0, -1
	m.normalizeReviewSelection()
}

func (m *Model) moveReviewSelection(delta int) {
	switch m.reviewPane {
	case reviewFilesPane:
		m.reviewFileIndex += delta
		m.reviewHunkIndex, m.reviewLineIndex, m.reviewRangeFrom = 0, 0, -1
	case reviewHunksPane:
		m.reviewHunkIndex += delta
		m.reviewLineIndex, m.reviewRangeFrom = 0, -1
	case reviewLinesPane:
		m.reviewLineIndex += delta
	}
	m.normalizeReviewSelection()
}

func (m *Model) cycleReviewPane(delta int) {
	value := int(m.reviewPane) + delta
	if value < int(reviewFilesPane) {
		value = int(reviewLinesPane)
	}
	if value > int(reviewLinesPane) {
		value = int(reviewFilesPane)
	}
	m.reviewPane = reviewPane(value)
}

func (m Model) updateStructuredReview(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.reviewRequestOn {
		return m.updateReviewRequest(message)
	}
	if m.reviewMutation != nil {
		switch message.String() {
		case "y", "enter":
			mutation := *m.reviewMutation
			m.notice = notice{text: "Updating the selected review item…", kind: noticeInfo}
			return m, m.runReviewMutation(mutation)
		case "n", "esc", "ctrl+c":
			m.reviewMutation = nil
			m.notice = notice{text: "Review update cancelled.", kind: noticeInfo}
		}
		return m, nil
	}

	switch message.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		m.cycleReviewPane(1)
		return m, nil
	case "shift+tab":
		m.cycleReviewPane(-1)
		return m, nil
	case "left", "h":
		m.cycleReviewPane(-1)
		return m, nil
	case "right", "l":
		m.cycleReviewPane(1)
		return m, nil
	case "1":
		m.setReviewScope(review.All)
		return m, nil
	case "2":
		m.setReviewScope(review.Unstaged)
		return m, nil
	case "3":
		m.setReviewScope(review.Staged)
		return m, nil
	case "up", "k", "ctrl+p":
		m.moveReviewSelection(-1)
		return m, nil
	case "down", "j", "ctrl+n":
		m.moveReviewSelection(1)
		return m, nil
	case "[":
		m.reviewPane = reviewHunksPane
		m.moveReviewSelection(-1)
		return m, nil
	case "]":
		m.reviewPane = reviewHunksPane
		m.moveReviewSelection(1)
		return m, nil
	case "f":
		file, ok := m.selectedReviewFile()
		if !ok {
			return m, nil
		}
		m.reviewRawFiles[file.ID] = !m.reviewRawFiles[file.ID]
		if m.reviewRawFiles[file.ID] {
			m.notice = notice{text: "Showing the raw patch for this file. Press f for the focused hunk.", kind: noticeInfo}
		} else {
			m.notice = notice{text: "Showing the focused selected hunk. Press f for this file's raw patch.", kind: noticeInfo}
		}
		return m, nil
	case "v":
		if _, _, ok := m.selectedReviewHunk(); !ok {
			m.notice = notice{text: "Select a textual hunk before selecting a range.", kind: noticeInfo}
			return m, nil
		}
		m.reviewPane = reviewLinesPane
		m.reviewRangeFrom = m.reviewLineIndex
		m.notice = notice{text: "Range start set. Move to the last line, then press r to request a change.", kind: noticeInfo}
		return m, nil
	case "r":
		if m.reviewRangeFrom < 0 {
			m.notice = notice{text: "Press v on a focused diff line, select the range, then press r.", kind: noticeInfo}
			return m, nil
		}
		if _, ok := m.reviewSelection(); !ok {
			m.notice = notice{text: "The selected range is no longer available. Refresh and choose it again.", kind: noticeError}
			return m, nil
		}
		m.reviewRequest.Reset()
		_ = m.reviewRequest.Focus()
		m.reviewRequestOn = true
		return m, nil
	case "s":
		return m.confirmReviewMutation(false)
	case "S":
		return m.confirmReviewMutation(true)
	case "c":
		return m.prepareContinuation()
	case "d":
		if m.outcome == nil {
			m.notice = notice{text: "No retained worktree is available for review.", kind: noticeError}
			return m, nil
		}
		m.notice = notice{text: "Refreshing the retained-worktree review…", kind: noticeInfo}
		return m, loadReview(*m.outcome)
	case "e":
		if m.outcome == nil || m.outcome.StatePath == "" {
			m.notice = notice{text: "No retained run record is available for patch handoff.", kind: noticeError}
			return m, nil
		}
		m.commandOutput = "Export:  gator export " + m.outcome.StatePath + " > gator-review.patch\nCheck:   gator apply --check " + m.outcome.StatePath + "\nApply:   gator apply " + m.outcome.StatePath
		m.notice = notice{text: "Patch handoff commands shown. Apply remains explicit and requires a clean compatible checkout.", kind: noticeInfo}
		return m, nil
	case "t":
		m.transcriptReturn = reviewScreen
		m.transcriptIndex = 0
		m.screen = transcriptScreen
		return m, nil
	case "y":
		return m.openThreadTree(reviewScreen)
	case "n":
		m.returnToComposer()
		return m, nil
	case "esc":
		if m.reviewRangeFrom >= 0 {
			m.reviewRangeFrom = -1
			m.notice = notice{text: "Range selection cleared.", kind: noticeInfo}
			return m, nil
		}
		m.screen = composeScreen
		m.focus = taskField
		return m, m.focusField()
	}
	return m, nil
}

func (m Model) confirmReviewMutation(whole bool) (tea.Model, tea.Cmd) {
	if m.reviewScope == review.All {
		m.notice = notice{text: "Choose Unstaged to stage, or Staged to unstage, before changing the index.", kind: noticeInfo}
		return m, nil
	}
	file, ok := m.selectedReviewFile()
	if !ok {
		m.notice = notice{text: "No changed file is selected.", kind: noticeInfo}
		return m, nil
	}
	mutation := reviewMutation{file: file, scope: m.reviewScope, whole: whole}
	if !whole {
		_, hunk, hunkOK := m.selectedReviewHunk()
		if !hunkOK {
			m.notice = notice{text: "This file has no textual hunk. Use S to update the whole file.", kind: noticeInfo}
			return m, nil
		}
		mutation.hunk = hunk
	}
	m.reviewMutation = &mutation
	verb := "stage"
	if mutation.scope == review.Staged {
		verb = "unstage"
	}
	target := "selected hunk"
	if whole {
		target = file.Path
	}
	m.notice = notice{text: fmt.Sprintf("Confirm %s %s in the retained worktree: y/Enter confirms, n/Esc cancels.", verb, target), kind: noticeInfo}
	return m, nil
}

func (m Model) runReviewMutation(mutation reviewMutation) tea.Cmd {
	worktreePath := ""
	if m.outcome != nil {
		worktreePath = m.outcome.Worktree.Path
	}
	return func() tea.Msg {
		if worktreePath == "" {
			return reviewMutationDoneMsg{err: fmt.Errorf("retained worktree is unavailable")}
		}
		var err error
		switch {
		case mutation.whole && mutation.scope == review.Unstaged:
			err = review.StageFile(context.Background(), worktreePath, mutation.file.Path)
		case mutation.whole && mutation.scope == review.Staged:
			err = review.UnstageFile(context.Background(), worktreePath, mutation.file.Path)
		case !mutation.whole && mutation.scope == review.Unstaged:
			err = review.StageHunk(context.Background(), worktreePath, mutation.hunk.ID)
		case !mutation.whole && mutation.scope == review.Staged:
			err = review.UnstageHunk(context.Background(), worktreePath, mutation.hunk.ID)
		default:
			err = fmt.Errorf("review scope is not mutable")
		}
		return reviewMutationDoneMsg{err: err}
	}
}

func (m Model) updateReviewRequest(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc", "ctrl+c":
		m.reviewRequestOn = false
		m.reviewRequest.Blur()
		m.notice = notice{text: "Change request cancelled. The selected range remains local.", kind: noticeInfo}
		return m, nil
	case "ctrl+enter":
		return m.submitReviewRequest()
	}
	var command tea.Cmd
	m.reviewRequest, command = m.reviewRequest.Update(message)
	return m, command
}

func (m Model) submitReviewRequest() (tea.Model, tea.Cmd) {
	if m.outcome == nil || m.outcome.StatePath == "" {
		m.notice = notice{text: "This review has no retained thread to continue.", kind: noticeError}
		return m, nil
	}
	selection, ok := m.reviewSelection()
	if !ok {
		m.notice = notice{text: "The selected range is no longer available. Refresh and choose it again.", kind: noticeError}
		return m, nil
	}
	file, hunk, _ := m.selectedReviewHunk()
	feedback, err := journal.SaveReviewFeedback(m.outcome.StatePath, journal.ReviewFeedback{
		File:        file.Path,
		HunkID:      hunk.ID,
		Side:        selection.side,
		StartLine:   selection.startLine,
		EndLine:     selection.endLine,
		Before:      selection.before,
		After:       selection.after,
		Instruction: m.reviewRequest.Value(),
	})
	if err != nil {
		m.notice = notice{text: "Save review request: " + err.Error(), kind: noticeError}
		return m, nil
	}
	m.reviewRequestOn = false
	m.reviewRequest.Blur()
	next, _ := m.prepareContinuation()
	updated := next.(Model)
	updated.task.SetValue(journal.ReviewFollowUp(feedback))
	updated.notice = notice{text: "Sending a constrained follow-up with the selected review context…", kind: noticeInfo}
	return updated.startRun()
}

func (m Model) reviewSelection() (reviewRange, bool) {
	_, hunk, ok := m.selectedReviewHunk()
	if !ok {
		return reviewRange{}, false
	}
	selectable := selectableReviewLines(hunk)
	if m.reviewRangeFrom < 0 || m.reviewRangeFrom >= len(selectable) || m.reviewLineIndex < 0 || m.reviewLineIndex >= len(selectable) {
		return reviewRange{}, false
	}
	first, last := m.reviewRangeFrom, m.reviewLineIndex
	if first > last {
		first, last = last, first
	}
	value := reviewRange{}
	oldStart, oldEnd, newStart, newEnd := 0, 0, 0, 0
	for _, index := range selectable[first : last+1] {
		line := hunk.Lines[index]
		text := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(line.Text, "+"), "-"), " ")
		switch line.Kind {
		case "deletion":
			value.before = append(value.before, text)
			oldStart, oldEnd = extendReviewRange(oldStart, oldEnd, line.OldLine)
		case "addition":
			value.after = append(value.after, text)
			newStart, newEnd = extendReviewRange(newStart, newEnd, line.NewLine)
		case "context":
			value.before = append(value.before, text)
			value.after = append(value.after, text)
			oldStart, oldEnd = extendReviewRange(oldStart, oldEnd, line.OldLine)
			newStart, newEnd = extendReviewRange(newStart, newEnd, line.NewLine)
		}
	}
	switch {
	case oldStart > 0 && newStart > 0:
		value.side = "both"
		value.startLine = min(oldStart, newStart)
		value.endLine = max(oldEnd, newEnd)
	case oldStart > 0:
		value.side, value.startLine, value.endLine = "old", oldStart, oldEnd
	case newStart > 0:
		value.side, value.startLine, value.endLine = "new", newStart, newEnd
	default:
		return reviewRange{}, false
	}
	return value, true
}

func extendReviewRange(start, end, line int) (int, int) {
	if line <= 0 {
		return start, end
	}
	if start == 0 || line < start {
		start = line
	}
	if line > end {
		end = line
	}
	return start, end
}

func (m Model) structuredReviewView() string {
	set := m.currentReviewSet()
	scope := map[review.Scope]string{review.All: "all changes", review.Unstaged: "unstaged", review.Staged: "staged"}[m.reviewScope]
	summary := fmt.Sprintf("%s · %d files · %d additions · %d deletions", scope, set.Stats.Files, set.Stats.Additions, set.Stats.Deletions)
	sections := []string{m.header("review"), m.inline(keyStyle.Render(summary)), m.inline(dimStyle.Render("1 all · 2 unstaged · 3 staged · Tab changes pane · mouse clicks select"))}
	if m.reviewSnapshot.Truncated {
		sections = append(sections, m.inline(errorStyle.Render("Review source is truncated; inspect the retained worktree before staging or requesting changes.")))
	}
	if len(set.Files) == 0 {
		sections = append(sections, m.panel(dimStyle.Render("No changed files are currently visible in the retained worktree.")))
		return m.reviewFooter(sections)
	}
	if m.compactLayout() || m.width < 92 {
		sections = append(sections, m.reviewFilesView(), m.reviewHunksAndPatchView())
	} else {
		leftWidth := max(28, m.inlineWidth()/3)
		rightWidth := max(36, m.inlineWidth()-leftWidth)
		left := lipgloss.NewStyle().Width(leftWidth).Render(m.reviewFilesView())
		right := lipgloss.NewStyle().Width(rightWidth).Render(m.reviewHunksAndPatchView())
		sections = append(sections, lipgloss.JoinHorizontal(lipgloss.Top, left, right))
	}
	return m.reviewFooter(sections)
}

func (m Model) reviewFooter(sections []string) string {
	if m.reviewMutation != nil {
		sections = append(sections, m.panel(keyStyle.Render("Confirm review update: y/Enter confirm · n/Esc cancel")))
	}
	if m.reviewRequestOn {
		sections = append(sections, labelStyle.Render("Request a change"), m.panel(m.reviewRequest.View()), m.footer("Ctrl+Enter send follow-up", "Esc cancel"))
		return strings.Join(sections, "\n")
	}
	sections = append(sections, m.noticeView(), m.footer("up/down choose", "f focused/raw", "v range", "r request change", "s hunk", "S file", "d refresh", "e handoff", "c continue", "q quit"))
	return strings.Join(sections, "\n")
}

func (m Model) reviewFilesView() string {
	set := m.currentReviewSet()
	lines := []string{reviewPaneLabel("files", m.reviewPane == reviewFilesPane)}
	for index, file := range set.Files {
		prefix := "  "
		if index == m.reviewFileIndex {
			prefix = "> "
		}
		label := fmt.Sprintf("%s%s  +%d −%d", prefix, file.Path, file.Stats.Additions, file.Stats.Deletions)
		if file.Binary {
			label += "  binary"
		}
		if index == m.reviewFileIndex {
			label = keyStyle.Render(compact(label, max(16, m.inlineWidth()/3-2)))
		} else {
			label = compact(label, max(16, m.inlineWidth()/3-2))
		}
		lines = append(lines, label)
	}
	return m.panel(strings.Join(lines, "\n"))
}

func (m Model) reviewHunksAndPatchView() string {
	file, ok := m.selectedReviewFile()
	if !ok {
		return m.panel(dimStyle.Render("No file selected."))
	}
	lines := []string{reviewPaneLabel("hunks · "+file.Path, m.reviewPane == reviewHunksPane)}
	if len(file.Hunks) == 0 {
		lines = append(lines, dimStyle.Render("Binary or metadata-only change; use S for the whole file."))
	} else {
		for index, hunk := range file.Hunks {
			prefix := "  "
			if index == m.reviewHunkIndex {
				prefix = "> "
			}
			label := fmt.Sprintf("%s%d  %s  +%d −%d", prefix, index+1, hunk.Header, hunk.Stats.Additions, hunk.Stats.Deletions)
			if index == m.reviewHunkIndex {
				label = keyStyle.Render(compact(label, max(20, m.panelTextWidth()-2)))
			} else {
				label = compact(label, max(20, m.panelTextWidth()-2))
			}
			lines = append(lines, label)
		}
	}
	lines = append(lines, "", reviewPaneLabel("change", m.reviewPane == reviewLinesPane))
	if m.reviewRawFiles[file.ID] {
		lines = append(lines, dimStyle.Render("Raw patch for this file"))
		lines = append(lines, renderReviewPatch(file.Patch)...)
	} else if _, hunk, found := m.selectedReviewHunk(); found {
		lines = append(lines, dimStyle.Render("Focused selected hunk"))
		lines = append(lines, renderFocusedReviewHunk(hunk, m.reviewLineIndex, m.reviewRangeFrom)...)
	} else {
		lines = append(lines, dimStyle.Render("No textual hunk is available."))
	}
	return m.panel(strings.Join(lines, "\n"))
}

func reviewPaneLabel(value string, selected bool) string {
	if selected {
		return keyStyle.Render(value)
	}
	return labelStyle.Render(value)
}

func renderReviewPatch(patch string) []string {
	rows := strings.Split(strings.TrimSuffix(patch, "\n"), "\n")
	result := make([]string, 0, len(rows))
	for _, row := range rows {
		result = append(result, renderDiffLine(row))
	}
	return result
}

func renderFocusedReviewHunk(hunk review.Hunk, selected, from int) []string {
	lines := []string{renderDiffLine(hunk.Header)}
	selectable := selectableReviewLines(hunk)
	for index, line := range hunk.Lines {
		prefix := "   "
		for cursor, lineIndex := range selectable {
			if lineIndex != index {
				continue
			}
			if cursor == selected {
				prefix = ">> "
			} else if from >= 0 && ((from <= cursor && cursor <= selected) || (selected <= cursor && cursor <= from)) {
				prefix = "•  "
			}
			break
		}
		lineNumber := ""
		if line.OldLine > 0 && line.NewLine > 0 {
			lineNumber = fmt.Sprintf("%d/%d ", line.OldLine, line.NewLine)
		} else if line.OldLine > 0 {
			lineNumber = fmt.Sprintf("%d/- ", line.OldLine)
		} else if line.NewLine > 0 {
			lineNumber = fmt.Sprintf("-/%d ", line.NewLine)
		}
		lines = append(lines, prefix+dimStyle.Render(lineNumber)+renderDiffLine(line.Text))
	}
	return lines
}

func (m Model) updateReviewMouse(message tea.MouseMsg) (tea.Model, tea.Cmd) {
	if !m.reviewLoaded || m.reviewRequestOn || m.reviewMutation != nil {
		return m, nil
	}
	if message.Button == tea.MouseButtonWheelUp {
		m.moveReviewSelection(-1)
		return m, nil
	}
	if message.Button == tea.MouseButtonWheelDown {
		m.moveReviewSelection(1)
		return m, nil
	}
	if message.Action != tea.MouseActionPress || message.Button != tea.MouseButtonLeft {
		return m, nil
	}
	set := m.currentReviewSet()
	if len(set.Files) == 0 {
		return m, nil
	}
	if !m.compactLayout() && m.width >= 92 && message.X < max(28, m.inlineWidth()/3) && message.Y >= 4 {
		index := message.Y - 4
		if index >= 0 && index < len(set.Files) {
			m.reviewPane, m.reviewFileIndex = reviewFilesPane, index
			m.reviewHunkIndex, m.reviewLineIndex, m.reviewRangeFrom = 0, 0, -1
			m.normalizeReviewSelection()
		}
		return m, nil
	}
	file, ok := m.selectedReviewFile()
	if !ok {
		return m, nil
	}
	start := 4
	if m.compactLayout() || m.width < 92 {
		start += len(set.Files) + 1
	}
	if message.Y >= start && message.Y < start+len(file.Hunks) {
		m.reviewPane, m.reviewHunkIndex = reviewHunksPane, message.Y-start
		m.reviewLineIndex, m.reviewRangeFrom = 0, -1
		m.normalizeReviewSelection()
		return m, nil
	}
	if _, hunk, ok := m.selectedReviewHunk(); ok && !m.reviewRawFiles[file.ID] {
		lineStart := start + len(file.Hunks) + 3
		lineIndex := message.Y - lineStart
		if lineIndex >= 0 && lineIndex < len(hunk.Lines) {
			for cursor, rawIndex := range selectableReviewLines(hunk) {
				if rawIndex == lineIndex {
					m.reviewPane, m.reviewLineIndex = reviewLinesPane, cursor
					return m, nil
				}
			}
		}
	}
	return m, nil
}
