package tui

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/instructions"
	"github.com/gongahkia/gator/internal/journal"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/tools"
)

type runOptionField uint8

const (
	runFieldMaxSteps runOptionField = iota
	runFieldBaseURL
	runFieldSandbox
	runFieldNetwork
	runFieldBaseRef
	runFieldSetup
	runFieldCopyIgnored
	runFieldScopes
	runFieldProfile
	runFieldScouts
	runFieldAllowed
	runFieldPrefixes
)

type runOptionsState struct {
	field          runOptionField
	maxSteps       textinput.Model
	baseURL        textinput.Model
	sandboxMode    string
	networkMode    string
	sandboxAck     bool
	networkAck     bool
	baseRef        textinput.Model
	setup          textarea.Model
	copyIgnored    bool
	copyIgnoredAck bool
	setupAck       bool
	scopes         textarea.Model
	profileName    string
	profiles       []instructions.Profile
	profileIndex   int
	scouts         textarea.Model
	allowed        textarea.Model
	prefixes       textarea.Model
	confirm        string
	err            error
}

func newRunOptionsState() runOptionsState {
	maxSteps := textinput.New()
	maxSteps.Prompt = ""
	maxSteps.Placeholder = "leave empty to use /effort"
	maxSteps.CharLimit = 8
	maxSteps.Width = 60
	maxSteps.Blur()

	baseURL := textinput.New()
	baseURL.Prompt = ""
	baseURL.Placeholder = "one-run endpoint override (not saved to config.json)"
	baseURL.CharLimit = 512
	baseURL.Width = 60
	baseURL.Blur()

	baseRef := textinput.New()
	baseRef.Prompt = ""
	baseRef.Placeholder = "Git ref or revision for a new worktree"
	baseRef.CharLimit = 256
	baseRef.Width = 60
	baseRef.Blur()

	return runOptionsState{
		maxSteps: maxSteps,
		baseURL:  baseURL,
		baseRef:  baseRef,
		setup:    newRunOptionTextarea("one setup argv per line (Execute, new runs only)"),
		scopes:   newRunOptionTextarea("one repository-relative path per line"),
		scouts:   newRunOptionTextarea("one read-only scout assignment per line (max 4)"),
		allowed:  newRunOptionTextarea("one exact pre-approved argv per line"),
		prefixes: newRunOptionTextarea("one literal argv prefix per line, e.g. go test"),
	}
}

func newRunOptionTextarea(placeholder string) textarea.Model {
	field := textarea.New()
	field.Prompt = ""
	field.Placeholder = placeholder
	field.ShowLineNumbers = false
	field.CharLimit = 4 * 1024
	field.SetHeight(3)
	field.SetWidth(76)
	field.Blur()
	return field
}

func (m Model) openRunOptions() (tea.Model, tea.Cmd) {
	m.screen = runOptionsScreen
	profiles, err := instructions.ListProfiles(m.config.RepositoryPath)
	m.runOptions.err = err
	m.runOptions.profiles = profiles
	m.runOptions.profileIndex = 0
	for index, profile := range profiles {
		if profile.Name == m.runOptions.profileName {
			m.runOptions.profileIndex = index + 1
			break
		}
	}
	if m.runSafetyAckNeeded() {
		m.runOptions.confirm = m.nextRunSafetyAck()
		m.notice = notice{text: "Confirm the highlighted advanced option before this run can start.", kind: noticeInfo}
	} else {
		m.runOptions.confirm = ""
		m.notice = notice{text: "Advanced run options apply to the next native Gator run. --trust-commands is not a TUI toggle. Sandbox-off and network-allow require confirmation and are not saved as config defaults.", kind: noticeInfo}
	}
	m.focusRunOptionField()
	return m, nil
}

func (m *Model) applyDraftRunOptions(draft journal.Draft) {
	if draft.MaxSteps > 0 {
		m.runOptions.maxSteps.SetValue(strconv.Itoa(draft.MaxSteps))
	}
	m.runOptions.baseRef.SetValue(draft.BaseRef)
	m.runOptions.baseURL.SetValue(draft.BaseURL)
	m.runOptions.sandboxMode = draft.Sandbox
	m.runOptions.networkMode = draft.Network
	m.runOptions.sandboxAck = draft.Sandbox != string(sandbox.Off)
	m.runOptions.networkAck = draft.Network != string(sandbox.AllowNetwork)
	m.runOptions.setup.SetValue(draft.Setup)
	m.runOptions.copyIgnored = draft.CopyIgnored
	m.runOptions.copyIgnoredAck = draft.CopyIgnored
	m.runOptions.setupAck = strings.TrimSpace(draft.Setup) != ""
	m.runOptions.scopes.SetValue(draft.Scopes)
	m.runOptions.profileName = draft.Profile
	m.runOptions.scouts.SetValue(draft.Scouts)
	m.runOptions.allowed.SetValue(draft.AllowedCommands)
	m.runOptions.prefixes.SetValue(draft.CommandPrefixes)
}

