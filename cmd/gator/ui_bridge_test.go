package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBridgeKeyMapsTerminalControls(t *testing.T) {
	for name, want := range map[string]tea.KeyType{"esc": tea.KeyEsc, "enter": tea.KeyEnter, "ctrl+c": tea.KeyCtrlC, "up": tea.KeyUp} {
		key, ok := bridgeKey(name)
		if !ok || key.Type != want {
			t.Fatalf("bridgeKey(%q) = %#v, %v", name, key, ok)
		}
	}
	if _, ok := bridgeKey("\n"); ok {
		t.Fatal("bridgeKey accepted a newline")
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
