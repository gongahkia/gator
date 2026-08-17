package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/gongahkia/gator/internal/journal"
)

func (m *Model) toggleDrawer() {
	m.drawerOpen = !m.drawerOpen
	if m.drawerOpen {
		if len(m.recentThreads) == 0 {
			m.refreshDrawerThreads()
		}
		m.notice = notice{text: "Control center open. Tab changes section; Ctrl+B or Esc closes it.", kind: noticeInfo}
	} else if m.focus != taskField {
		m.focus = taskField
		_ = m.focusField()
	}
	m.resizeInputs()
	m.syncTranscript(m.followTranscript)
}

func (m *Model) openRuntimeDrawer(active field) tea.Cmd {
	m.drawerOpen = true
	m.drawerSection = drawerRuntime
	m.focus = active
	m.resizeInputs()
	m.syncTranscript(m.followTranscript)
	return m.focusField()
}

func (m *Model) refreshDrawerThreads() {
	var (
		threads []journal.RecentThread
		err     error
	)
	if m.recentAll {
		threads, err = journal.ListAllRecentThreads(m.config.StateDir, 8)
	} else {
		threads, err = journal.ListRecentThreads(m.config.StateDir, m.config.RepositoryPath, 8)
	}
	if err != nil {
		m.notice = notice{text: "Load control-center threads: " + err.Error(), kind: noticeError}
		return
	}
	m.recentThreads = threads
	m.drawerIndex = min(max(0, m.drawerIndex), max(0, len(threads)-1))
}

func (m Model) updateDrawer(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.focus != taskField {
		return m.updateComposer(message)
	}
	switch message.String() {
	case "esc", "ctrl+b":
		m.toggleDrawer()
		return m, nil
	case "tab":
		m.drawerSection = (m.drawerSection + 1) % 4
		if m.drawerSection == drawerThreads && len(m.recentThreads) == 0 {
			m.refreshDrawerThreads()
		}
		return m, nil
	case "shift+tab":
		m.drawerSection = (m.drawerSection + 3) % 4
		if m.drawerSection == drawerThreads && len(m.recentThreads) == 0 {
			m.refreshDrawerThreads()
		}
		return m, nil
	case "p":
		return m, m.openRuntimeDrawer(providerField)
	case "m":
		return m, m.openRuntimeDrawer(modelField)
	case "v":
		return m, m.openRuntimeDrawer(verificationField)
	}
	switch m.drawerSection {
	case drawerThreads:
		switch message.String() {
		case "r":
			m.refreshDrawerThreads()
		case "a":
			m.recentAll = !m.recentAll
			m.refreshDrawerThreads()
		case "up", "k", "ctrl+p":
			if len(m.recentThreads) > 0 {
				m.drawerIndex = (m.drawerIndex - 1 + len(m.recentThreads)) % len(m.recentThreads)
			}
		case "down", "j", "ctrl+n":
			if len(m.recentThreads) > 0 {
				m.drawerIndex = (m.drawerIndex + 1) % len(m.recentThreads)
			}
		case "enter":
			if len(m.recentThreads) == 0 {
				return m, nil
			}
			selected := m.recentThreads[m.drawerIndex]
			if !selected.Available {
				m.notice = notice{text: "The selected retained worktree no longer exists.", kind: noticeError}
				return m, nil
			}
			return m.beginContinuation(selected.HeadStatePath)
		}
	case drawerReview:
		switch message.String() {
		case "r":
			if m.outcome == nil || m.outcome.Worktree.Path == "" {
				m.notice = notice{text: "No retained worktree is available for diff review.", kind: noticeError}
				return m, nil
			}
			m.notice = notice{text: "Refreshing the current worktree diff...", kind: noticeInfo}
			return m, loadDiff(m.outcome.Worktree.Root)
		case "enter":
			if m.outcome == nil {
				m.notice = notice{text: "No completed run is available to review yet.", kind: noticeInfo}
				return m, nil
			}
			m.drawerOpen = false
			m.screen = reviewScreen
			m.resizeInputs()
			return m, nil
		}
	}
	return m, nil
}

func (m Model) drawerView(width, height int) string {
	width = max(1, width)
	contentWidth := max(12, width-2)
	tabs := []string{"threads", "runtime", "activity", "review"}
	for index, tab := range tabs {
		if drawerSection(index) == m.drawerSection {
			tabs[index] = keyStyle.Render("[" + tab + "]")
		} else {
			tabs[index] = dimStyle.Render(tab)
		}
	}
	sections := []string{
		headerStyle.Render("Control center"),
		strings.Join(tabs, " "),
		m.drawerSectionView(contentWidth),
		m.inline(dimStyle.Render("Tab sections · Ctrl+B close")),
	}
	return lipgloss.NewStyle().Width(width).PaddingLeft(1).Render(strings.Join(sections, "\n\n"))
}

