package browser

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A real child process that accepts a request but never replies. EOF normally
// exits cleanly; the stubborn variant also ignores EOF to exercise forced stop.
func TestSidecarProcessHelper(t *testing.T) {
	mode := os.Getenv("GATOR_SIDECAR_TEST_HELPER")
	if mode == "" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID     uint64 `json:"id"`
			Method string `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			os.Exit(2)
		}
		if request.Method == "snapshot" {
			_ = os.WriteFile(os.Getenv("GATOR_SIDECAR_TEST_MARKER"), []byte("admitted"), 0o600)
			if mode == "stubborn" {
				for {
					time.Sleep(time.Hour)
				}
			}
			continue
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"id": request.ID, "result": map[string]any{}})
	}
	os.Exit(0)
}

func hungSidecar(t *testing.T, stubborn bool) (*sidecar, string) {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "admitted")
	mode := "cooperative"
	if stubborn {
		mode = "stubborn"
	}
	command := exec.Command(os.Args[0], "-test.run=^TestSidecarProcessHelper$")
	command.Env = append(os.Environ(), "GATOR_SIDECAR_TEST_HELPER="+mode, "GATOR_SIDECAR_TEST_MARKER="+marker)
	driver, err := startSidecarProcess(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = driver.Close() })
	return driver, marker
}

func waitForSidecarAdmission(t *testing.T, marker string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not receive request")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSidecarCancellationClosesAdmittedStreamButNotQueuedCall(t *testing.T) {
	driver, marker := hungSidecar(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := driver.Snapshot(ctx, "tab"); result <- err }()
	waitForSidecarAdmission(t, marker)
	queued, cancelQueued := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancelQueued()
	if _, err := driver.CandidateTabs(queued); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued call: %v", err)
	}
	select {
	case <-driver.Done():
		t.Fatal("queued cancellation stopped admitted work")
	default:
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "outcome may be unknown") {
			t.Fatalf("interrupted call: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt pipe read")
	}
	if err := driver.Close(); err != nil {
		t.Fatal(err)
	}
	if !driver.process.ProcessState.Success() {
		t.Fatal("EOF did not allow graceful shutdown")
	}
	if _, err := driver.CandidateTabs(context.Background()); err == nil {
		t.Fatal("closed driver reused")
	}
}

func TestSidecarCloseInterruptsStubbornProcess(t *testing.T) {
	driver, marker := hungSidecar(t, true)
	result := make(chan error, 1)
	go func() { _, err := driver.Snapshot(context.Background(), "tab"); result <- err }()
	waitForSidecarAdmission(t, marker)
	done := make(chan error, 1)
	go func() { done <- driver.Close() }()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("hung request succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("Close waits behind blocked I/O")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("stubborn process was not reaped")
	}
	if driver.process.ProcessState == nil {
		t.Fatal("missing child exit state")
	}
}

func TestServiceRevokesHungEngineOnDisconnectOrStop(t *testing.T) {
	for _, stop := range []bool{false, true} {
		name := "disconnect"
		if stop {
			name = "stop"
		}
		t.Run(name, func(t *testing.T) {
			driver, marker := hungSidecar(t, false)
			store, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			session, err := store.Start(StartOptions{})
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.SelectTabs(session.ID, []Tab{{ID: "tab", URL: "about:blank"}})
			if err != nil {
				t.Fatal(err)
			}
			_, token, err := CreateToken(store, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			service, err := NewService(ServiceConfig{Store: store, SessionID: session.ID, Token: token, Driver: driver, Socket: socketPath(store, session.ID)})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- service.Run(ctx) }()
			waitForBrowserSocket(t, socketPath(store, session.ID))
			client, err := NewClient(store, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			requestCtx, disconnect := context.WithCancel(context.Background())
			defer disconnect()
			result := make(chan error, 1)
			go func() { _, err := client.Snapshot(requestCtx, session.ID, "tab"); result <- err }()
			waitForSidecarAdmission(t, marker)
			if stop {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
				defer stopCancel()
				if _, err := client.Stop(stopCtx); err != nil {
					t.Fatalf("stop blocked behind hung request: %v", err)
				}
			} else {
				disconnect()
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(4 * time.Second):
				t.Fatal("controller did not stop")
			}
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("interrupted request succeeded")
				}
			case <-time.After(time.Second):
				t.Fatal("caller did not return")
			}
			recorded, err := store.Get(session.ID)
			if err != nil || recorded.State != StateStopped {
				t.Fatalf("session not revoked: %#v, %v", recorded, err)
			}
			if _, err := os.Stat(tokenPath(store, session.ID)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("token not removed: %v", err)
			}
		})
	}
}
