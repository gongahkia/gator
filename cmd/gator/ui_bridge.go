package main

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/worktui"
)

//go:embed ui/gator-ui.cjs
var terminalUIBundle []byte

// launchTerminalUI starts the bundled React renderer. A missing Node runtime
// leaves the native Go interface available, including in single-file releases.
func launchTerminalUI(conversationID, goalID string) (bool, error) {
	if os.Getenv("GATOR_NATIVE_TUI") == "1" {
		return false, nil
	}
	node, err := exec.LookPath("node")
	if err != nil {
		return false, nil
	}
	binary, err := os.Executable()
	if err != nil {
		return false, err
	}
	bundle, err := os.CreateTemp("", "gator-ui-*.cjs")
	if err != nil {
		return false, err
	}
	defer os.Remove(bundle.Name())
	if err := bundle.Chmod(0o600); err != nil {
		bundle.Close()
		return false, err
	}
	if _, err := bundle.Write(terminalUIBundle); err != nil {
		bundle.Close()
		return false, err
	}
	if err := bundle.Close(); err != nil {
		return false, err
	}
	command := exec.Command(node, bundle.Name(), binary, conversationID, goalID)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return true, command.Run()
}

type uiFrame struct {
	Version int    `json:"version"`
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Key     string `json:"key,omitempty"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
}

type uiBridgeModel struct {
	model worktui.Model
	out   *json.Encoder
	last  []byte
}

func (m uiBridgeModel) Init() tea.Cmd {
	m.sendState()
	return m.model.Init()
}

func (m uiBridgeModel) Update(value tea.Msg) (tea.Model, tea.Cmd) {
	var next tea.Model
	var command tea.Cmd
	switch frame := value.(type) {
	case uiFrame:
		switch frame.Type {
		case "submit":
			next, command = m.model.SubmitText(frame.Text)
		case "key":
			key, ok := bridgeKey(frame.Key)
			if !ok {
				return m, nil
			}
			next, command = m.model.Update(key)
		case "resize":
			next, command = m.model.Update(tea.WindowSizeMsg{Width: max(20, frame.Width), Height: max(8, frame.Height)})
		default:
			return m, nil
		}
	default:
		next, command = m.model.Update(value)
	}
	m.model = next.(worktui.Model)
	m.sendState()
	return m, command
}

func (m uiBridgeModel) View() string { return "" }

func (m *uiBridgeModel) sendState() {
	state, err := json.Marshal(m.model.BridgeState())
	if err != nil || bytes.Equal(state, m.last) {
		return
	}
	m.last = state
	_ = m.out.Encode(struct {
		Version int             `json:"version"`
		Type    string          `json:"type"`
		State   json.RawMessage `json:"state"`
	}{Version: 1, Type: "state", State: state})
}

func bridgeKey(name string) (tea.KeyMsg, bool) {
	special := map[string]tea.KeyType{
		"enter": tea.KeyEnter, "esc": tea.KeyEsc, "up": tea.KeyUp, "down": tea.KeyDown,
		"left": tea.KeyLeft, "right": tea.KeyRight, "tab": tea.KeyTab,
		"backspace": tea.KeyBackspace, "delete": tea.KeyDelete,
		"pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
		"ctrl+c": tea.KeyCtrlC, "ctrl+p": tea.KeyCtrlP, "ctrl+x": tea.KeyCtrlX,
		"ctrl+i": tea.KeyCtrlI, "ctrl+b": tea.KeyCtrlB, "ctrl+j": tea.KeyCtrlJ,
		"ctrl+g": tea.KeyCtrlG, "ctrl+u": tea.KeyCtrlU,
	}
	if kind, ok := special[name]; ok {
		return tea.KeyMsg{Type: kind}, true
	}
	if name == "space" {
		return tea.KeyMsg{Type: tea.KeySpace}, true
	}
	if name != "" && !strings.ContainsAny(name, "\r\n") {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}, true
	}
	return tea.KeyMsg{}, false
}

func runUIBridge(application worktui.Model) error {
	program := tea.NewProgram(uiBridgeModel{model: application, out: json.NewEncoder(os.Stdout)}, tea.WithoutRenderer(), tea.WithInput(nil))
	inputDone := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 4096), 4*1024*1024)
		for scanner.Scan() {
			var frame uiFrame
			decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&frame); err != nil || frame.Version != 1 {
				inputDone <- fmt.Errorf("invalid UI frame")
				program.Quit()
				return
			}
			if frame.Type == "exit" {
				program.Quit()
				inputDone <- nil
				return
			}
			program.Send(frame)
		}
		inputDone <- scanner.Err()
		program.Quit()
	}()
	final, err := program.Run()
	if model, ok := final.(uiBridgeModel); ok {
		model.model.Close()
	} else {
		application.Close()
	}
	select {
	case inputErr := <-inputDone:
		if inputErr != nil {
			return errors.Join(err, inputErr)
		}
	default:
	}
	return err
}