func (m Model) draftRunOptions() journal.Draft {
	maxSteps, _ := parseExactTurnCap(m.runOptions.maxSteps.Value())
	return journal.Draft{
		MaxSteps:        maxSteps,
		BaseRef:         strings.TrimSpace(m.runOptions.baseRef.Value()),
		BaseURL:         strings.TrimSpace(m.runOptions.baseURL.Value()),
		Sandbox:         m.runOptions.sandboxMode,
		Network:         m.runOptions.networkMode,
		Setup:           m.runOptions.setup.Value(),
		CopyIgnored:     m.runOptions.copyIgnored,
		Scopes:          m.runOptions.scopes.Value(),
		Profile:         m.runOptions.profileName,
		Scouts:          m.runOptions.scouts.Value(),
		AllowedCommands: m.runOptions.allowed.Value(),
		CommandPrefixes: m.runOptions.prefixes.Value(),
	}
}

func (m Model) continuationLocked() bool {
	return m.resumeStatePath != "" || m.forkStatePath != ""
}

func (m Model) updateRunOptions(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.runOptions.confirm != "" {
		switch message.String() {
		case "enter", "y":
			switch m.runOptions.confirm {
			case "copy-ignored":
				m.runOptions.copyIgnored = true
				m.runOptions.copyIgnoredAck = true
			case "setup":
				m.runOptions.setupAck = true
			case "sandbox-off":
				m.runOptions.sandboxMode = string(sandbox.Off)
				m.runOptions.sandboxAck = true
			case "network-allow":
				m.runOptions.networkMode = string(sandbox.AllowNetwork)
				m.runOptions.networkAck = true
			}
			m.runOptions.confirm = m.nextRunSafetyAck()
			if m.runOptions.confirm == "" {
				m.notice = notice{text: "Advanced option confirmed.", kind: noticeSuccess}
			}
			m.persistDraft()
			return m, nil
		case "esc", "n":
			if m.runOptions.confirm == "copy-ignored" {
				m.runOptions.copyIgnored = false
				m.runOptions.copyIgnoredAck = false
			}
			if m.runOptions.confirm == "sandbox-off" {
				m.runOptions.sandboxMode = ""
				m.runOptions.sandboxAck = false
			}
			if m.runOptions.confirm == "network-allow" {
				m.runOptions.networkMode = ""
				m.runOptions.networkAck = false
			}
			m.runOptions.confirm = ""
			m.notice = notice{text: "Confirmation cancelled. The run will not start until remaining advanced options are confirmed.", kind: noticeInfo}
			return m, nil
		default:
			return m, nil
		}
	}
	switch message.String() {
	case "esc", "q":
		m.blurRunOptionFields()
		m.screen = composeScreen
		m.persistDraft()
		m.refreshPreflight()
		return m, m.focusField()
	case "tab":
		m.advanceRunOptionField(1)
		return m, nil
	case "shift+tab":
		m.advanceRunOptionField(-1)
		return m, nil
	}
	if m.runOptions.field == runFieldSandbox {
		switch message.String() {
		case " ", "enter", "left", "right", "h", "l":
			m.cycleSandboxMode()
			m.persistDraft()
			return m, nil
		}
	}
	if m.runOptions.field == runFieldNetwork {
		switch message.String() {
		case " ", "enter", "left", "right", "h", "l":
			m.cycleNetworkMode()
			m.persistDraft()
			return m, nil
		}
	}
	if m.runOptions.field == runFieldCopyIgnored {
		switch message.String() {
		case " ", "enter", "c":
			if m.continuationLocked() {
				m.notice = notice{text: "Copy-ignored files apply only to a new worktree.", kind: noticeInfo}
				return m, nil
			}
			if m.runOptions.copyIgnored {
				m.runOptions.copyIgnored = false
				m.runOptions.copyIgnoredAck = false
			} else {
				m.runOptions.copyIgnored = true
				m.runOptions.copyIgnoredAck = false
				m.runOptions.confirm = "copy-ignored"
			}
			m.persistDraft()
			return m, nil
		}
	}
	if m.runOptions.field == runFieldProfile {
		switch message.String() {
		case "left", "h":
			m.moveProfileSelection(-1)
			m.persistDraft()
			return m, nil
		case "right", "l", "enter":
			m.moveProfileSelection(1)
			m.persistDraft()
			return m, nil
		}
	}
	var command tea.Cmd
	switch m.runOptions.field {
	case runFieldMaxSteps:
		m.runOptions.maxSteps, command = m.runOptions.maxSteps.Update(message)
	case runFieldBaseURL:
		m.runOptions.baseURL, command = m.runOptions.baseURL.Update(message)
	case runFieldBaseRef:
		if m.continuationLocked() {
			return m, nil
		}
		m.runOptions.baseRef, command = m.runOptions.baseRef.Update(message)
	case runFieldSetup:
		if m.continuationLocked() {
			return m, nil
		}
		m.runOptions.setup, command = m.runOptions.setup.Update(message)
		m.runOptions.setupAck = false
	case runFieldScopes:
		m.runOptions.scopes, command = m.runOptions.scopes.Update(message)
	case runFieldScouts:
		if m.continuationLocked() {
			return m, nil
		}
		m.runOptions.scouts, command = m.runOptions.scouts.Update(message)
	case runFieldAllowed:
		m.runOptions.allowed, command = m.runOptions.allowed.Update(message)
	case runFieldPrefixes:
		m.runOptions.prefixes, command = m.runOptions.prefixes.Update(message)
	}
	m.persistDraft()
	m.refreshPreflight()
	return m, command
}

