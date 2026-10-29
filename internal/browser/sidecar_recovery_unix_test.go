//go:build darwin || linux

package browser

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func integrationRecoveryStore(t *testing.T) *Store {
	t.Helper()
	stateDir := os.Getenv("GATOR_BROWSER_INTEGRATION_STATE_DIR")
	if stateDir == "" {
		t.Skip("set GATOR_BROWSER_INTEGRATION_STATE_DIR to a preinstalled test runtime")
	}
	installed, err := Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	directory := runtimeDirectory(store)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string][]byte{"package.json": runtimePackageJSON, "package-lock.json": runtimePackageLock, "driver.mjs": runtimeDriver} {
		if err := os.WriteFile(filepath.Join(directory, name), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"node_modules", "browsers"} {
		if err := os.Symlink(filepath.Join(runtimeDirectory(installed), name), filepath.Join(directory, name)); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func sidecarDescendants(t *testing.T, pid int) []int {
	t.Helper()
	data, err := exec.Command("ps", "-axo", "pid=,ppid=").Output()
	if err != nil {
		t.Fatal(err)
	}
	children := map[int][]int{}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		child, _ := strconv.Atoi(parts[0])
		parent, _ := strconv.Atoi(parts[1])
		children[parent] = append(children[parent], child)
	}
	result := []int{pid}
	for n := 0; n < len(result); n++ {
		result = append(result, children[result[n]]...)
	}
	return result
}

func waitForBrowserProcessExit(t *testing.T, pids []int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var live []int
		for _, pid := range pids {
			if syscall.Kill(pid, 0) == nil {
				live = append(live, pid)
			}
		}
		if len(live) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("owned browser processes survived shutdown: %v", live)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestSidecarStalledPageRecoveryIntegration(t *testing.T) {
	for _, stop := range []bool{false, true} {
		name := "cancel"
		if stop {
			name = "stop"
		}
		t.Run(name, func(t *testing.T) {
			store := integrationRecoveryStore(t)
			admitted := make(chan struct{})
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if hits.Add(1) == 1 {
					close(admitted)
				}
				<-r.Context().Done()
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			d, err := StartSidecar(ctx, SidecarConfig{Store: store, Mode: ModeManaged})
			if err != nil {
				t.Fatal(err)
			}
			driver := d.(*sidecar)
			defer driver.Close()
			pids := sidecarDescendants(t, driver.process.Process.Pid)
			if len(pids) < 2 {
				t.Fatal("no native browser child observed")
			}
			session, err := store.Start(StartOptions{})
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
			done := make(chan error, 1)
			go func() { done <- service.Run(ctx) }()
			waitForBrowserSocket(t, socketPath(store, session.ID))
			client, err := NewClient(store, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			tabs, err := client.CandidateTabs(ctx)
			if err != nil || len(tabs) != 1 {
				t.Fatalf("tabs: %#v, %v", tabs, err)
			}
			if _, err = client.SelectTabs(ctx, tabs); err != nil {
				t.Fatal(err)
			}
			if _, err = client.AddOrigin(ctx, server.URL); err != nil {
				t.Fatal(err)
			}
			requestCtx, interrupt := context.WithCancel(ctx)
			defer interrupt()
			result := make(chan error, 1)
			go func() { _, err := client.Navigate(requestCtx, session.ID, tabs[0].ID, server.URL); result <- err }()
			select {
			case <-admitted:
			case <-time.After(5 * time.Second):
				t.Fatal("navigation did not reach stalled server")
			}
			if stop {
				stopCtx, stopCancel := context.WithTimeout(ctx, time.Second)
				defer stopCancel()
				if _, err := client.Stop(stopCtx); err != nil {
					t.Fatal(err)
				}
			} else {
				interrupt()
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("controller did not recover")
			}
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("stalled request succeeded")
				}
			case <-time.After(time.Second):
				t.Fatal("request did not return")
			}
			waitForBrowserProcessExit(t, pids)
			if hits.Load() != 1 {
				t.Fatalf("interrupted navigation replayed %d times", hits.Load())
			}
			recorded, err := store.Get(session.ID)
			if err != nil || recorded.State != StateStopped {
				t.Fatalf("revocation: %#v, %v", recorded, err)
			}
			if _, err := os.Stat(tokenPath(store, session.ID)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("token retained: %v", err)
			}
			fresh, err := StartSidecar(ctx, SidecarConfig{Store: store, Mode: ModeManaged})
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Close()
			freshPIDs := sidecarDescendants(t, fresh.(*sidecar).process.Process.Pid)
			tabs, err = fresh.CandidateTabs(ctx)
			if err != nil || len(tabs) != 1 || tabs[0].URL != "about:blank" {
				t.Fatalf("fresh browser: %#v, %v", tabs, err)
			}
			_ = fresh.Close()
			waitForBrowserProcessExit(t, freshPIDs)
		})
	}
}
