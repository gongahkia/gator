package worktui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModelManagementCannotOpenDuringWork(t *testing.T) {
	m := New(Config{Models: func() (ModelPanel, error) {
		t.Fatal("model management opened during a live Work run")
		return nil, nil
	}})
	m.running = true
	next, command := m.runLocalCommand("/model")
	if command != nil || !next.(Model).running || !strings.Contains(next.(Model).status, "Finish or cancel") {
		t.Fatal("model management did not preserve the running operation")
	}
}

func TestModelManagementReturnsToSameConversationAndCancelsOnTerminalClose(t *testing.T) {
	panel := &testModelPanel{}
	m := New(Config{Models: func() (ModelPanel, error) { return panel, nil }})
	m.home, m.conversation, m.source = false, "retained-conversation", "/selected/source"
	m.input = "unfinished draft"
	next, _ := m.openModels()
	m = next.(Model)
	if m.View() != "model panel" {
		t.Fatal("model panel was not rendered")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.models != nil || m.home || m.conversation != "retained-conversation" || m.source != "/selected/source" || m.input != "unfinished draft" {
		t.Fatal("model management changed the active conversation or draft")
	}
	panel.closed = false
	next, _ = m.openModels()
	next.(Model).Close()
	if !panel.closed {
		t.Fatal("terminal close did not release the model panel")
	}
}

type testModelPanel struct{ closed bool }

func (m *testModelPanel) Init() tea.Cmd { return nil }
func (m *testModelPanel) View() string  { return "model panel" }
func (m *testModelPanel) Close()        { m.closed = true }
func (m *testModelPanel) Closed() bool  { return m.closed }
func (m *testModelPanel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyMsg); ok && key.Type == tea.KeyEsc {
		m.closed = true
	}
	return m, nil
}
