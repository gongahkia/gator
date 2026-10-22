package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/workapp"
)

type uiFrame struct {
	Version       int                               `json:"version"`
	Type          string                            `json:"type"`
	Text          string                            `json:"text,omitempty"`
	Key           string                            `json:"key,omitempty"`
	Error         string                            `json:"error,omitempty"`
	Width         int                               `json:"width,omitempty"`
	Height        int                               `json:"height,omitempty"`
	Panel         *workapp.BridgePanelAction        `json:"panel,omitempty"`
	Queue         *workapp.BridgeQueueAction        `json:"queue,omitempty"`
	Conversation  *workapp.BridgeConversationAction `json:"conversation,omitempty"`
	InteractionID int                               `json:"interaction_id,omitempty"`
	Approved      bool                              `json:"approved,omitempty"`
	Nonce         string                            `json:"nonce,omitempty"`
}

type uiBridgeModel struct {
	model workapp.Model
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
		case "prepare_update":
			if frame.Nonce == "" || len(frame.Nonce) > 128 {
				return m, nil
			}
			reason := m.model.RestartBlockReason()
			_ = m.out.Encode(struct {
				Version int    `json:"version"`
				Type    string `json:"type"`
				Nonce   string `json:"nonce"`
				Ready   bool   `json:"ready"`
				Reason  string `json:"reason,omitempty"`
			}{1, "update_ready", frame.Nonce, reason == "", reason})
			if reason == "" {
				return m, tea.Quit
			}
			return m, nil
		case "submit":
			next, command = m.model.SubmitText(frame.Text)
		case "navigate":
			next, command = m.model.NavigateBridge(frame.Text)
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
		case "panel":
			if frame.Panel == nil {
				return m, nil
			}
			key, ok := bridgeKey(frame.Key)
			if frame.Panel.Action == "key" && !ok {
				return m, nil
			}
			next, command = m.model.ApplyBridgePanel(*frame.Panel, key)
		case "approval":
			next, command = m.model.RespondBridgeApproval(frame.InteractionID, frame.Approved)
		case "queue":
			if frame.Queue == nil {
				return m, nil
			}
			next, command = m.model.ApplyBridgeQueue(*frame.Queue)
		case "conversation":
			if frame.Conversation == nil {
				return m, nil
			}
			next, command = m.model.OpenBridgeConversation(*frame.Conversation)
		case "resize":
			next, command = m.model.Update(tea.WindowSizeMsg{Width: max(20, frame.Width), Height: max(8, frame.Height)})
		default:
			return m, nil
		}
	default:
		next, command = m.model.Update(value)
	}
	m.model = next.(workapp.Model)
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

func runUIBridge(application workapp.Model) error {
	// The renderer handles Ctrl+C in chat. During an external terminal handoff,
	// the editor or login process receives SIGINT; keep the backend session alive.
	signal.Ignore(os.Interrupt)
	defer signal.Reset(os.Interrupt)
	return runUIBridgeWithIO(application, os.Stdin, os.Stdout)
}

func runUIBridgeWithIO(application workapp.Model, in io.Reader, out io.Writer) error {
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
