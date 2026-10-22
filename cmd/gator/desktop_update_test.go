package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/state"
)

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
