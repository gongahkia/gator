package browser

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"testing"
	"time"
)

func TestServiceAuthenticatesAndRestrictsControllerToSelectedTabs(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	session, err := store.Start(StartOptions{VisualCapture: true})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, token, err := CreateToken(store, session.ID)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	driver := &serviceDriver{tabs: []Tab{{ID: "tab-one", URL: "http://127.0.0.1:4500/"}, {ID: "tab-secret", Title: "private", URL: "https://private.example/"}}}
	service, err := NewService(ServiceConfig{Store: store, SessionID: session.ID, Token: token, Driver: driver, Socket: socketPath(store, session.ID)})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- service.Run(ctx) }()
	waitForBrowserSocket(t, socketPath(store, session.ID))
	client, err := NewClient(store, session.ID)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	candidates, err := client.CandidateTabs(context.Background())
	if err != nil || len(candidates) != 2 {
		t.Fatalf("CandidateTabs = %#v, %v", candidates, err)
	}
	if _, err := client.SelectTabs(context.Background(), []Tab{candidates[0]}); err != nil {
		t.Fatalf("SelectTabs: %v", err)
	}
	tabs, err := client.Tabs(context.Background(), session.ID)
	if err != nil || len(tabs) != 1 || tabs[0].ID != "tab-one" {
		t.Fatalf("selected Tabs = %#v, %v", tabs, err)
	}
	if _, err := client.Snapshot(context.Background(), session.ID, "tab-secret"); err == nil || err.Error() != "browser tab is not developer-selected for this session" {
		t.Fatalf("unselected tab error = %v", err)
	}
	if _, err := client.Navigate(context.Background(), session.ID, "tab-one", "http://127.0.0.1:4500/next"); err == nil || err.Error() != "browser URL origin is not approved for this session" {
		t.Fatalf("unapproved navigation error = %v", err)
	}
	if _, err := client.AddOrigin(context.Background(), "http://127.0.0.1:4500"); err != nil {
		t.Fatalf("AddOrigin: %v", err)
	}
	if _, err := client.Navigate(context.Background(), session.ID, "tab-one", "http://127.0.0.1:4500/next"); err != nil {
		t.Fatalf("approved navigation: %v", err)
	}
	capture, err := client.Screenshot(context.Background(), session.ID, "tab-one")
	if err != nil || string(capture.PNG) != "png" || capture.Artifact.Kind != "screenshot" {
		t.Fatalf("Screenshot = %#v, %v", capture, err)
	}
	artifacts, err := client.Artifacts()
	if err != nil || len(artifacts) != 1 || artifacts[0].Name != "screenshot.png" {
		t.Fatalf("Artifacts = %#v, %v", artifacts, err)
	}
	if _, err := client.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("service: %v", err)
	}
	if _, err := os.Stat(tokenPath(store, session.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("session token remained after stop: %v", err)
	}
}

func TestServiceRejectsInvalidToken(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	session, err := store.Start(StartOptions{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, token, err := CreateToken(store, session.ID)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	service, err := NewService(ServiceConfig{Store: store, SessionID: session.ID, Token: token, Driver: &serviceDriver{}, Socket: socketPath(store, session.ID)})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- service.Run(ctx) }()
	waitForBrowserSocket(t, socketPath(store, session.ID))
	connection, err := net.Dial("unix", socketPath(store, session.ID))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if err := json.NewEncoder(connection).Encode(rpcRequest{ID: 1, Token: []byte("wrong"), Method: "session"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	var response rpcResponse
	if err := json.NewDecoder(connection).Decode(&response); err != nil || response.Error != "browser session authentication failed" {
		t.Fatalf("response = %#v, %v", response, err)
	}
	_ = connection.Close()
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("service: %v", err)
	}
}

func waitForBrowserSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.Mode().Perm() == 0o600 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("browser socket %q was not ready", path)
}

type serviceDriver struct {
	tabs    []Tab
	origins []Origin
}

func (driver *serviceDriver) SetOrigins(_ context.Context, origins []Origin) error {
	driver.origins = append([]Origin(nil), origins...)
	return nil
}
func (driver *serviceDriver) CandidateTabs(context.Context) ([]Tab, error) {
	return append([]Tab(nil), driver.tabs...), nil
}
func (*serviceDriver) Snapshot(_ context.Context, tabID string) (Snapshot, error) {
	return Snapshot{TabID: tabID, URL: "http://127.0.0.1:4500/"}, nil
}
func (*serviceDriver) Screenshot(context.Context, string) ([]byte, error) { return []byte("png"), nil }
func (*serviceDriver) Navigate(_ context.Context, tabID, target string) (Snapshot, error) {
	return Snapshot{TabID: tabID, URL: target}, nil
}
func (*serviceDriver) Click(_ context.Context, tabID, _ string) (Snapshot, error) {
	return Snapshot{TabID: tabID}, nil
}
func (*serviceDriver) Fill(_ context.Context, tabID, _, _ string) (Snapshot, error) {
	return Snapshot{TabID: tabID}, nil
}
func (*serviceDriver) Select(_ context.Context, tabID, _, _ string) (Snapshot, error) {
	return Snapshot{TabID: tabID}, nil
}
func (*serviceDriver) Press(_ context.Context, tabID, _ string) (Snapshot, error) {
	return Snapshot{TabID: tabID}, nil
}
func (*serviceDriver) Download(context.Context, string, string) (Download, error) {
	return Download{}, nil
}
func (*serviceDriver) Upload(_ context.Context, tabID, _, _ string) (Snapshot, error) {
	return Snapshot{TabID: tabID}, nil
}
func (*serviceDriver) Close() error { return nil }
