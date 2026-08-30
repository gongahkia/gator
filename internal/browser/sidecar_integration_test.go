package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// TestSidecarManagedBrowserIntegration is intentionally opt-in because it
// launches Chromium. Set GATOR_BROWSER_INTEGRATION_STATE_DIR to a private
// state directory whose runtime was explicitly installed with
// `gator browser install`.
func TestSidecarManagedBrowserIntegration(t *testing.T) {
	stateDir := os.Getenv("GATOR_BROWSER_INTEGRATION_STATE_DIR")
	if stateDir == "" {
		t.Skip("set GATOR_BROWSER_INTEGRATION_STATE_DIR to run Chromium integration")
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/":
			_, _ = writer.Write([]byte(`<!doctype html><title>Gator browser fixture</title><button id="change" onclick="document.body.dataset.clicked='yes';this.textContent='clicked'">change</button><script>fetch('/data')</script>`))
		case "/data":
			_, _ = writer.Write([]byte("ok"))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	store, err := Open(stateDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if status := Runtime(store); !status.Installed {
		t.Fatalf("runtime status = %#v", status)
	}
	session, err := store.Start(StartOptions{VisualCapture: true})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, token, err := CreateToken(store, session.ID)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	driver, err := StartSidecar(ctx, SidecarConfig{Store: store, Mode: ModeManaged})
	if err != nil {
		t.Fatalf("StartSidecar: %v", err)
	}
	service, err := NewService(ServiceConfig{Store: store, SessionID: session.ID, Token: token, Driver: driver, Socket: socketPath(store, session.ID)})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- service.Run(ctx) }()
	waitForBrowserSocket(t, socketPath(store, session.ID))
	client, err := NewClient(store, session.ID)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	tabs, err := client.CandidateTabs(context.Background())
	if err != nil || len(tabs) != 1 {
		t.Fatalf("CandidateTabs = %#v, %v", tabs, err)
	}
	if _, err := client.SelectTabs(context.Background(), tabs); err != nil {
		t.Fatalf("SelectTabs: %v", err)
	}
	if _, err := client.AddOrigin(context.Background(), server.URL); err != nil {
		t.Fatalf("AddOrigin: %v", err)
	}
	snapshot, err := client.Navigate(context.Background(), session.ID, tabs[0].ID, server.URL)
	if err != nil || snapshot.Title != "Gator browser fixture" || len(snapshot.Elements) == 0 {
		t.Fatalf("Navigate = %#v, %v", snapshot, err)
	}
	var button string
	for _, element := range snapshot.Elements {
		if element.Role == "button" && strings.Contains(element.Name, "change") {
			button = element.Ref
		}
	}
	if button == "" {
		t.Fatalf("snapshot elements = %#v", snapshot.Elements)
	}
	snapshot, err = client.Click(context.Background(), session.ID, tabs[0].ID, button)
	if err != nil || !strings.Contains(snapshot.Text, "clicked") {
		t.Fatalf("Click = %#v, %v", snapshot, err)
	}
	capture, err := client.Screenshot(context.Background(), session.ID, tabs[0].ID)
	if err != nil || len(capture.PNG) == 0 || capture.Artifact.Kind != "screenshot" {
		t.Fatalf("Screenshot = %#v, %v", capture, err)
	}
	if _, err := client.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("service: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("browser service did not stop")
	}
}
