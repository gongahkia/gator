package main

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/worktui"
)

//go:embed ui/gator-ui.cjs
var terminalUIBundle []byte

// launchTerminalUI starts the bundled React renderer. Interactive Work requires
// Node 22 or newer; headless commands do not depend on this runtime.
func launchTerminalUI(conversationID, goalID string) error {
	return launchBundledUI([]string{"work", conversationID, goalID}, os.Stdout, "interactive Gator")
}

func launchDoctorUI(arguments []string, out io.Writer) error {
	return launchBundledUI(append([]string{"doctor"}, arguments...), out, "gator doctor")
}

func launchBundledUI(arguments []string, out io.Writer, surface string) error {
	node, err := exec.LookPath("node")
	if err != nil {
		return fmt.Errorf("%s requires Node.js 22 or newer; install Node.js and retry", surface)
	}
	if !supportedNodeUI(node) {
		return fmt.Errorf("%s requires Node.js 22 or newer; upgrade Node.js and retry", surface)
	}
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	bundle, err := os.CreateTemp("", "gator-ui-*.cjs")
	if err != nil {
		return err
	}
	defer os.Remove(bundle.Name())
	if err := bundle.Chmod(0o600); err != nil {
		bundle.Close()
		return err
	}
	if _, err := bundle.Write(terminalUIBundle); err != nil {
		bundle.Close()
		return err
	}
	if err := bundle.Close(); err != nil {
		return err
	}
	command := exec.Command(node, append([]string{bundle.Name(), binary}, arguments...)...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, out, os.Stderr
	if settings, err := loadSettings(); err == nil {
		command.Env = append(os.Environ(), "GATOR_UI_THEME="+settings.Theme)
	}
	// The child may hand the terminal to an editor. Ctrl+C should interrupt that
	// foreground job and leave the launcher waiting for the React UI to resume.
	signal.Ignore(os.Interrupt)
	defer signal.Reset(os.Interrupt)
	return command.Run()
}

func supportedNodeUI(node string) bool {
	version, err := exec.Command(node, "--version").Output()
	if err != nil {
		return false
	}
	major, err := strconv.Atoi(strings.SplitN(strings.TrimPrefix(strings.TrimSpace(string(version)), "v"), ".", 2)[0])
	return err == nil && major >= 22
}

type uiFrame struct {
	Version int    `json:"version"`
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Key     string `json:"key,omitempty"`
	Error   string `json:"error,omitempty"`
	Width   int    `json:"width,omitempty"`
	Height  int    `json:"height,omitempty"`
}

type uiBridgeModel struct {
	model worktui.Model
	out   *json.Encoder
	last  []byte
}

func (m *uiBridgeModel) Init() tea.Cmd {
	m.sendState()
	return m.model.Init()
}

func (m *uiBridgeModel) Update(value tea.Msg) (tea.Model, tea.Cmd) {
	var next tea.Model
	var command tea.Cmd
	switch frame := value.(type) {
	case uiFrame:
		switch frame.Type {
		case "submit":
			next, command = m.model.SubmitText(frame.Text)
		case "key":
			if frame.Key == "ctrl+g" || frame.Key == "ctrl+o" {
				m.model = m.model.SetBridgeInput(frame.Text)
			}
			key, ok := bridgeKey(frame.Key)
			if !ok {
				return m, nil
			}
			next, command = m.model.Update(key)
		case "external_done":
			next, command = m.model.FinishBridgeExternal(frame.Error)
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

func (m *uiBridgeModel) View() string { return "" }

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
		"left": tea.KeyLeft, "right": tea.KeyRight, "tab": tea.KeyTab, "shift+tab": tea.KeyShiftTab,
		"backspace": tea.KeyBackspace, "delete": tea.KeyDelete,
		"pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
		"ctrl+c": tea.KeyCtrlC, "ctrl+p": tea.KeyCtrlP, "ctrl+x": tea.KeyCtrlX,
		"ctrl+i": tea.KeyCtrlI, "ctrl+b": tea.KeyCtrlB, "ctrl+j": tea.KeyCtrlJ,
		"ctrl+g": tea.KeyCtrlG, "ctrl+o": tea.KeyCtrlO, "ctrl+u": tea.KeyCtrlU, "ctrl+r": tea.KeyCtrlR,
		"ctrl+q": tea.KeyCtrlQ, "f1": tea.KeyF1,
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
	// The renderer handles Ctrl+C in chat. During an external terminal handoff,
	// the editor or login process receives SIGINT; keep the backend session alive.
	signal.Ignore(os.Interrupt)
	defer signal.Reset(os.Interrupt)
	return runUIBridgeWithIO(application, os.Stdin, os.Stdout)
}

func runUIBridgeWithIO(application worktui.Model, in io.Reader, out io.Writer) error {
	program := tea.NewProgram(&uiBridgeModel{model: application, out: json.NewEncoder(out)}, tea.WithoutRenderer(), tea.WithInput(nil), tea.WithoutSignalHandler())
	inputDone := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(in)
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
	if model, ok := final.(*uiBridgeModel); ok {
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
