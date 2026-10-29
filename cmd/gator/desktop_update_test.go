package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/state"
)

func TestDesktopUpdateGuardReportsResolvedProfileLocations(t *testing.T) {
	root, configuration := t.TempDir(), t.TempDir()
	t.Setenv("GATOR_STATE_DIR", root)
	t.Setenv("GATOR_CONFIG_DIR", configuration)
	var out bytes.Buffer
	if err := holdDesktopUpdateGuard(strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Ready        bool     `json:"ready"`
		ProfilePaths []string `json:"profile_paths"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Ready || len(result.ProfilePaths) != 2 || result.ProfilePaths[0] != filepath.Join(root, "gator") || result.ProfilePaths[1] != filepath.Join(configuration, "gator") {
		t.Fatalf("wrong protected locations: %+v", result)
	}
}

func TestDesktopUpdateGuardRefusesActiveWork(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", root)
	release, err := state.Guard(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	var out bytes.Buffer
	if err := holdDesktopUpdateGuard(strings.NewReader(""), &out); err == nil || out.Len() != 0 {
		t.Fatalf("active work was accepted: %s, %v", out.String(), err)
	}
}

func TestDesktopUpdateGuardRefusesIdleSupervisorAndInvalidState(t *testing.T) {
	for _, payload := range []string{fmt.Sprint(os.Getpid()), "invalid"} {
		t.Run(payload, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("GATOR_STATE_DIR", root)
			directory := filepath.Join(root, "gator", "jobs")
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "supervisor.lock"), []byte(payload), 0600); err != nil {
				t.Fatal(err)
			}
			if err := holdDesktopUpdateGuard(strings.NewReader(""), io.Discard); err == nil {
				t.Fatal("supervisor accepted")
			}
		})
	}
}

type updateReadyWriter struct{ ready chan struct{} }

func (w updateReadyWriter) Write(p []byte) (int, error) { close(w.ready); return len(p), nil }

func TestDesktopUpdateGuardHoldsLeaseUntilHostReleasesIt(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", root)
	in, host := io.Pipe()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- holdDesktopUpdateGuard(in, updateReadyWriter{ready}) }()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("guard did not become ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("guard readiness timed out")
	}
	if release, err := state.Guard(root, true); err == nil {
		release()
		t.Error("lease was not held")
	}
	_ = host.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	release, err := state.Guard(root, true)
	if err != nil {
		t.Fatal(err)
	}
	release()
}
