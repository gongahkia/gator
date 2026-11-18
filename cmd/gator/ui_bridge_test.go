package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/modelcatalog"
	"github.com/gongahkia/gator/internal/workapp"
	"github.com/gongahkia/gator/internal/worksession"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestUIBridgeNativeCatalogAcknowledgementIsNotQueryOrRenameCompletion(t *testing.T) {
	root := t.TempDir()
	store, err := worksession.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.CreateChat(root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	bridge := &uiBridgeModel{model: workapp.New(workapp.Config{BridgeMode: true, ManagedSessions: true, SessionStore: &store, QueryConversations: store.PageConversationsContext, OrganizeConversation: store.OrganizeConversation}), out: json.NewEncoder(&out)}
	_, cmd := bridge.Update(uiFrame{Type: "native_catalog", Nonce: "query-nonce", NativeCatalog: &workapp.NativeCatalogAction{RequestID: "query-exact", Action: "query", Scope: "all"}})
	if cmd == nil || bridge.model.BridgeState().NativeCatalogQueryResult != nil || !bytes.Contains(out.Bytes(), []byte(`"type":"action_applied"`)) {
		t.Fatal("query acknowledgement conflated with result", out.String())
	}
	bridge.Update(cmd())
	page := bridge.model.BridgeState().NativeCatalogQueryResult
	if page == nil || page.RequestID != "query-exact" || page.Page.Items[0].ID != c.ID {
		t.Fatal(page)
	}
	item := page.Page.Items[0]
	out.Reset()
	_, cmd = bridge.Update(uiFrame{Type: "native_catalog", Nonce: "rename-nonce", NativeCatalog: &workapp.NativeCatalogAction{RequestID: "rename-exact", Action: "rename", PageID: page.PageID, ID: item.ID, Version: item.Version, Title: "Confirmed title"}})
	c, err = store.Load(c.ID)
	if err != nil || c.Title == "Confirmed title" || bridge.model.BridgeState().NativeCatalogActionResult != nil || cmd == nil || !bytes.Contains(out.Bytes(), []byte(`"nonce":"rename-nonce"`)) {
		t.Fatal("ack prematurely confirmed rename", out.String(), err)
	}
	if bridge.model.RestartBlockReason() == "" {
		t.Fatal("restart accepted an unfinished metadata change")
	}
	bridge.Update(cmd())
	result := bridge.model.BridgeState().NativeCatalogActionResult
	if result == nil || result.RequestID != "rename-exact" || result.Error != "" || result.Item.Title != "Confirmed title" {
		t.Fatal(result)
	}
}

func TestUIBridgeUpdateReadinessAcknowledgesBeforeQuitting(t *testing.T) {
	var out bytes.Buffer
	bridge := &uiBridgeModel{model: workapp.New(workapp.Config{CurrentFolder: t.TempDir()}), out: json.NewEncoder(&out)}
	_, command := bridge.Update(uiFrame{Type: "prepare_update", Nonce: "reviewed-update"})
	var response struct {
		Type  string
		Nonce string
		Ready bool
	}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &response); err != nil {
		t.Fatal(err)
	}
	if response.Type != "update_ready" || response.Nonce != "reviewed-update" || !response.Ready || command == nil {
		t.Fatalf("update response: %s", out.String())
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatal("idle acknowledgement did not quit")
	}
}

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
		finished <- runUIBridgeWithIO(workapp.New(workapp.Config{CurrentFolder: t.TempDir(), Models: func() (workapp.ModelPanel, error) {
			return modelcatalog.NewModelCatalogPanel(modelcatalog.Config{}), nil
		}}), input, receive)
		receive.Close()
	}()
	frames := make(chan workapp.BridgeState, 4)
	go func() {
		decoder := json.NewDecoder(output)
		for {
			var frame struct {
				Version int                 `json:"version"`
				Type    string              `json:"type"`
				State   workapp.BridgeState `json:"state"`
			}
			if decoder.Decode(&frame) != nil {
				return
			}
			frames <- frame.State
		}
	}()
	waitState := func() workapp.BridgeState {
		t.Helper()
		select {
		case state := <-frames:
			return state
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for UI state")
			return workapp.BridgeState{}
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
		finished <- runUIBridgeWithIO(workapp.New(workapp.Config{CurrentFolder: t.TempDir(), BridgeMode: true}), input, receive)
		receive.Close()
	}()
	decoder := json.NewDecoder(output)
	readState := func() workapp.BridgeState {
		t.Helper()
		var frame struct {
			State workapp.BridgeState `json:"state"`
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

func TestUIBridgeWorkspaceAckPrecedesExactResolutionWithoutNavigation(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	bridge := &uiBridgeModel{model: workapp.New(workapp.Config{BridgeMode: true, ManagedSessions: true, CurrentFolder: root, ResolveSource: func(path string) (string, error) { r, e := workspace.Open(path); return r.Path(), e }}), out: json.NewEncoder(&out)}
	_, cmd := bridge.Update(uiFrame{Type: "native_workspace", Nonce: "transport-only", NativeWorkspace: &workapp.NativeWorkspaceAction{RequestID: "exact-workspace", Path: root}})
	if cmd == nil || bridge.model.BridgeState().NativeWorkspaceResult != nil || !bytes.Contains(out.Bytes(), []byte(`"nonce":"transport-only"`)) {
		t.Fatal("ack treated as resolution", out.String())
	}
	bridge.Update(cmd())
	result := bridge.model.BridgeState().NativeWorkspaceResult
	if result == nil || result.RequestID != "exact-workspace" || result.Error != "" || bridge.model.BridgeState().SessionRequest != nil {
		t.Fatal("resolution navigated or lost identity", result)
	}
}