func (m *Model) moveProfileSelection(delta int) {
	count := len(m.runOptions.profiles) + 1
	m.runOptions.profileIndex = (m.runOptions.profileIndex + delta + count) % count
	if m.runOptions.profileIndex == 0 {
		m.runOptions.profileName = ""
		return
	}
	m.runOptions.profileName = m.runOptions.profiles[m.runOptions.profileIndex-1].Name
}

func (m *Model) advanceRunOptionField(delta int) {
	fields := m.visibleRunOptionFields()
	if len(fields) == 0 {
		return
	}
	index := 0
	for i, field := range fields {
		if field == m.runOptions.field {
			index = i
			break
		}
	}
	m.runOptions.field = fields[(index+delta+len(fields))%len(fields)]
	m.focusRunOptionField()
}

func (m Model) visibleRunOptionFields() []runOptionField {
	fields := []runOptionField{runFieldMaxSteps, runFieldBaseURL, runFieldSandbox, runFieldNetwork}
	if !m.continuationLocked() {
		fields = append(fields, runFieldBaseRef, runFieldSetup, runFieldCopyIgnored)
	}
	fields = append(fields, runFieldScopes, runFieldProfile)
	if !m.continuationLocked() {
		fields = append(fields, runFieldScouts)
	}
	return append(fields, runFieldAllowed, runFieldPrefixes)
}

func (m *Model) focusRunOptionField() {
	m.blurRunOptionFields()
	switch m.runOptions.field {
	case runFieldMaxSteps:
		_ = m.runOptions.maxSteps.Focus()
	case runFieldBaseURL:
		_ = m.runOptions.baseURL.Focus()
	case runFieldBaseRef:
		_ = m.runOptions.baseRef.Focus()
	case runFieldSetup:
		_ = m.runOptions.setup.Focus()
	case runFieldScopes:
		_ = m.runOptions.scopes.Focus()
	case runFieldScouts:
		_ = m.runOptions.scouts.Focus()
	case runFieldAllowed:
		_ = m.runOptions.allowed.Focus()
	case runFieldPrefixes:
		_ = m.runOptions.prefixes.Focus()
	}
}

func (m *Model) blurRunOptionFields() {
	m.runOptions.maxSteps.Blur()
	m.runOptions.baseURL.Blur()
	m.runOptions.baseRef.Blur()
	m.runOptions.setup.Blur()
	m.runOptions.scopes.Blur()
	m.runOptions.scouts.Blur()
	m.runOptions.allowed.Blur()
	m.runOptions.prefixes.Blur()
}

