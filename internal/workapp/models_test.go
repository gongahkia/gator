package workapp

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/uisnapshot"
)

func TestDesktopModelSetupExitReconcilesReadinessWithoutSending(t *testing.T) {
	for _, destination := range []string{"/chat", "/conversations"} {
		for _, configured := range []bool{false, true} {
			t.Run(destination+"/"+map[bool]string{false: "unconfigured", true: "configured"}[configured], func(t *testing.T) {
				provider := ""
				panel := &testModelPanel{}
				m := New(Config{FirstRun: true, CurrentFolder: "/work", Models: func() (ModelPanel, error) { return panel, nil },
					SelectedModel: func() (string, string, error) { return provider, "fixture", nil },
					Run:           func(_, _, _ string, _ RunOptions) RunResult { t.Fatal("setup sent a request"); return RunResult{} },
				})
				next, _ := m.openModels()
				m = next.(Model)
				m.pendingPrompt = "understand this file"
				if configured {
					provider = "fixture"
				}
				next, _ = m.NavigateBridge(destination)
				m = next.(Model)
				if m.firstRun == configured || m.models != nil || !panel.closed || m.input != "understand this file" || m.pendingPrompt != "" || m.running {
					t.Fatalf("setup exit lost readiness or draft: %#v", m)
				}
			})
		}
	}
	panel := &testModelPanel{}
	m := New(Config{FirstRun: true, SelectedModel: func() (string, string, error) { return "", "", errors.New("unavailable") }})
	m.models = panel
	next, _ := m.NavigateBridge("/chat")
	if !next.(Model).firstRun {
		t.Fatal("failed selection read marked setup complete")
	}
}

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

func TestModelsAliasOpensTheSameModelPicker(t *testing.T) {
	panel := &testModelPanel{}
	m := New(Config{Models: func() (ModelPanel, error) { return panel, nil }})
	next, _ := m.runLocalCommand("/models")
	if next.(Model).models != panel {
		t.Fatal("/models did not open the canonical model picker")
	}
}

func TestModelManagementReturnsToSameConversationAndCancelsOnTerminalClose(t *testing.T) {
	panel := &testModelPanel{}
	m := New(Config{Models: func() (ModelPanel, error) { return panel, nil }})
	m.home, m.conversation, m.source = false, "retained-conversation", "/selected/source"
	m.input = "unfinished draft"
	next, _ := m.openModels()
	m = next.(Model)
	if panel := m.BridgeState().Panel; panel == nil || panel.Title != "Models" {
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
func (m *testModelPanel) UISnapshot() uisnapshot.Panel {
	return uisnapshot.Panel{Kind: "models", Title: "Models"}
}
func (m *testModelPanel) Close()       { m.closed = true }
func (m *testModelPanel) Closed() bool { return m.closed }
func (m *testModelPanel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyMsg); ok && key.Type == tea.KeyEsc {
		m.closed = true
	}
	return m, nil
}
