package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type diffDisplayMode uint8

const (
	focusedDiffDisplay diffDisplayMode = iota
	fullDiffDisplay
)

const gatorWordmark = "🐊 Gator"

func (m Model) header(mode string) string {
	repository := filepath.Base(filepath.Clean(m.config.RepositoryPath))
	return m.inline(headerStyle.Render(gatorWordmark) + dimStyle.Render("  "+mode+" · "+repository))
}

func (m Model) fieldView(label, hint, value string) string {
	sections := []string{m.inline(labelStyle.Render(label))}
	if hint != "" {
		sections = append(sections, m.inline(dimStyle.Render(compact(hint, m.inlineWidth()))))
	}
	sections = append(sections, m.panel(value))
	return strings.Join(sections, "\n")
}

func (m Model) diffView() string {
	if m.diffErr != nil {
		return errorStyle.Render("Unable to load diff: " + m.diffErr.Error())
	}
	diff := m.reviewDiffText()
	if diff == "" {
		return dimStyle.Render("No changed files are currently visible in the retained worktree.")
	}
	lines := strings.Split(diff, "\n")
	pageSize := m.reviewDiffPageSize()
	start := min(max(0, m.diffOffset), max(0, len(lines)-pageSize))
	end := min(len(lines), start+pageSize)
	content := make([]string, 0, end-start+3)
	if m.diffMode == focusedDiffDisplay && m.focusedDiffInfo.HiddenLines > 0 {
		content = append(content, dimStyle.Render(fmt.Sprintf("Focused review: collapsed %d duplicate lines in %d blocks; press f for the full patch.", m.focusedDiffInfo.HiddenLines, m.focusedDiffInfo.HiddenBlocks)))
	}
	if start > 0 {
		content = append(content, dimStyle.Render(fmt.Sprintf("… %d diff lines above · up/k or PgUp scroll", start)))
	}
	for _, line := range lines[start:end] {
		content = append(content, renderDiffLine(line))
	}
	if end < len(lines) {
		content = append(content, dimStyle.Render(fmt.Sprintf("… %d diff lines below · down/j or PgDn scroll", len(lines)-end)))
	}
	if m.diffTruncated {
		content = append(content, dimStyle.Render("… diff source truncated; inspect the retained worktree for the full patch."))
	}
	return m.panel(strings.Join(content, "\n"))
}

func (m Model) reviewDiffText() string {
	if m.diffMode == fullDiffDisplay {
		return m.diff
	}
	if m.focusedDiff == "" && m.diff != "" {
		return m.diff
	}
	return m.focusedDiff
}

func (m Model) reviewDiffPageSize() int {
	return max(1, m.height-12)
}

func (m *Model) scrollReviewDiff(delta int) {
	lines := strings.Split(m.reviewDiffText(), "\n")
	maxOffset := max(0, len(lines)-m.reviewDiffPageSize())
	m.diffOffset = min(max(0, m.diffOffset+delta), maxOffset)
}

func (m *Model) jumpReviewDiffToEnd() {
	lines := strings.Split(m.reviewDiffText(), "\n")
	m.diffOffset = max(0, len(lines)-m.reviewDiffPageSize())
}

func renderDiffLine(line string) string {
	switch {
	case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
		return okStyle.Render(line)
	case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
		return errorStyle.Render(line)
	case strings.HasPrefix(line, "@@ "):
		return keyStyle.Render(line)
	case strings.HasPrefix(line, "diff --") || strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ "):
		return dimStyle.Render(line)
	case strings.HasPrefix(line, "  … ") && strings.Contains(line, "collapsed in focused review"):
		return dimStyle.Render(line)
	default:
		return line
	}
}

func (m Model) noticeView() string {
	if strings.TrimSpace(m.notice.text) == "" {
		return ""
	}
	switch m.notice.kind {
	case noticeError:
		return m.inline(errorStyle.Render(compact(m.notice.text, m.inlineWidth())))
	case noticeSuccess:
		return m.inline(okStyle.Render(compact(m.notice.text, m.inlineWidth())))
	default:
		return m.inline(dimStyle.Render(compact(m.notice.text, m.inlineWidth())))
	}
}

func (m Model) footer(keys ...string) string {
	var rendered []string
	for _, key := range keys {
		if key == "" {
			continue
		}
		parts := strings.SplitN(key, " ", 2)
		if len(parts) == 1 {
			rendered = append(rendered, keyStyle.Render(parts[0]))
			continue
		}
		rendered = append(rendered, keyStyle.Render(parts[0])+" "+dimStyle.Render(parts[1]))
	}
	return m.inline(strings.Join(rendered, "  "))
}

func (m Model) compactLayout() bool {
	if m.width == 0 || m.height == 0 {
		return false
	}
	return m.width < 72 || m.height < 40
}

func (m Model) constrainedLayout() bool {
	if m.width == 0 || m.height == 0 {
		return false
	}
	return m.width < 48 || m.height < 18
}

func (m Model) panelTextWidth() int {
	return max(1, m.inlineWidth()-2)
}

func (m Model) inlineWidth() int {
	if m.width <= 0 {
		return 0
	}
	return m.conversationWidth()
}

func (m Model) panel(value string) string {
	if m.width < 4 {
		return m.inline(value)
	}
	return panelStyle.Width(m.width).Render(value)
}

func (m Model) inline(value string) string {
	if m.width <= 0 {
		return value
	}
	return lipgloss.NewStyle().MaxWidth(m.inlineWidth()).Render(value)
}

func (m Model) fitToTerminal(view string) string {
	if m.height <= 0 {
		return view
	}
	lines := strings.Split(view, "\n")
	if len(lines) <= m.height {
		return view
	}
	more := m.inline(dimStyle.Render("… resize terminal for more"))
	if m.height == 1 {
		return more
	}
	return strings.Join(append(lines[:m.height-1], more), "\n")
}

func (m Model) popupLimit() int {
	limit := 8
	switch {
	case m.height < 16:
		limit = 1
	case m.height < 22:
		limit = 3
	case m.height < 32:
		limit = 5
	}
	if m.width < 48 {
		limit = min(limit, 3)
	}
	return max(1, limit)
}

func (m Model) recentRunLimit() int {
	return max(1, min(6, max(1, m.height-6)/3))
}

func (m Model) visibleRange(total, selected, limit int) (int, int) {
	if total == 0 || limit <= 0 {
		return 0, 0
	}
	limit = min(limit, total)
	selected = min(max(0, selected), total-1)
	start := selected - limit/2
	start = max(0, min(start, total-limit))
	return start, start + limit
}
