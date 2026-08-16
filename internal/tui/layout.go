package tui

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) header(mode string) string {
	repository := filepath.Base(filepath.Clean(m.config.RepositoryPath))
	return m.inline(headerStyle.Render("Gator") + "  " + dimStyle.Render(mode+" · "+repository))
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
	if m.diff == "" {
		return dimStyle.Render("No changed files are currently visible in the retained worktree.")
	}
	lines := strings.Split(m.diff, "\n")
	maxLines := max(1, m.height-10)
	truncatedByView := len(lines) > maxLines
	if truncatedByView {
		lines = lines[:maxLines]
	}
	content := strings.Join(lines, "\n")
	if m.diffTruncated || truncatedByView {
		content += "\n" + dimStyle.Render("… diff preview truncated; inspect the retained worktree for the full patch.")
	}
	return m.panel(content)
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

func (m Model) compactComposer() bool {
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
	return max(1, m.width-8)
}

func (m Model) inlineWidth() int {
	return max(1, m.width)
}

func (m Model) panel(value string) string {
	if m.width < 8 {
		return m.inline(value)
	}
	return panelStyle.Width(m.width - 4).Render(value)
}

func (m Model) inline(value string) string {
	if m.width <= 0 {
		return value
	}
	return lipgloss.NewStyle().MaxWidth(m.width).Render(value)
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
