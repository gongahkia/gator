package tui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type managementSection uint8

const (
	managementSettings managementSection = iota
	managementTrust
	managementMCPAuth
	managementRuns
	managementWorktrees
	managementChildren
	managementExtensions
	managementSectionCount
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
	// mcpLoginServer and mcpLoginURL describe an in-flight MCP browser
	// authorization. The URL is shown before Gator waits for its callback so
	// the developer can complete or cancel it deliberately.
	mcpLoginServer string
	mcpLoginURL    string
	// installSource is the visible source entry field for a new extension.
	// installPreview holds staged bytes awaiting explicit confirmation.
	installSource  *textinput.Model
	installPreview *ExtensionInstallPreview
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

type mcpLoginDoneMsg struct {
	server string
	err    error
}

type extensionPreparedMsg struct {
	preview ExtensionInstallPreview
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
	m.management.selectedRunRecord = m.activeRunRecord()
	m.management.checkedRunRecord = ""
	m.management.actionDetail = ""
	m.management.mcpLoginServer = ""
	m.management.mcpLoginURL = ""
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
		case "install-extension":
			replace := confirmation.value2 == "replace"
			if err = backend.CommitExtensionInstall(confirmation.id, confirmation.value, replace); err == nil {
				detail = "Installed and enabled the reviewed bundle. Its tools stay approval-gated and sandboxed."
			}
		case "remove-mcp-credential":
			if err = backend.RemoveMCPCredential(confirmation.id); err == nil {
				detail = "Removed Gator's stored OAuth credential for MCP server " + confirmation.id + ". Any credential held by another application is unchanged."
			}
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
	if message.String() == "ctrl+c" && m.oauthLogin != nil {
		m.oauthCancel()
		m.oauthLogin.Cancel()
		m.notice = notice{text: "MCP authorization cancellation requested.", kind: noticeInfo}
		return m, nil
	}
	if m.management.loading {
		if message.String() == "esc" {
			m.screen = composeScreen
			return m, m.focusField()
		}
		return m, nil
	}
	if m.management.installSource != nil && m.management.confirm == nil {
		return m.updateExtensionSourceEntry(message)
	}
	if m.management.confirm != nil {
		switch message.String() {
		case "y", "enter":
			confirmation := *m.management.confirm
			m.management.confirm = nil
			m.management.loading = true
			m.management.actionDetail = ""
			if confirmation.action == "install-extension" {
				m.management.installPreview = nil
			}
			return m, runManagementAction(m.management.backend, confirmation)
		case "n", "esc", "ctrl+c":
			m.management.confirm = nil
			m.notice = notice{text: "Management action cancelled.", kind: noticeInfo}
			if m.management.installPreview != nil {
				m.discardStagedExtension()
				m.notice = notice{text: "Discarded the staged extension bundle without installing it.", kind: noticeInfo}
			}
		}
		return m, nil
	}
	switch message.String() {
	case "esc", "q":
		m.discardStagedExtension()
		m.screen = composeScreen
		return m, m.focusField()
	case "left", "h":
		m.management.section = (m.management.section + managementSectionCount - 1) % managementSectionCount
		m.management.index = 0
	case "right", "tab":
		m.management.section = (m.management.section + 1) % managementSectionCount
		m.management.index = 0
	case "l":
		if m.management.section == managementMCPAuth {
			return m.beginMCPLogin()
		}
		m.management.section = (m.management.section + 1) % managementSectionCount
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
		if m.management.section == managementMCPAuth {
			if item, ok := m.selectedMCPAuth(); ok {
				m.management.confirm = &managementConfirmation{
					action: "remove-mcp-credential", id: item.Server,
					label: "Remove Gator's stored OAuth credential for MCP server " + item.Server,
				}
			}
			break
		}
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
	case "i":
		if m.management.section == managementExtensions {
			return m.beginExtensionSourceEntry()
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

func (m Model) beginExtensionSourceEntry() (tea.Model, tea.Cmd) {
	m.discardStagedExtension()
	input := textinput.New()
	input.Prompt = "› "
	input.Placeholder = "/path/to/bundle or https://host/owner/repo.git"
	input.CharLimit = 1024
	input.Width = 60
	input.Focus()
	m.management.installSource = &input
	m.notice = notice{text: "Enter a local bundle directory or an https Git URL. Gator stages and hashes it before anything is installed.", kind: noticeInfo}
	return m, textinput.Blink
}

func (m Model) updateExtensionSourceEntry(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc", "ctrl+c":
		m.management.installSource = nil
		m.notice = notice{text: "Extension installation cancelled. Nothing was fetched or staged.", kind: noticeInfo}
		return m, nil
	case "enter":
		source := strings.TrimSpace(m.management.installSource.Value())
		if err := validateExtensionSource(source); err != nil {
			m.notice = notice{text: err.Error(), kind: noticeError}
			return m, nil
		}
		m.management.installSource = nil
		m.management.loading = true
		m.management.actionDetail = ""
		m.notice = notice{text: "Staging and hashing the extension source. Nothing is installed until you confirm the exact bundle.", kind: noticeInfo}
		return m, prepareExtension(m.management.backend, source)
	}
	input, command := m.management.installSource.Update(message)
	m.management.installSource = &input
	return m, command
}

// validateExtensionSource keeps the TUI narrower than the CLI on purpose. A
// one-key install of executable code accepts only a real local directory or a
// hosted HTTPS Git URL: no shell, package manager, ssh, git@, or plain HTTP.
func validateExtensionSource(source string) error {
	source = strings.TrimSpace(source)
	if source == "" {
		return errors.New("enter a local bundle directory or an https Git repository URL")
	}
	if strings.ContainsAny(source, "\r\n\x00") || strings.HasPrefix(source, "-") {
		return errors.New("extension source contains unsupported characters")
	}
	if strings.HasPrefix(source, "git@") || strings.HasPrefix(source, "ssh://") {
		return errors.New("install ssh and git@ sources with the gator extension command; the TUI accepts only local directories and https URLs")
	}
	if parsed, err := url.Parse(source); err == nil && parsed.Scheme != "" {
		if parsed.Scheme != "https" {
			return errors.New("remote extension sources must use https")
		}
		if parsed.Host == "" || parsed.User != nil {
			return errors.New("https extension sources require a host and must not embed credentials")
		}
		return nil
	}
	info, err := os.Stat(source)
	if err != nil {
		return errors.New("extension source must be an existing local directory or an https Git repository URL")
	}
	if !info.IsDir() {
		return errors.New("local extension sources must be a bundle directory")
	}
	return nil
}

func prepareExtension(backend ManagementBackend, source string) tea.Cmd {
	return func() tea.Msg {
		preview, err := backend.PrepareExtension(source)
		return extensionPreparedMsg{preview: preview, err: err}
	}
}

// discardStagedExtension removes staged executable code whenever the review is
// abandoned. It runs synchronously so leaving the screen cannot leave an
// unreviewed bundle behind in the store.
func (m *Model) discardStagedExtension() {
	m.management.installSource = nil
	preview := m.management.installPreview
	m.management.installPreview = nil
	if preview == nil || m.management.backend == nil {
		return
	}
	_ = m.management.backend.DiscardExtensionPrepare(preview.Token)
}

// beginMCPLogin refuses before any browser URL exists unless this exact
// project manifest hash is already trusted. The backend repeats that check, so
// an out-of-date snapshot cannot authorize an untrusted remote server.
func (m Model) beginMCPLogin() (tea.Model, tea.Cmd) {
	item, ok := m.selectedMCPAuth()
	if !ok {
		return m, nil
	}
	if !item.BundleTrusted {
		m.notice = notice{text: "Trust the current .gator/mcp.json hash in the trust section before authenticating " + item.Server + ".", kind: noticeError}
		return m, nil
	}
	if m.oauthLogin != nil {
		m.notice = notice{text: "An authorization is already waiting for its browser callback. Press Ctrl+C to cancel it.", kind: noticeInfo}
		return m, nil
	}
	login, err := m.management.backend.BeginMCPOAuthLogin(item.Server)
	if err != nil {
		m.notice = notice{text: "Start MCP authorization: " + err.Error(), kind: noticeError}
		return m, nil
	}
	loginContext, cancel := context.WithCancel(context.Background())
	m.oauthLogin = login
	m.oauthCancel = cancel
	m.oauthProvider = mcpLoginProvider(item.Server)
	m.management.mcpLoginServer = item.Server
	m.management.mcpLoginURL = login.URL()
	m.notice = notice{text: "Open the authorization URL below. Gator is waiting for its loopback callback; press Ctrl+C to cancel.", kind: noticeInfo}
	server := item.Server
	return m, func() tea.Msg {
		return mcpLoginDoneMsg{server: server, err: login.Complete(loginContext)}
	}
}

func mcpLoginProvider(server string) string { return "mcp:" + server }

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
	case managementMCPAuth:
		return len(m.management.data.MCPAuth)
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

func (m Model) selectedMCPAuth() (ManagedMCPAuth, bool) {
	if m.management.section != managementMCPAuth || len(m.management.data.MCPAuth) == 0 {
		return ManagedMCPAuth{}, false
	}
	return m.management.data.MCPAuth[min(m.management.index, len(m.management.data.MCPAuth)-1)], true
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
	tabs := []string{"settings", "trust", "mcp auth", "runs", "worktrees", "children", "extensions"}
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
	if m.management.installSource != nil {
		sections = append(sections, m.panel(labelStyle.Render("Install an extension")+"\n"+m.management.installSource.View()+"\n"+dimStyle.Render("Local bundle directory or https Git URL. Gator stages a private copy and shows its hash before anything is installed.")))
	}
	if preview := m.management.installPreview; preview != nil {
		state := "new installation"
		if preview.AlreadyInstalled {
			state = "replaces the installed bundle with this ID"
		}
		sections = append(sections, m.panel(labelStyle.Render("Staged extension")+"\n"+
			preview.ID+"  "+preview.Name+"\n"+
			"source: "+preview.Source+"\n"+
			"sha256: "+preview.Hash+"\n"+
			fmt.Sprintf("tools=%d commands=%d skills=%d prompts=%d ui=%d", preview.Tools, preview.Commands, preview.Skills, preview.Prompts, preview.UI)+"\n"+
			state+"\n"+
			dimStyle.Render("Confirming installs exactly these staged bytes. Extension tools run with your user's authority inside the active sandbox and still request approval.")))
	}
	if confirmation := m.management.confirm; confirmation != nil {
		sections = append(sections, m.panel(errorStyle.Render(confirmation.label+"?\nThis action requires explicit confirmation. Press y to continue or n to cancel.")))
	}
	if m.management.mcpLoginURL != "" {
		sections = append(sections, m.panel(labelStyle.Render("Authorize MCP server "+m.management.mcpLoginServer)+"\n"+m.management.mcpLoginURL+"\n"+dimStyle.Render("Gator is waiting for its own loopback callback. Press Ctrl+C to cancel.")))
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
	case managementMCPAuth:
		footer = append([]string{"l sign in", "d remove credential"}, footer...)
	case managementRuns:
		footer = append([]string{"enter select", "c check", "a apply", "e export patch", "t export HTML"}, footer...)
	case managementWorktrees:
		footer = append([]string{"x remove", "p prune metadata"}, footer...)
	case managementExtensions:
		footer = append([]string{"i install from source", "space enable/disable", "x remove"}, footer...)
	}
	if m.management.installSource != nil {
		footer = []string{"enter stage and review", "esc cancel"}
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
	case managementMCPAuth:
		for _, item := range m.management.data.MCPAuth {
			rows = append(rows, item.Server+"  "+mcpAuthStateLabel(item))
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
			rows = append(rows, fmt.Sprintf("batch %s  %s  comparison=%s  conflicts=%d", item.ID, item.Status, valueOrEmpty(item.ComparisonStatus), len(item.Conflicts)))
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
			detail := labelStyle.Render("Selected bundle") + "\n" + item.Kind + "\nsha256: " + item.Hash + "\n" + dimStyle.Render("Trust activates only this exact content hash. Every executable operation still follows its own approval and sandbox policy.")
			if item.Kind == "lsp" {
				detail += "\n" + m.lspRuntimeDetail()
			}
			return detail
		}
	case managementMCPAuth:
		if item, ok := m.selectedMCPAuth(); ok {
			gate := "bundle hash trusted"
			if !item.BundleTrusted {
				gate = "bundle hash not trusted · sign-in is unavailable"
			}
			return labelStyle.Render("Streamable HTTP MCP server") + "\n" +
				item.Server + "\n" +
				mcpAuthStateLabel(item) + " · " + gate + "\n" +
				dimStyle.Render("Sign-in opens a loopback PKCE flow bound to this exact configured resource. Removing a credential deletes only Gator's stored token; every MCP tool still requests approval before use.")
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
			detail := "worktree: " + valueOrEmpty(item.WorktreePath) +
				"\nbatch: " + valueOrEmpty(item.BatchID) +
				"\nmodel: " + valueOrEmpty(strings.Trim(item.Provider+"/"+item.Model, "/")) +
				"\nprofile: " + valueOrEmpty(item.Profile) +
				"\ndeclared paths: " + valueOrEmpty(strings.Join(item.DeclaredPaths, ", ")) +
				"\nchanged paths: " + valueOrEmpty(strings.Join(item.ChangedPaths, ", ")) +
				"\npolicy: mode=" + valueOrEmpty(item.EffectiveMode) +
				" sandbox=" + valueOrEmpty(item.EffectiveSandbox) +
				" network=" + valueOrEmpty(item.EffectiveNetwork) +
				fmt.Sprintf(" max_steps=%d", item.MaxSteps) +
				" omit=" + valueOrEmpty(strings.Join(item.OmittedCapabilities, ","))
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
				"three-way comparison: " + valueOrEmpty(item.ComparisonStatus),
				fmt.Sprintf("conflicts: %d", len(item.Conflicts)),
			}
			if item.ComparisonTree != "" {
				lines = append(lines, "comparison tree: "+item.ComparisonTree)
			}
			if item.ComparisonDetail != "" {
				lines = append(lines, compact(item.ComparisonDetail, m.panelTextWidth()))
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

func mcpAuthStateLabel(item ManagedMCPAuth) string {
	switch {
	case item.Authenticated:
		return "authenticated"
	case item.Expired:
		return "expired"
	default:
		return "not authenticated"
	}
}

func (m Model) lspRuntimeDetail() string {
	if len(m.management.data.LSPRuntime) == 0 {
		return "session cache: none\n" + dimStyle.Render("Status never starts a language server. The first approved lookup starts one for a trusted hash.")
	}
	lines := []string{fmt.Sprintf("session cache: %d manager(s)", len(m.management.data.LSPRuntime))}
	for _, entry := range m.management.data.LSPRuntime {
		state := "idle"
		if entry.Active > 0 {
			state = fmt.Sprintf("in use (%d)", entry.Active)
		}
		if entry.Retired {
			state += " · retired"
		}
		hash := entry.Hash
		if len(hash) > 12 {
			hash = hash[:12]
		}
		servers := make([]string, 0, len(entry.Servers))
		for _, server := range entry.Servers {
			label := server.Name + "=idle"
			if server.Started {
				label = server.Name + "=running"
			}
			servers = append(servers, label)
		}
		line := "worktree " + valueOrEmpty(entry.Worktree) + " hash " + hash + " " + state
		if len(servers) > 0 {
			line += " · " + strings.Join(servers, ", ")
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n") + "\n" + dimStyle.Render("This view never calls Acquire or starts a server.")
}

func valueOrEmpty(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
