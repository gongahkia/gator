package browser

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestIndependentBrowserStoresRetainConcurrentSessionsAndRevocations(t *testing.T) {
	root := t.TempDir()
	first, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	prior, err := first.Start(StartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store, err := Open(root)
			if err != nil {
				t.Error(err)
				return
			}
			if i == 0 {
				_, err = store.Stop(prior.ID)
			} else {
				_, err = store.Start(StartOptions{Paused: true})
			}
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	sessions, err := first.List()
	if err != nil || len(sessions) != 24 {
		t.Fatalf("lost sessions: %d %v", len(sessions), err)
	}
	stopped, err := first.Get(prior.ID)
	if err != nil || stopped.State != StateStopped {
		t.Fatalf("revocation lost: %#v %v", stopped, err)
	}
}

func TestStorePersistsSafeSessionMetadataAndStops(t *testing.T) {
	stateDir := t.TempDir()
	store, err := Open(stateDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	store.now = func() time.Time { return time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC) }
	session, err := store.Start(StartOptions{Headed: true, VisualCapture: true})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if session.Mode != ModeManaged || !session.Headed || !session.VisualCapture || session.State != StateRunning {
		t.Fatalf("started session = %#v", session)
	}
	session, err = store.SelectTabs(session.ID, []Tab{{ID: "tab-a", Title: "example", URL: "https://example.com/"}})
	if err != nil {
		t.Fatalf("SelectTabs: %v", err)
	}
	lookup := resolver{"example.com": {{IP: net.ParseIP("93.184.216.34")}}}
	session, err = store.AddOrigin(context.Background(), session.ID, "https://example.com", lookup)
	if err != nil {
		t.Fatalf("AddOrigin: %v", err)
	}
	_, upload, err := store.AllowUpload(session.ID, "fixture.png")
	if err != nil {
		t.Fatalf("AllowUpload: %v", err)
	}
	if upload.ID == "" || upload.Name != "fixture.png" {
		t.Fatalf("upload = %#v", upload)
	}
	contents, err := os.ReadFile(filepath.Join(store.Directory(), sessionsFile))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if !strings.Contains(string(contents), session.ID) || strings.Contains(string(contents), "contains-secret") {
		t.Fatalf("unexpected session state: %q", contents)
	}
	if info, err := os.Stat(filepath.Join(store.Directory(), sessionsFile)); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("session file mode = %v, %v", info.Mode(), err)
	}
	stopped, err := store.Stop(session.ID)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if stopped.State != StateStopped || stopped.StoppedAt == nil || len(stopped.SelectedTabs) != 0 || len(stopped.AllowedUploads) != 0 {
		t.Fatalf("stopped session = %#v", stopped)
	}
	if _, err := store.SetVisualCapture(session.ID, false); err != ErrStopped {
		t.Fatalf("SetVisualCapture stopped error = %v", err)
	}
}

func TestStoreAttachedSessionDoesNotPersistCDPEndpoint(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	session, err := store.Attach(AttachOptions{CDPEndpoint: "http://127.0.0.1:9222"})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	contents, err := os.ReadFile(filepath.Join(store.Directory(), sessionsFile))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if got := string(contents); strings.Contains(got, "9222") || strings.Contains(got, "127.0.0.1") {
		t.Fatalf("private CDP endpoint persisted in %q", got)
	}
	if session.Mode != ModeAttached || session.Headed {
		t.Fatalf("attached session = %#v", session)
	}
}
