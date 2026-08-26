package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type managementSection uint8

const (
	managementSettings managementSection = iota
	managementTrust
	managementRuns
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
	// selectedRunRecord scopes child browsing and artifact operations without
	// exposing private state paths as editable user input.
	selectedRunRecord string
	checkedRunRecord  string
	actionDetail      string
}

type managementConfirmation struct {
	action string
	id     string
	value  string
	value2 string
	label  string
}

type managementSnapshotMsg struct {
	snapshot ManagementSnapshot
	err      error
}

type managementActionMsg struct {
	message    string
	detail     string
	checkedRun string
	err        error
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
	m.management.selectedRunRecord = m.activeRunRecord()
	m.management.checkedRunRecord = ""
	m.management.actionDetail = ""
	m.screen = managementScreen
	return m, loadManagement(m.management.backend, m.management.selectedRunRecord)
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
		detail := ""
		checkedRun := ""
		switch confirmation.action {
		case "set-policy":
			err = backend.SetExecutionPolicy(confirmation.value, confirmation.value2)
		case "set-defaults":
			err = backend.SetDefaults(confirmation.value, confirmation.value2)
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
		case "export-patch", "export-transcript":
			kind := strings.TrimPrefix(confirmation.action, "export-")
			detail, err = backend.ExportArtifact(kind, confirmation.id)
		case "check-patch":
			var bytes int
			bytes, err = backend.CheckPatch(confirmation.id)
			if err == nil {
				detail = fmt.Sprintf("Patch is compatible with the clean active checkout (%d bytes).", bytes)
				checkedRun = confirmation.id
			}
		case "apply-patch":
			var bytes int
			bytes, err = backend.ApplyPatch(confirmation.id)
			if err == nil {
				detail = fmt.Sprintf("Applied %d bytes to the active checkout. Review and commit the resulting changes.", bytes)
			}
		default:
			err = fmt.Errorf("unknown management action %q", confirmation.action)
		}
		return managementActionMsg{message: confirmation.label, detail: detail, checkedRun: checkedRun, err: err}
	}
}

