package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/modelcatalog"
	"github.com/gongahkia/gator/internal/worktui"
)

func TestBridgeKeyMapsTerminalControls(t *testing.T) {
	for name, want := range map[string]tea.KeyType{"esc": tea.KeyEsc, "enter": tea.KeyEnter, "ctrl+c": tea.KeyCtrlC, "up": tea.KeyUp, "shift+tab": tea.KeyShiftTab} {
		key, ok := bridgeKey(name)
		if !ok || key.Type != want {
			t.Fatalf("bridgeKey(%q) = %#v, %v", name, key, ok)
		}
	}
	if _, ok := bridgeKey("\n"); ok {
		t.Fatal("bridgeKey accepted a newline")
	}
}

func TestUIBridgeRoutesCommandAndPublishesState(t *testing.T) {
	input, send := io.Pipe()
	output, receive := io.Pipe()
	defer input.Close()
	defer send.Close()
	defer output.Close()
	defer receive.Close()
	finished := make(chan error, 1)
	go func() {
		finished <- runUIBridgeWithIO(worktui.New(worktui.Config{CurrentFolder: t.TempDir(), Models: func() (worktui.ModelPanel, error) {
			return modelcatalog.NewModelCatalogPanel(modelcatalog.Config{}), nil
		}}), input, receive)
		receive.Close()
	}()
	frames := make(chan worktui.BridgeState, 4)
	go func() {
		decoder := json.NewDecoder(output)
		for {
			var frame struct {
				Version int                 `json:"version"`
				Type    string              `json:"type"`
				State   worktui.BridgeState `json:"state"`
			}
			if decoder.Decode(&frame) != nil {
				return
			}
			frames <- frame.State
		}
	}()
	waitState := func() worktui.BridgeState {
		t.Helper()
		select {
		case state := <-frames:
			return state
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for UI state")
			return worktui.BridgeState{}
		}
	}
	if initial := waitState(); !initial.Home || initial.Mode != "chat" {
		t.Fatalf("initial UI state = %#v", initial)
	}
	if _, err := send.Write([]byte("{\"version\":1,\"type\":\"submit\",\"text\":\"/help\"}\n")); err != nil {
		t.Fatal(err)
	}
	if state := waitState(); len(state.Messages) != 1 || state.Messages[0].Role != "Gator" {
		t.Fatalf("submitted UI state = %#v", state)
	}
	if _, err := send.Write([]byte("{\"version\":1,\"type\":\"submit\",\"text\":\"/model\"}\n")); err != nil {
		t.Fatal(err)
	}
	if state := waitState(); state.Mode != "panel" || state.Panel == nil || state.Panel.Title != "Models" {
		t.Fatalf("model UI state = %#v", state)
	}
	if _, err := send.Write([]byte("{\"version\":1,\"type\":\"exit\"}\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("UI bridge did not exit")
	}
}

func TestUIBridgeHandsEditorToFrontendAndRestoresDraft(t *testing.T) {
	t.Setenv("VISUAL", "true")
	input, send := io.Pipe()
	output, receive := io.Pipe()
	defer input.Close()
	defer send.Close()
	defer output.Close()
	defer receive.Close()
	finished := make(chan error, 1)
	go func() {
		finished <- runUIBridgeWithIO(worktui.New(worktui.Config{CurrentFolder: t.TempDir(), BridgeMode: true}), input, receive)
		receive.Close()
	}()
	decoder := json.NewDecoder(output)
	readState := func() worktui.BridgeState {
		t.Helper()
		var frame struct {
			State worktui.BridgeState `json:"state"`
		}
		if err := decoder.Decode(&frame); err != nil {
			t.Fatal(err)
		}
		return frame.State
	}
	_ = readState()
	if _, err := send.Write([]byte("{\"version\":1,\"type\":\"key\",\"key\":\"ctrl+g\",\"text\":\"before editor\"}\n")); err != nil {
		t.Fatal(err)
	}
	state := readState()
	if state.External == nil || state.External.Kind != "editor" || state.Input != "before editor" {
		t.Fatalf("editor request = %#v", state)
	}
	path := state.External.Args[len(state.External.Args)-1]
	if err := os.WriteFile(path, []byte("after editor"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := send.Write([]byte("{\"version\":1,\"type\":\"external_done\"}\n")); err != nil {
		t.Fatal(err)
	}
	state = readState()
	if state.External != nil || state.Input != "after editor" {
		t.Fatalf("restored editor state = %#v", state)
	}
	if _, err := send.Write([]byte("{\"version\":1,\"type\":\"exit\"}\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("UI bridge did not exit")
	}
}

func TestNodeRuntimeGuard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Gator does not support Windows")
	}
	path := filepath.Join(t.TempDir(), "node")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf 'v21.9.0\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if supportedNodeUI(path) {
		t.Fatal("Node 21 unexpectedly passed the UI guard")
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf 'v22.0.0\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if !supportedNodeUI(path) {
		t.Fatal("Node 22 did not pass the UI guard")
	}
}