func (m Model) drawerSectionView(width int) string {
	switch m.drawerSection {
	case drawerThreads:
		return m.drawerThreadsView(width)
	case drawerRuntime:
		return m.drawerRuntimeView(width)
	case drawerActivity:
		return m.drawerActivityView(width)
	case drawerReview:
		return m.drawerReviewView(width)
	default:
		return ""
	}
}

func (m Model) drawerThreadsView(width int) string {
	scope := "this repository"
	if m.recentAll {
		scope = "all repositories"
	}
	lines := []string{labelStyle.Render("Recent threads · " + scope)}
	if len(m.recentThreads) == 0 {
		lines = append(lines, dimStyle.Render("No retained threads found."))
	} else {
		for index, thread := range m.recentThreads {
			prefix := "  "
			if index == m.drawerIndex {
				prefix = "> "
			}
			state := ""
			if !thread.Available {
				state = " · missing"
			}
			lines = append(lines, prefix+compact(thread.Provider+" · "+thread.Model+state, width-2))
			lines = append(lines, dimStyle.Render("    "+compact(thread.Task, width-4)))
		}
	}
	lines = append(lines, dimStyle.Render("Enter continue · a scope · r refresh"))
	return strings.Join(lines, "\n")
}

func (m Model) drawerRuntimeView(width int) string {
	provider := strings.TrimSpace(m.provider.Value())
	modelName := strings.TrimSpace(m.model.Value())
	if provider == "" {
		provider = "provider not selected"
	}
	if modelName == "" {
		modelName = "provider default"
	}
	lines := []string{
		labelStyle.Render("Runtime"),
		"provider: " + provider,
		"model: " + modelName,
		"mode: " + m.runMode.String(),
	}
	if m.focus == providerField {
		lines = append(lines, "\nprovider", m.provider.View(), m.drawerDropdownView(width))
	} else if m.focus == modelField {
		lines = append(lines, "\nmodel", m.model.View(), m.drawerDropdownView(width))
	} else if m.focus == verificationField {
		lines = append(lines, "\nverification", m.verification.View())
	} else {
		lines = append(lines, "", dimStyle.Render("p provider · m model · v verification"))
	}
	return strings.Join(lines, "\n")
}

func (m Model) drawerDropdownView(width int) string {
	if !m.dropdownVisible() {
		return ""
	}
	options := m.dropdownOptions()
	if len(options) == 0 {
		return ""
	}
	start, end := m.visibleRange(len(options), m.dropdownIndex, min(5, len(options)))
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		prefix := "  "
		if index == m.dropdownIndex {
			prefix = "> "
		}
		lines = append(lines, prefix+compact(options[index].label, width-2))
	}
	return strings.Join(lines, "\n")
}

func (m Model) drawerActivityView(width int) string {
	lines := []string{labelStyle.Render("Activity")}
	if m.execution != nil {
		lines = append(lines, compact(m.activityPhaseLabel()+" · "+m.activity.detail, width))
	} else {
		lines = append(lines, dimStyle.Render("No run is active."))
	}
	if len(m.verificationStatus) > 0 {
		lines = append(lines, "", labelStyle.Render("Verification"))
		for _, status := range m.verificationStatus {
			lines = append(lines, compact(verificationPhaseLabel(status.phase)+" · "+strings.Join(status.argv, " "), width))
		}
	}
	if len(m.queue) > 0 {
		lines = append(lines, "", labelStyle.Render(m.queueSummary()))
		for _, item := range m.queue[:min(3, len(m.queue))] {
			lines = append(lines, dimStyle.Render(compact(item.text, width)))
		}
	}
	if m.commandOutput != "" {
		lines = append(lines, "", labelStyle.Render("Local status"), compact(m.commandOutput, width*3))
	}
	return strings.Join(lines, "\n")
}

func (m Model) drawerReviewView(width int) string {
	lines := []string{labelStyle.Render("Review")}
	if m.outcome == nil {
		lines = append(lines, dimStyle.Render("No completed Gator run is available."))
	} else {
		lines = append(lines, compact(m.verificationSummary(), width))
		if m.diffStats.ready {
			lines = append(lines, fmt.Sprintf("%d files · +%d −%d", m.diffStats.files, m.diffStats.additions, m.diffStats.deletions))
		}
		if m.outcome.Worktree.Path != "" {
			lines = append(lines, dimStyle.Render(compact(m.outcome.Worktree.Path, width)))
		}
	}
	lines = append(lines, "", dimStyle.Render("Enter full review · r refresh diff"))
	return strings.Join(lines, "\n")
}
