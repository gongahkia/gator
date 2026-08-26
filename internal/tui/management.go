package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type managementSection uint8

const (
	managementTrust managementSection = iota
	managementWorktrees
	managementChildren
	managementExtensions
)

type managementState struct {
	backend ManagementBackend
	data    ManagementSnapshot
	section managementSection
	index   int
	loading bool
	err     error
	confirm *managementConfirmation
}

type managementConfirmation struct {
	action string
	id     string
	label  string
}

type managementSnapshotMsg struct {
	snapshot ManagementSnapshot
	err      error
}

type managementActionMsg struct {
	message string
	err     error
}

func newManagementState(backend ManagementBackend) managementState {
	return managementState{backend: backend}
}

func (m Model) openManagement() (tea.Model, tea.Cmd) {
	if m.management.backend == nil {
		m.notice = notice{text: "Management services are unavailable in this TUI session.", kind: noticeError}
		return m, nil
	}
	m.management.loading = true
	m.management.err = nil
	m.management.confirm = nil
	m.management.index = 0
	m.screen = managementScreen
	return m, loadManagement(m.management.backend, m.activeRunRecord())
}

func (m Model) activeRunRecord() string {
	if m.outcome != nil && strings.TrimSpace(m.outcome.StatePath) != "" {
		return m.outcome.StatePath
	}
	return m.resumeStatePath
}

func loadManagement(backend ManagementBackend, runRecord string) tea.Cmd {
	return func() tea.Msg {
		snapshot, err := backend.Snapshot(runRecord)
		return managementSnapshotMsg{snapshot: snapshot, err: err}
	}
}

func runManagementAction(backend ManagementBackend, confirmation managementConfirmation) tea.Cmd {
	return func() tea.Msg {
		var err error
		switch confirmation.action {
		case "trust":
			err = backend.SetTrust(confirmation.id, true)
		case "untrust":
			err = backend.SetTrust(confirmation.id, false)
		case "enable-extension":
			err = backend.SetExtensionEnabled(confirmation.id, true)
		case "disable-extension":
			err = backend.SetExtensionEnabled(confirmation.id, false)
		case "remove-extension":
			err = backend.RemoveExtension(confirmation.id)
		case "remove-worktree":
			err = backend.RemoveWorktree(confirmation.id)
		case "prune-worktrees":
			err = backend.PruneWorktrees()
		default:
			err = fmt.Errorf("unknown management action %q", confirmation.action)
		}
		return managementActionMsg{message: confirmation.label, err: err}
	}
}

func (m Model) updateManagement(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.management.loading {
		if message.String() == "esc" {
			m.screen = composeScreen
			return m, m.focusField()
		}
		return m, nil
	}
	if m.management.confirm != nil {
		switch message.String() {
		case "y", "enter":
			confirmation := *m.management.confirm
			m.management.confirm = nil
			m.management.loading = true
			return m, runManagementAction(m.management.backend, confirmation)
		case "n", "esc", "ctrl+c":
			m.management.confirm = nil
			m.notice = notice{text: "Management action cancelled.", kind: noticeInfo}
		}
		return m, nil
	}
	switch message.String() {
	case "esc", "q":
		m.screen = composeScreen
		return m, m.focusField()
	case "left", "h":
		m.management.section = (m.management.section + 3) % 4
		m.management.index = 0
	case "right", "l", "tab":
		m.management.section = (m.management.section + 1) % 4
		m.management.index = 0
	case "up", "k", "ctrl+p":
		m.moveManagementSelection(-1)
	case "down", "j", "ctrl+n":
		m.moveManagementSelection(1)
	case "r", "ctrl+r":
		m.management.loading = true
		return m, loadManagement(m.management.backend, m.activeRunRecord())
	case "t":
		if item, ok := m.selectedTrust(); ok && item.Configured {
			action := "trust"
			label := "Trust current " + item.Kind + " bundle"
			if item.Trusted {
				action = "untrust"
				label = "Stop trusting " + item.Kind + " bundle"
			}
			m.management.confirm = &managementConfirmation{action: action, id: item.Kind, label: label}
		}
	case " ":
		if item, ok := m.selectedExtension(); ok {
			action := "enable-extension"
			label := "Enable extension " + item.ID
			if item.Enabled {
				action = "disable-extension"
				label = "Disable extension " + item.ID
			}
			m.management.confirm = &managementConfirmation{action: action, id: item.ID, label: label}
		}
	case "x":
		switch m.management.section {
		case managementWorktrees:
			if item, ok := m.selectedWorktree(); ok {
				m.management.confirm = &managementConfirmation{action: "remove-worktree", id: item.ID, label: "Permanently remove retained worktree " + item.ID}
			}
		case managementExtensions:
			if item, ok := m.selectedExtension(); ok {
				m.management.confirm = &managementConfirmation{action: "remove-extension", id: item.ID, label: "Permanently remove extension " + item.ID}
			}
		}
	case "p":
		if m.management.section == managementWorktrees {
			m.management.confirm = &managementConfirmation{action: "prune-worktrees", label: "Prune stale Git worktree metadata"}
		}
	}
	return m, nil
}

func (m *Model) moveManagementSelection(delta int) {
	count := m.managementItemCount()
	if count == 0 {
		m.management.index = 0
		return
	}
	m.management.index = (m.management.index + delta + count) % count
}

func (m Model) managementItemCount() int {
	switch m.management.section {
	case managementTrust:
		return len(m.management.data.Trusts)
	case managementWorktrees:
		return len(m.management.data.Worktrees)
	case managementChildren:
		return len(m.management.data.Children)
	case managementExtensions:
		return len(m.management.data.Extensions)
	default:
		return 0
	}
}