func runImmediateManagementAction(backend ManagementBackend, action, runRecord, label string) tea.Cmd {
	return runManagementAction(backend, managementConfirmation{action: action, id: runRecord, label: label})
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
			m.management.actionDetail = ""
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
		m.management.section = (m.management.section + 5) % 6
		m.management.index = 0
	case "right", "l", "tab":
		m.management.section = (m.management.section + 1) % 6
		m.management.index = 0
	case "up", "k", "ctrl+p":
		m.moveManagementSelection(-1)
	case "down", "j", "ctrl+n":
		m.moveManagementSelection(1)
	case "r", "ctrl+r":
		m.management.loading = true
		m.management.checkedRunRecord = ""
		m.management.actionDetail = ""
		return m, loadManagement(m.management.backend, m.management.selectedRunRecord)
	case "enter":
		switch m.management.section {
		case managementSettings:
			return m.prepareSettingsChange()
		case managementRuns:
			if item, ok := m.selectedRun(); ok {
				m.management.selectedRunRecord = item.StatePath
				m.management.checkedRunRecord = ""
				m.management.loading = true
				m.notice = notice{text: "Selected retained run " + item.ID + " for artifact and child inspection.", kind: noticeInfo}
				return m, loadManagement(m.management.backend, item.StatePath)
			}
		}
	case "d":
		if m.management.section == managementSettings {
			provider := strings.TrimSpace(m.provider.Value())
			model := strings.TrimSpace(m.model.Value())
			if provider != "" {
				m.management.confirm = &managementConfirmation{
					action: "set-defaults", value: provider, value2: model,
					label: "Save " + provider + "/" + valueOrEmpty(model) + " as the default provider and model",
				}
			}
		}
	case "c":
		if item, ok := m.selectedRun(); ok {
			m.management.loading = true
			m.management.actionDetail = ""
			return m, runImmediateManagementAction(m.management.backend, "check-patch", item.StatePath, "Check retained patch compatibility")
		}
	case "e":
		if item, ok := m.selectedRun(); ok {
			m.management.loading = true
			m.management.actionDetail = ""
			return m, runImmediateManagementAction(m.management.backend, "export-patch", item.StatePath, "Export retained patch privately")
		}
	case "t":
		if item, ok := m.selectedRun(); ok {
			m.management.loading = true
			m.management.actionDetail = ""
			return m, runImmediateManagementAction(m.management.backend, "export-transcript", item.StatePath, "Export retained HTML transcript privately")
		} else if item, ok := m.selectedTrust(); ok && item.Configured {
			action := "trust"
			label := "Trust current " + item.Kind + " bundle"
			if item.Trusted {
				action = "untrust"
				label = "Stop trusting " + item.Kind + " bundle"
			}
			m.management.confirm = &managementConfirmation{action: action, id: item.Kind, label: label}
		}
	case "a":
		if item, ok := m.selectedRun(); ok {
			if m.management.checkedRunRecord != item.StatePath {
				m.notice = notice{text: "Run a successful compatibility check with c before applying this retained patch.", kind: noticeError}
				break
			}
			m.management.confirm = &managementConfirmation{
				action: "apply-patch", id: item.StatePath,
				label: "Apply retained run " + item.ID + " to the clean active checkout",
			}
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

func (m Model) prepareSettingsChange() (tea.Model, tea.Cmd) {
	settings := m.management.data.Settings
	switch m.management.index {
	case 0:
		next := "off"
		if settings.SandboxMode == "off" {
			next = "strict"
		}
		label := "Set default process sandbox to " + next
		if next == "off" {
			label += " (approved commands will have host authority)"
		}
		m.management.confirm = &managementConfirmation{
			action: "set-policy", value: next, value2: settings.Network, label: label,
		}
	case 1:
		next := "allow"
		if settings.Network == "allow" {
			next = "deny"
		}
		label := "Set default sandbox network access to " + next
		if next == "allow" {
			label += " (agent-started processes may make outbound connections)"
		}
		m.management.confirm = &managementConfirmation{
			action: "set-policy", value: settings.SandboxMode, value2: next, label: label,
		}
	case 2:
		provider := strings.TrimSpace(m.provider.Value())
		model := strings.TrimSpace(m.model.Value())
		if provider == "" {
			m.notice = notice{text: "Select a provider in /model before saving TUI defaults.", kind: noticeError}
			return m, nil
		}
		m.management.confirm = &managementConfirmation{
			action: "set-defaults", value: provider, value2: model,
			label: "Save " + provider + "/" + valueOrEmpty(model) + " as the default provider and model",
		}
	}
	return m, nil
}

func (m Model) managementItemCount() int {
	switch m.management.section {
	case managementSettings:
		return 3
	case managementTrust:
		return len(m.management.data.Trusts)
	case managementRuns:
		return len(m.management.data.Runs)
	case managementWorktrees:
		return len(m.management.data.Worktrees)
	case managementChildren:
		return len(m.management.data.Children) + len(m.management.data.Batches)
	case managementExtensions:
		return len(m.management.data.Extensions)
	default:
		return 0
	}
}

func (m Model) selectedRun() (ManagedRun, bool) {
	if m.management.section != managementRuns || len(m.management.data.Runs) == 0 {
		return ManagedRun{}, false
	}
	return m.management.data.Runs[min(m.management.index, len(m.management.data.Runs)-1)], true
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
	tabs := []string{"settings", "trust", "runs", "worktrees", "children", "extensions"}
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
	if m.management.actionDetail != "" {
		sections = append(sections, m.panel(okStyle.Render(m.management.actionDetail)))
	}
	sections = append(sections, m.noticeView())
	footer := []string{"left/right section", "up/down choose", "r refresh", "esc return"}
	switch m.management.section {
	case managementSettings:
		footer = append([]string{"enter change", "d save current model defaults"}, footer...)
	case managementTrust:
		footer = append([]string{"t trust/untrust"}, footer...)
	case managementRuns:
		footer = append([]string{"enter select", "c check", "a apply", "e export patch", "t export HTML"}, footer...)
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
	case managementSettings:
		settings := m.management.data.Settings
		rows = append(rows,
			"sandbox  "+valueOrEmpty(settings.SandboxMode),
			"network  "+valueOrEmpty(settings.Network),
			"defaults  "+valueOrEmpty(settings.DefaultProvider)+"/"+valueOrEmpty(settings.DefaultModel),
		)
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
	case managementRuns:
		for _, item := range m.management.data.Runs {
			state := "worktree missing"
			if item.Available {
				state = "available"
			}
			selected := ""
			if item.StatePath == m.management.selectedRunRecord {
				selected = " · selected"
			}
			rows = append(rows, fmt.Sprintf("%s  %s/%s  %s%s", item.ID, item.Provider, item.Model, state, selected))
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
			rows = append(rows, fmt.Sprintf("child %s  %s  role=%s  patch=%d bytes", item.ID, item.Status, role, item.PatchBytes))
		}
		for _, item := range m.management.data.Batches {
			rows = append(rows, fmt.Sprintf("batch %s  %s  children=%d  conflicts=%d", item.ID, item.Status, len(item.ChildIDs), len(item.Conflicts)))
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
	case managementSettings:
		switch m.management.index {
		case 0:
			return labelStyle.Render("Process sandbox") + "\n" + dimStyle.Render("Strict is the safe default. Turning it off gives every separately approved command the Gator process's host authority.")
		case 1:
			return labelStyle.Render("Sandbox network") + "\n" + dimStyle.Render("Denied by default. Allowing network affects agent-started processes; Gator web tools retain their own URL/query approvals.")
		default:
			return labelStyle.Render("Provider defaults") + "\n" + dimStyle.Render("Enter or d saves the provider/model currently selected in /model. Credentials remain in the private auth store.")
		}
	case managementTrust:
		if item, ok := m.selectedTrust(); ok && item.Hash != "" {
			return labelStyle.Render("Selected bundle") + "\n" + item.Kind + "\nsha256: " + item.Hash + "\n" + dimStyle.Render("Trust activates only this exact content hash. Every executable operation still follows its own approval and sandbox policy.")
		}
	case managementRuns:
		if item, ok := m.selectedRun(); ok {
			availability := "retained worktree missing"
			if item.Available {
				availability = "retained worktree available"
			}
			check := "not checked for apply"
			if m.management.checkedRunRecord == item.StatePath {
				check = "compatibility check passed in this TUI state"
			}
			return labelStyle.Render("Retained run") + "\n" +
				compact(item.Task, m.panelTextWidth()) + "\n" +
				"updated: " + item.UpdatedAt.Local().Format("2006-01-02 15:04") + "\n" +
				availability + " · " + check + "\n" +
				dimStyle.Render("Exports stay private under Gator's state directory. Apply requires a clean checkout, a successful c check, then a separate confirmation.")
		}
	case managementWorktrees:
		if item, ok := m.selectedWorktree(); ok {
			return labelStyle.Render("Selected worktree") + "\n" + item.Path + "\n" + dimStyle.Render("Removal deletes retained files. Private run records remain separate.")
		}
	case managementChildren:
		if m.management.index < len(m.management.data.Children) {
			item := m.management.data.Children[min(m.management.index, len(m.management.data.Children)-1)]
			detail := "worktree: " + valueOrEmpty(item.WorktreePath) + "\nbatch: " + valueOrEmpty(item.BatchID)
			if item.Error != "" {
				detail += "\nerror: " + item.Error
			}
			return labelStyle.Render("Writer child") + "\n" + detail
		}
		batchIndex := m.management.index - len(m.management.data.Children)
		if batchIndex >= 0 && batchIndex < len(m.management.data.Batches) {
			item := m.management.data.Batches[batchIndex]
			lines := []string{
				"status: " + item.Status,
				"children: " + valueOrEmpty(strings.Join(item.ChildIDs, ", ")),
				fmt.Sprintf("conflicts: %d", len(item.Conflicts)),
			}
			for index, conflict := range item.Conflicts {
				lines = append(lines, fmt.Sprintf("%d. %s · children=%s · paths=%s", index+1, conflict.Kind, strings.Join(conflict.ChildIDs, ","), strings.Join(conflict.Paths, ",")))
				if strings.TrimSpace(conflict.Detail) != "" {
					lines = append(lines, "   "+compact(conflict.Detail, max(8, m.panelTextWidth()-3)))
				}
			}
			if item.Error != "" {
				lines = append(lines, "error: "+item.Error)
			}
			return labelStyle.Render("Parallel writer batch") + "\n" + strings.Join(lines, "\n")
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