func (m Model) runOptionsView() string {
	sections := []string{m.header("run options")}
	if m.runOptions.confirm != "" {
		sections = append(sections, m.panel(errorStyle.Render(m.runSafetyAckLabel(m.runOptions.confirm))))
		sections = append(sections, m.noticeView(), m.footer("enter/y confirm", "n/esc cancel"))
		return strings.Join(sections, "\n")
	}
	lines := []string{
		m.runOptionLine(runFieldMaxSteps, "Exact turn cap", m.runOptions.maxSteps.View()+"  "+dimStyle.Render("overrides /effort when set")),
		m.runOptionLine(runFieldBaseURL, "One-run base URL", m.runOptions.baseURL.View()),
		m.runOptionLine(runFieldSandbox, "Sandbox override", m.sandboxOverrideLabel()),
		m.runOptionLine(runFieldNetwork, "Network override", m.networkOverrideLabel()),
	}
	if m.continuationLocked() {
		lines = append(lines, dimStyle.Render("Base revision, setup, copy-ignored, and scouts apply only to a new worktree."))
	} else {
		lines = append(lines,
			m.runOptionLine(runFieldBaseRef, "Base revision", m.runOptions.baseRef.View()),
			m.runOptionLine(runFieldSetup, "Worktree setup", m.runOptions.setup.View()),
			m.runOptionLine(runFieldCopyIgnored, "Copy ignored files", m.copyIgnoredLabel()),
		)
	}
	lines = append(lines,
		m.runOptionLine(runFieldScopes, "Additional scopes", m.runOptions.scopes.View()),
		m.runOptionLine(runFieldProfile, "Agent profile", m.profilePickerLabel()),
	)
	if !m.continuationLocked() {
		lines = append(lines, m.runOptionLine(runFieldScouts, "Pre-run scouts", m.runOptions.scouts.View()))
	}
	lines = append(lines,
		m.runOptionLine(runFieldAllowed, "Exact allow-command", m.runOptions.allowed.View()),
		m.runOptionLine(runFieldPrefixes, "Literal command prefixes", m.runOptions.prefixes.View()),
		dimStyle.Render("Prefixes are anchored literal tokens (go test matches go test ./pkg). Verification still uses exact argv. Globs, shells, and launchers are rejected."),
	)
	if m.runOptions.err != nil {
		lines = append(lines, errorStyle.Render("Profiles unavailable: "+m.runOptions.err.Error()))
	}
	sections = append(sections, m.panel(strings.Join(lines, "\n")), m.noticeView(), m.footer("tab next field", "space cycle sandbox/network/copy-ignored", "←/→ profile", "esc composer"))
	return strings.Join(sections, "\n")
}

func (m Model) runOptionLine(field runOptionField, label, value string) string {
	prefix := "  "
	if m.runOptions.field == field {
		prefix = "> "
	}
	return prefix + labelStyle.Render(label) + "\n" + value
}

func (m *Model) cycleSandboxMode() {
	switch m.runOptions.sandboxMode {
	case "":
		m.runOptions.sandboxMode = string(sandbox.Strict)
		m.runOptions.sandboxAck = true
	case string(sandbox.Strict):
		m.runOptions.sandboxMode = string(sandbox.Off)
		m.runOptions.sandboxAck = false
		m.runOptions.confirm = "sandbox-off"
	default:
		m.runOptions.sandboxMode = ""
		m.runOptions.sandboxAck = false
	}
}

func (m *Model) cycleNetworkMode() {
	switch m.runOptions.networkMode {
	case "":
		m.runOptions.networkMode = string(sandbox.DenyNetwork)
		m.runOptions.networkAck = true
	case string(sandbox.DenyNetwork):
		m.runOptions.networkMode = string(sandbox.AllowNetwork)
		m.runOptions.networkAck = false
		m.runOptions.confirm = "network-allow"
	default:
		m.runOptions.networkMode = ""
		m.runOptions.networkAck = false
	}
}

func (m Model) sandboxOverrideLabel() string {
	switch m.runOptions.sandboxMode {
	case string(sandbox.Strict):
		return "strict (this run only)"
	case string(sandbox.Off):
		if m.runOptions.sandboxAck {
			return "off (this run only, confirmed) · approved commands use host authority"
		}
		return "off · confirmation required"
	default:
		return "inherit config (" + string(m.config.Execution.Normalize().Mode) + ")"
	}
}

func (m Model) networkOverrideLabel() string {
	switch m.runOptions.networkMode {
	case string(sandbox.DenyNetwork):
		return "deny (this run only)"
	case string(sandbox.AllowNetwork):
		if m.runOptions.networkAck {
			return "allow (this run only, confirmed) · agent-started processes may use the network"
		}
		return "allow · confirmation required"
	default:
		return "inherit config (" + string(m.config.Execution.Normalize().Network) + ")"
	}
}

