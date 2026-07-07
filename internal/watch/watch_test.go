package watch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWatcherRunsMarkerAndRemovesIt(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	runs := make(chan Marker, 1)
	errs := make(chan error, 1)
	watcher := &Watcher{
		Root:     root,
		Debounce: 20 * time.Millisecond,
		Ready:    ready,
		Runner: func(_ context.Context, marker Marker) error {
			runs <- marker
			return nil
		},
	}
	go func() {
		errs <- watcher.Run(ctx)
	}()
	<-ready
	if err := os.WriteFile(path, []byte("package main\n// ai: add main\n"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	select {
	case marker := <-runs:
		if !strings.Contains(marker.Instruction, "add main") || !strings.Contains(marker.Instruction, "main.go:2") {
			t.Fatalf("marker = %#v", marker)
		}
	case err := <-errs:
		t.Fatalf("watcher error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for marker run")
	}
	eventually(t, func() bool {
		data, err := os.ReadFile(path)
		return err == nil && !strings.Contains(string(data), "ai:")
	})
	cancel()
	if err := <-errs; err != nil {
		t.Fatalf("watcher stop: %v", err)
	}
}

func TestWatcherHonorsGitIgnore(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored/\n"), 0o644); err != nil {
		t.Fatalf("write gitignore: %v", err)
	}
	ignoredDir := filepath.Join(root, "ignored")
	if err := os.Mkdir(ignoredDir, 0o755); err != nil {
		t.Fatalf("mkdir ignored: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	runs := make(chan Marker, 1)
	errs := make(chan error, 1)
	watcher := &Watcher{
		Root:     root,
		Debounce: 20 * time.Millisecond,
		Ready:    ready,
		Runner: func(_ context.Context, marker Marker) error {
			runs <- marker
			return nil
		},
	}
	go func() {
		errs <- watcher.Run(ctx)
	}()
	<-ready
	if err := os.WriteFile(filepath.Join(ignoredDir, "main.go"), []byte("// ai: ignored\n"), 0o644); err != nil {
		t.Fatalf("write ignored marker: %v", err)
	}
	select {
	case marker := <-runs:
		t.Fatalf("ignored marker ran: %#v", marker)
	case err := <-errs:
		t.Fatalf("watcher error: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	cancel()
	if err := <-errs; err != nil {
		t.Fatalf("watcher stop: %v", err)
	}
}

func TestIgnoreMatcherNegation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.go\n!keep.go\n"), 0o644); err != nil {
		t.Fatalf("write gitignore: %v", err)
	}
	matcher, err := LoadIgnore(root)
	if err != nil {
		t.Fatalf("load ignore: %v", err)
	}
	if !matcher.Ignored(filepath.Join(root, "drop.go"), false) {
		t.Fatal("drop.go not ignored")
	}
	if matcher.Ignored(filepath.Join(root, "keep.go"), false) {
		t.Fatal("keep.go ignored")
	}
}

func eventually(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met")
}
