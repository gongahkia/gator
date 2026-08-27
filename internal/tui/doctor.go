package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type doctorState struct {
	backend DoctorBackend
	data    DoctorSnapshot
	loading bool
	err     error
}

type doctorSnapshotMsg struct {
	snapshot DoctorSnapshot
	err      error
}

func (m Model) openDoctor() (tea.Model, tea.Cmd) {
	if m.config.Doctor == nil {
		m.notice = notice{text: "Diagnostics are unavailable in this TUI session.", kind: noticeError}
		return m, nil
	}
	m.screen = doctorScreen
	m.doctor.backend = m.config.Doctor
	m.doctor.loading = true
	m.doctor.err = nil
	m.notice = notice{text: "Inspect-only diagnostics. This view does not start services or change configuration.", kind: noticeInfo}
	return m, loadDoctor(m.config.Doctor, strings.TrimSpace(m.provider.Value()))
}

func loadDoctor(backend DoctorBackend, provider string) tea.Cmd {
	return func() tea.Msg {
		snapshot, err := backend.Snapshot(provider)
		return doctorSnapshotMsg{snapshot: snapshot, err: err}
	}
}

func (m Model) updateDoctor(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc", "q":
		m.screen = composeScreen
		return m, m.focusField()
	case "r", "ctrl+r":
		m.doctor.loading = true
		return m, loadDoctor(m.doctor.backend, strings.TrimSpace(m.provider.Value()))
	case "enter":
		if len(m.doctor.data.SuggestedVerify) > 0 {
			m.verification.SetValue(strings.Join(m.doctor.data.SuggestedVerify, "\n"))
			m.persistDraft()
			m.refreshPreflight()
			m.notice = notice{text: "Copied suggested verification into /verify. Review it before starting a run.", kind: noticeSuccess}
		}
	}
	return m, nil
}

func (m Model) doctorView() string {
	sections := []string{m.header("doctor")}
	if m.doctor.loading {
		sections = append(sections, m.panel(dimStyle.Render("Collecting local diagnostics…")))
	} else if m.doctor.err != nil {
		sections = append(sections, m.panel(errorStyle.Render(m.doctor.err.Error())))
	} else {
		sections = append(sections, m.panel(m.doctorReport()))
	}
	sections = append(sections, m.noticeView(), m.footer("enter apply suggested verify", "r refresh", "esc return"))
	return strings.Join(sections, "\n")
}

func (m Model) doctorReport() string {
	data := m.doctor.data
	git := "not detected"
	if data.RepositoryDetected {
		git = "detected · " + data.RepositoryPath
	}
	lines := []string{
		"Repository: " + git,
		"Provider: " + valueOrEmpty(data.Provider),
		"Authentication (" + valueOrEmpty(data.AuthKind) + "): " + valueOrEmpty(data.AuthStatus),
		"Strict sandbox: " + valueOrEmpty(data.Sandbox),
		"Web search: " + valueOrEmpty(data.WebSearchStatus),
		"Effective TUI policy: sandbox=" + valueOrEmpty(data.EffectiveSandbox) + " network=" + valueOrEmpty(data.EffectiveNetwork) + " (not mutated here)",
		"Dependencies:",
	}
	for _, item := range data.Dependencies {
		state := "missing"
		if item.Installed {
			state = "installed"
		}
		role := "optional"
		if item.Required {
			role = "required"
		}
		lines = append(lines, fmt.Sprintf("  %s: %s (%s for %s)", item.Name, state, role, item.Purpose))
		if !item.Installed && item.HelpURL != "" {
			lines = append(lines, "    Official source: "+item.HelpURL)
		}
		for _, advice := range item.Advice {
			lines = append(lines, "    "+advice)
		}
	}
	lines = append(lines, "Local models: "+valueOrEmpty(data.LocalHost))
	for _, item := range data.LocalModels {
		state := "enabled"
		if !item.Allowed {
			state = "disabled: " + item.Reason
		}
		lines = append(lines, "  "+item.ID+": "+state+" · "+item.Needs)
	}
	if len(data.SuggestedVerify) == 0 {
		lines = append(lines, "Suggested verification: none")
	} else {
		for _, suggestion := range data.SuggestedVerify {
			lines = append(lines, "Suggested verification: "+suggestion)
		}
	}
	if !data.RepositoryDetected {
		lines = append(lines, "Run Gator from a Git checkout.")
	}
	return strings.Join(lines, "\n")
}