func (m Model) copyIgnoredLabel() string {
	if m.runOptions.copyIgnored {
		if m.runOptions.copyIgnoredAck {
			return "on (confirmed) · copies tracked .gator/worktreeinclude entries into the worktree"
		}
		return "on · confirmation required"
	}
	return "off (default)"
}

func (m Model) profilePickerLabel() string {
	if m.runOptions.profileIndex == 0 || m.runOptions.profileName == "" {
		return "(none) · profiles can only narrow policy"
	}
	profile := m.runOptions.profiles[m.runOptions.profileIndex-1]
	summary := profile.Description
	if len(profile.Policy.Omit) > 0 {
		summary += " · omit " + strings.Join(profile.Policy.Omit, ", ")
	}
	return profile.Name + "  " + dimStyle.Render(summary)
}

func (m Model) runSafetyAckNeeded() bool {
	return m.nextRunSafetyAck() != ""
}

func (m Model) nextRunSafetyAck() string {
	if !m.continuationLocked() {
		if m.runOptions.copyIgnored && !m.runOptions.copyIgnoredAck {
			return "copy-ignored"
		}
		setup, err := parseArgvLines(m.runOptions.setup.Value())
		if err == nil && len(setup) > 0 && m.runMode == gatorrun.ExecuteMode && !m.runOptions.setupAck {
			return "setup"
		}
	}
	if m.runOptions.sandboxMode == string(sandbox.Off) && !m.runOptions.sandboxAck {
		return "sandbox-off"
	}
	if m.runOptions.networkMode == string(sandbox.AllowNetwork) && !m.runOptions.networkAck {
		return "network-allow"
	}
	return ""
}

func (m Model) runSafetyAckLabel(kind string) string {
	switch kind {
	case "copy-ignored":
		return "Copy files listed in tracked .gator/worktreeinclude into the isolated worktree? This can expose ignored secrets to the agent."
	case "setup":
		return "Run the listed setup argv once after the worktree is created and before the model starts? Setup is Execute-only, sandbox-bound, and not a verifier."
	case "sandbox-off":
		return "Disable the OS process sandbox for this run only? Approved commands will run with the Gator user's host authority. This does not change config.json."
	case "network-allow":
		return "Allow outbound network for agent-started processes on this run only? Gator web tools still require their own URL approvals. This does not change config.json."
	default:
		return "Confirm this advanced run option."
	}
}

func parseExactTurnCap(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	steps, err := strconv.Atoi(value)
	if err != nil || steps < 1 {
		return 0, fmt.Errorf("exact turn cap must be a positive integer")
	}
	return steps, nil
}

func (m Model) collectRunOptionIssues() []string {
	var issues []string
	if _, err := parseExactTurnCap(m.runOptions.maxSteps.Value()); err != nil {
		issues = append(issues, err.Error())
	}
	if err := validateOneRunBaseURL(m.runOptions.baseURL.Value()); err != nil {
		issues = append(issues, err.Error())
	}
	if mode := strings.TrimSpace(m.runOptions.sandboxMode); mode != "" && mode != string(sandbox.Strict) && mode != string(sandbox.Off) {
		issues = append(issues, "sandbox override must be strict or off")
	}
	if network := strings.TrimSpace(m.runOptions.networkMode); network != "" && network != string(sandbox.DenyNetwork) && network != string(sandbox.AllowNetwork) {
		issues = append(issues, "network override must be deny or allow")
	}
	setup, err := parseArgvLines(m.runOptions.setup.Value())
	if err != nil {
		issues = append(issues, "setup: "+err.Error())
	} else if len(setup) > 8 {
		issues = append(issues, "at most 8 worktree setup commands are allowed")
	} else if len(setup) > 0 && m.runMode != gatorrun.ExecuteMode {
		issues = append(issues, "worktree setup commands are available only in Execute mode")
	}
	if len(parseLineList(m.runOptions.scouts.Value())) > 4 {
		issues = append(issues, "at most 4 read-only scouts may run in parallel")
	}
	if _, err := parseArgvLines(m.runOptions.allowed.Value()); err != nil {
		issues = append(issues, "allow-command: "+err.Error())
	}
	prefixes, err := parseArgvLines(m.runOptions.prefixes.Value())
	if err != nil {
		issues = append(issues, "command prefix: "+err.Error())
	} else {
		for _, pattern := range prefixes {
			if err := tools.ValidateCommandPrefix(pattern); err != nil {
				issues = append(issues, err.Error())
			}
		}
	}
	return issues
}