func (m Model) selectedTrust() (ManagedTrust, bool) {
	if m.management.section != managementTrust || len(m.management.data.Trusts) == 0 {
		return ManagedTrust{}, false
	}
	return m.management.data.Trusts[min(m.management.index, len(m.management.data.Trusts)-1)], true
}

func (m Model) selectedWorktree() (ManagedWorktree, bool) {
	if m.management.section != managementWorktrees || len(m.management.data.Worktrees) == 0 {
		return ManagedWorktree{}, false
	}
	return m.management.data.Worktrees[min(m.management.index, len(m.management.data.Worktrees)-1)], true
}

func (m Model) selectedExtension() (ManagedExtension, bool) {
	if m.management.section != managementExtensions || len(m.management.data.Extensions) == 0 {
		return ManagedExtension{}, false
	}
	return m.management.data.Extensions[min(m.management.index, len(m.management.data.Extensions)-1)], true
}

func (m Model) managementView() string {
	sections := []string{m.header("manage")}
	tabs := []string{"trust", "worktrees", "children", "extensions"}
	for index := range tabs {
		if managementSection(index) == m.management.section {
			tabs[index] = keyStyle.Render("[" + tabs[index] + "]")
		}
	}
	sections = append(sections, m.inline(strings.Join(tabs, "  ")))
	if m.management.loading {
		sections = append(sections, m.panel(dimStyle.Render("Refreshing local management state…")))
	} else if m.management.err != nil {
		sections = append(sections, m.panel(errorStyle.Render(m.management.err.Error())))
	} else {
		sections = append(sections, m.panel(m.managementRows()))
		if detail := m.managementDetail(); detail != "" {
			sections = append(sections, m.panel(detail))
		}
	}
	if confirmation := m.management.confirm; confirmation != nil {
		sections = append(sections, m.panel(errorStyle.Render(confirmation.label+"?\nThis action requires explicit confirmation. Press y to continue or n to cancel.")))
	}
	sections = append(sections, m.noticeView())
	footer := []string{"left/right section", "up/down choose", "r refresh", "esc return"}
	switch m.management.section {
	case managementTrust:
		footer = append([]string{"t trust/untrust"}, footer...)
	case managementWorktrees:
		footer = append([]string{"x remove", "p prune metadata"}, footer...)
	case managementExtensions:
		footer = append([]string{"space enable/disable", "x remove"}, footer...)
	}
	sections = append(sections, m.footer(footer...))
	return strings.Join(sections, "\n")
}

func (m Model) managementRows() string {
	var rows []string
	switch m.management.section {
	case managementTrust:
		for _, item := range m.management.data.Trusts {
			state := "not configured"
			if item.Configured {
				state = "disabled · hash not trusted"
			}
			if item.Trusted {
				state = "active"
			}
			rows = append(rows, item.Kind+"  "+state)
		}
	case managementWorktrees:
		for _, item := range m.management.data.Worktrees {
			rows = append(rows, item.ID+"  "+item.Path)
		}
	case managementChildren:
		for _, item := range m.management.data.Children {
			role := item.Role
			if role == "" {
				role = "-"
			}
			rows = append(rows, fmt.Sprintf("%s  %s  role=%s  patch=%d bytes", item.ID, item.Status, role, item.PatchBytes))
		}
	case managementExtensions:
		for _, item := range m.management.data.Extensions {
			state := "disabled"
			if item.Enabled {
				state = "enabled"
			}
			rows = append(rows, fmt.Sprintf("%s  %s  %s  tools=%d", item.ID, state, item.Name, item.Tools))
		}
	}
	if len(rows) == 0 {
		return dimStyle.Render("No items in this section.")
	}
	for index := range rows {
		prefix := "  "
		if index == m.management.index {
			prefix = "> "
			rows[index] = keyStyle.Render(compact(prefix+rows[index], m.panelTextWidth()))
		} else {
			rows[index] = compact(prefix+rows[index], m.panelTextWidth())
		}
	}
	return strings.Join(rows, "\n")
}

func (m Model) managementDetail() string {
	switch m.management.section {
	case managementTrust:
		if item, ok := m.selectedTrust(); ok && item.Hash != "" {
			return labelStyle.Render("Selected bundle") + "\n" + item.Kind + "\nsha256: " + item.Hash + "\n" + dimStyle.Render("Trust activates only this exact content hash. Every executable operation still follows its own approval and sandbox policy.")
		}
	case managementWorktrees:
		if item, ok := m.selectedWorktree(); ok {
			return labelStyle.Render("Selected worktree") + "\n" + item.Path + "\n" + dimStyle.Render("Removal deletes retained files. Private run records remain separate.")
		}
	case managementChildren:
		if len(m.management.data.Children) > 0 {
			item := m.management.data.Children[min(m.management.index, len(m.management.data.Children)-1)]
			detail := "worktree: " + valueOrEmpty(item.WorktreePath) + "\nbatch: " + valueOrEmpty(item.BatchID)
			if item.Error != "" {
				detail += "\nerror: " + item.Error
			}
			return labelStyle.Render("Writer child") + "\n" + detail
		}
	case managementExtensions:
		if item, ok := m.selectedExtension(); ok {
			return labelStyle.Render("Installed extension") + "\n" + valueOrEmpty(item.Description) + "\n" + dimStyle.Render("Sidecar tools remain approval-gated and run inside the active sandbox.")
		}
	}
	return ""
}

func valueOrEmpty(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
