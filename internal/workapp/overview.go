package workapp

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/workhome"
)

type overviewRefreshed struct {
	Generation uint64
	View       workhome.Overview
	Err        error
}
type controlExternalDone struct{ Err error }

func (m Model) readOverviewCommand() tea.Cmd {
	if m.config.ReadOverview == nil {
		return nil
	}
	read, generation := m.config.ReadOverview, m.overviewGeneration
	return func() tea.Msg {
		view, err := read(context.Background())
		return overviewRefreshed{generation, view, err}
	}
}
func (m *Model) refreshOverviewCommand() tea.Cmd {
	if m.config.ReadOverview == nil || m.overviewBusy {
		return nil
	}
	m.overviewGeneration++
	m.overviewBusy = true
	return m.readOverviewCommand()
}
func (m *Model) startControlExternal(args []string) tea.Cmd {
	if m.config.ControlCommand == nil {
		return func() tea.Msg { return controlExternalDone{errors.New("terminal review is unavailable")} }
	}
	command := m.config.ControlCommand(args)
	if command == nil {
		return func() tea.Msg { return controlExternalDone{errors.New("this terminal review is unavailable")} }
	}
	if m.config.BridgeMode {
		m.bridgeExternal = bridgeExternal("control", "", "", command)
		return nil
	}
	return tea.ExecProcess(command, func(err error) tea.Msg { return controlExternalDone{err} })
}

func (m Model) openOverviewGoal(id, command string) (tea.Model, tea.Cmd) {
	if m.running {
		m.sectionNotice = "Finish or stop current Work before switching goals."
		return m, nil
	}
	m.entries = []entry{{kind: "goal", id: id}}
	m.launcherMode = "goals"
	m.selected = 0
	m.paletteQuery = ""
	next, cmd := m.updateLauncher(tea.KeyMsg{Type: tea.KeyEnter})
	updated := next.(Model)
	if command != "" && updated.goalID == id {
		return updated.runLocalCommand(command)
	}
	return updated, cmd
}
func (m Model) openOverviewWork(id string) (tea.Model, tea.Cmd) {
	if m.config.HistoryStore == nil {
		m.sectionNotice = "Work history is unavailable."
		return m, nil
	}
	record, err := m.config.HistoryStore.Load(id)
	if err != nil {
		m.sectionNotice = err.Error()
		return m, nil
	}
	m.section, m.home, m.launcher = "history", false, false
	m.historyItems = []historyItem{{Record: record, Project: m.historyProject(record.ConversationID)}}
	m.selected = 0
	m.selectHistoryItem()
	return m, nil
}

func moveSourcePath(value, from, to string) string {
	original := value
	canonical := func(value string) string {
		probe := value
		for {
			if resolved, err := filepath.EvalSymlinks(probe); err == nil {
				suffix, _ := filepath.Rel(probe, value)
				return filepath.Join(resolved, suffix)
			}
			parent := filepath.Dir(probe)
			if parent == probe {
				return value
			}
			probe = parent
		}
	}
	value, from = canonical(value), canonical(from)
	relative, err := filepath.Rel(from, value)
	if err == nil && (relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return filepath.Join(to, relative)
	}
	return original
}