func mergeUniqueStrings(values ...[]string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, group := range values {
		for _, value := range group {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	return result
}

func (m Model) applyAdvancedRequest(request gatorrun.Request) (gatorrun.Request, error) {
	if issues := m.collectRunOptionIssues(); len(issues) > 0 {
		return request, fmt.Errorf("%s", strings.Join(issues, "\n"))
	}
	if maxSteps, _ := parseExactTurnCap(m.runOptions.maxSteps.Value()); maxSteps > 0 {
		request.MaxSteps = maxSteps
	}
	if override := strings.TrimSpace(m.runOptions.baseURL.Value()); override != "" {
		request.BaseURL = override
	}
	request.Profile = m.runOptions.profileName
	request.Scopes = mergeUniqueStrings(request.Scopes, parseLineList(m.runOptions.scopes.Value()))
	allowed, _ := parseArgvLines(m.runOptions.allowed.Value())
	request.AllowedCommands = allowed
	prefixes, _ := parseArgvLines(m.runOptions.prefixes.Value())
	request.AllowedCommandPrefixes = prefixes
	if !m.continuationLocked() {
		request.BaseRef = strings.TrimSpace(m.runOptions.baseRef.Value())
		request.CopyIgnoredFiles = m.runOptions.copyIgnored && m.runOptions.copyIgnoredAck
		setup, _ := parseArgvLines(m.runOptions.setup.Value())
		if m.runMode == gatorrun.ExecuteMode {
			request.Setup = setup
		}
		request.Scouts = parseLineList(m.runOptions.scouts.Value())
	}
	return request, nil
}

func (m Model) runOptionsStatus() string {
	maxSteps := m.effort.maxSteps(m.config.MaxSteps)
	if exact, err := parseExactTurnCap(m.runOptions.maxSteps.Value()); err == nil && exact > 0 {
		maxSteps = exact
	}
	profile := "(none)"
	if name := strings.TrimSpace(m.runOptions.profileName); name != "" {
		profile = name
	}
	prefixes, _ := parseArgvLines(m.runOptions.prefixes.Value())
	allowed, _ := parseArgvLines(m.runOptions.allowed.Value())
	return fmt.Sprintf("exact max steps: %d\nprofile: %s\nbase url: %s\nsandbox: %s\nnetwork: %s\nbase ref: %s\ncopy ignored: %t\nsetup commands: %d\nextra scopes: %d\nscouts: %d\nallow-command: %d\ncommand prefixes: %d",
		maxSteps, profile, m.runBaseURL(""), string(m.effectiveExecutionPolicy().Mode), string(m.effectiveExecutionPolicy().Network), strings.TrimSpace(m.runOptions.baseRef.Value()), m.runOptions.copyIgnored && m.runOptions.copyIgnoredAck,
		len(mustArgv(m.runOptions.setup.Value())), len(parseLineList(m.runOptions.scopes.Value())),
		len(parseLineList(m.runOptions.scouts.Value())), len(allowed), len(prefixes))
}

func mustArgv(value string) [][]string {
	commands, _ := parseArgvLines(value)
	return commands
}

func validateOneRunBaseURL(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	endpoint, err := url.Parse(value)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil {
		return fmt.Errorf("one-run base URL must be an absolute http(s) URL without credentials")
	}
	return nil
}

func (m Model) runBaseURL(sessionURL string) string {
	if override := strings.TrimSpace(m.runOptions.baseURL.Value()); override != "" {
		return override
	}
	if strings.TrimSpace(sessionURL) != "" {
		return sessionURL
	}
	return m.config.BaseURL
}

func (m Model) effectiveExecutionPolicy() sandbox.Policy {
	policy := m.config.Execution.Normalize()
	if mode := strings.TrimSpace(m.runOptions.sandboxMode); mode != "" {
		policy.Mode = sandbox.Mode(mode)
	}
	if network := strings.TrimSpace(m.runOptions.networkMode); network != "" {
		policy.Network = sandbox.Network(network)
	}
	return policy
}

func (m Model) applyExecutorOverrides(executor gatorrun.Executor) (gatorrun.Executor, error) {
	executor.Sandbox = m.effectiveExecutionPolicy()
	if err := executor.Sandbox.Validate(); err != nil {
		return gatorrun.Executor{}, err
	}
	return executor, nil
}
