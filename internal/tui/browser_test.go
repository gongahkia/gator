package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	gatorbrowser "github.com/gongahkia/gator/internal/browser"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/sandbox"
)

func TestBrowserStartCommandSelectsManagedSessionForNextRun(t *testing.T) {
	backend := &fakeBrowserBackend{started: gatorbrowser.Session{ID: "browser-1234567890", State: gatorbrowser.StateRunning, Mode: gatorbrowser.ModeManaged, SelectedTabs: []gatorbrowser.Tab{{ID: "tab-1", URL: "about:blank"}}}}
	model := New(Config{Browser: backend})
	model.task.SetValue("/browser start visual")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil {
		t.Fatal("browser start unexpectedly returned an async command")
	}
	updated := next.(Model)
	if !backend.startHeaded || !backend.startVisual || updated.browserSession != backend.started.ID {
		t.Fatalf("browser start = headed:%t visual:%t selected:%q", backend.startHeaded, backend.startVisual, updated.browserSession)
	}
}

func TestBrowserSessionRequiresNetworkAllowAndSelectedTabs(t *testing.T) {
	session := gatorbrowser.Session{ID: "browser-1234567890", State: gatorbrowser.StateRunning, SelectedTabs: []gatorbrowser.Tab{{ID: "tab-1", URL: "https://example.test/"}}}
	backend := &fakeBrowserBackend{controller: testBrowserController{session: session}}
	model := New(Config{Browser: backend})
	model.browserSession = session.ID
	request := &gatorrun.Request{BrowserSession: session.ID, Mode: gatorrun.ExecuteMode}
	if _, err := model.applyBrowserSession(gatorrun.Executor{Sandbox: sandbox.Policy{Network: sandbox.DenyNetwork}}, request); err == nil {
		t.Fatal("browser session was allowed with network deny")
	}
	executor, err := model.applyBrowserSession(gatorrun.Executor{Sandbox: sandbox.Policy{Network: sandbox.AllowNetwork}}, request)
	if err != nil || executor.Browser == nil {
		t.Fatalf("apply browser session = %#v, %v", executor.Browser, err)
	}
}

type fakeBrowserBackend struct {
	started     gatorbrowser.Session
	startHeaded bool
	startVisual bool
	controller  gatorbrowser.Controller
}

func (*fakeBrowserBackend) Sessions() ([]gatorbrowser.Session, error) { return nil, nil }
func (backend *fakeBrowserBackend) Start(headed, visual bool) (gatorbrowser.Session, error) {
	backend.startHeaded, backend.startVisual = headed, visual
	return backend.started, nil
}
func (*fakeBrowserBackend) Attach(string, bool) (gatorbrowser.Session, []gatorbrowser.Tab, error) {
	return gatorbrowser.Session{}, nil, nil
}
func (*fakeBrowserBackend) CandidateTabs(string) ([]gatorbrowser.Tab, error) { return nil, nil }
func (*fakeBrowserBackend) SelectTabs(string, []string) (gatorbrowser.Session, error) {
	return gatorbrowser.Session{}, nil
}
func (*fakeBrowserBackend) AddOrigin(string, string) (gatorbrowser.Session, error) {
	return gatorbrowser.Session{}, nil
}
func (*fakeBrowserBackend) RemoveOrigin(string, string) (gatorbrowser.Session, error) {
	return gatorbrowser.Session{}, nil
}
func (*fakeBrowserBackend) SetVisualCapture(string, bool) (gatorbrowser.Session, error) {
	return gatorbrowser.Session{}, nil
}
func (*fakeBrowserBackend) AllowUpload(string, string) (gatorbrowser.Upload, error) {
	return gatorbrowser.Upload{}, nil
}
func (*fakeBrowserBackend) Artifacts(string) ([]gatorbrowser.Artifact, error) { return nil, nil }
func (*fakeBrowserBackend) Stop(string) (gatorbrowser.Session, error) {
	return gatorbrowser.Session{}, nil
}
func (backend *fakeBrowserBackend) Controller(string) (gatorbrowser.Controller, error) {
	return backend.controller, nil
}

type testBrowserController struct {
	gatorbrowser.UnavailableController
	session gatorbrowser.Session
}

func (controller testBrowserController) Session(context.Context, string) (gatorbrowser.Session, error) {
	return controller.session, nil
}
