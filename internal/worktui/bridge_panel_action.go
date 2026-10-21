package worktui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/uisnapshot"
)

// BridgePanelAction is a mouse or native form action against a reviewed view.
// The view identity is checked before any field or selection is changed.
type BridgePanelAction struct {
	ID     string         `json:"id"`
	Action string         `json:"action"`
	Row    *int           `json:"row,omitempty"`
	Fields map[int]string `json:"fields,omitempty"`
	Text   string         `json:"text,omitempty"`
}

// ApplyBridgePanel keeps desktop controls on the same validation and review
// paths as the terminal. Field changes and the requested key are one action.
func (m Model) ApplyBridgePanel(event BridgePanelAction, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.bridgeNotice = ""
	if m.bridgeExternal != nil {
		m.bridgeNotice = "Finish the current step in Terminal first."
		return m, nil
	}
	panel := m.bridgePanel()
	if panel == nil || event.ID == "" || event.ID != bridgePanelID(*panel) {
		m.status = "This view changed. Review it and try again."
		m.bridgeNotice = m.status
		return m, nil
	}
	if event.Action != "select" && event.Action != "activate" && event.Action != "key" && event.Action != "query" {
		return m, nil
	}
	if m.controlBusy || m.followupSaving {
		// Only cancellation may reach a workflow while it is mutating state.
		if event.Action == "key" && key.Type == tea.KeyEsc {
			return m.Update(key)
		}
		return m, nil
	}
	if event.Row != nil && (*event.Row < 0 || *event.Row >= len(panel.Rows)) {
		return m, nil
	}
	for index, value := range event.Fields {
		if index < 0 || index >= len(panel.Rows) || panel.Rows[index].Input == nil {
			return m, nil
		}
		field := panel.Rows[index].Input
		if len(value) > field.Limit || (!field.Multiline && strings.ContainsAny(value, "\r\n")) || strings.ContainsRune(value, 0) {
			m.status = fmt.Sprintf("Check the value for %s.", panel.Rows[index].Label)
			m.bridgeNotice = m.status
			return m, nil
		}
	}
	if event.Action == "query" {
		if !m.launcher || m.launcherMode == "status-line" || len(event.Text) > 4096 {
			return m, nil
		}
		m.paletteQuery, m.selected = event.Text, 0
		return m, nil
	}
	for index, value := range event.Fields {
		m.setBridgeField(index, value)
	}
	if event.Row != nil {
		m.selectBridgeRow(*event.Row)
	}
	switch event.Action {
	case "activate":
		// Forms must use their explicit review/save action.
		if event.Row == nil || panel.Rows[*event.Row].Input != nil {
			return m, nil
		}
		return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	case "key":
		return m.Update(key)
	}
	return m, nil
}

type bridgeModelFields interface {
	SelectBridgeRow(int)
	SetBridgeField(int, string)
}

func (m *Model) selectBridgeRow(index int) {
	switch {
	case m.models != nil:
		if fields, ok := m.models.(bridgeModelFields); ok {
			fields.SelectBridgeRow(index)
		}
	case m.controlForm != nil:
		m.controlField = index
	case m.followupForm != nil:
		m.followupForm.Field = index
	case m.learningForm != nil:
		m.learningForm.Field = index
	default:
		m.selected = index
	}
}

func (m *Model) setBridgeField(index int, value string) {
	switch {
	case m.models != nil:
		if fields, ok := m.models.(bridgeModelFields); ok {
			fields.SetBridgeField(index, value)
		}
	case m.controlForm != nil:
		m.controlForm.Fields[index].Value = value
	case m.followupForm != nil:
		d := &m.followupForm.Draft
		switch index {
		case 0:
			d.Title = value
		case 1:
			d.Outcome = value
		case 2:
			d.SourcePath = value
		case 4:
			d.GmailQuery = value
		case 7:
			d.AlertWhen = value
		}
	case m.learningForm != nil:
		if index == 2 {
			m.learningForm.Key = value
		} else if index == 3 {
			m.learningForm.Content = value
		}
	case m.contextEditing:
		m.contextEdit = value
	}
}

func bridgeInput(value string, limit int, multiline bool) *uisnapshot.Input {
	return &uisnapshot.Input{Value: value, Limit: limit, Multiline: multiline}
}
