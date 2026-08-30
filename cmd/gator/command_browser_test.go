package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	gatorbrowser "github.com/gongahkia/gator/internal/browser"
	"github.com/gongahkia/gator/internal/journal"
)

func TestBrowserStopRevokesUnavailableSession(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	resolvedStateDir, err := journal.ResolveStateDir(stateDir)
	if err != nil {
		t.Fatalf("ResolveStateDir: %v", err)
	}
	store, err := gatorbrowser.Open(resolvedStateDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	session, err := store.Start(gatorbrowser.StartOptions{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, _, err := gatorbrowser.CreateToken(store, session.ID); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	var output bytes.Buffer
	if err := browserCommand([]string{"stop", session.ID}, &output); err != nil {
		t.Fatalf("browser stop: %v", err)
	}
	stopped, err := store.Get(session.ID)
	if err != nil || stopped.State != gatorbrowser.StateStopped {
		t.Fatalf("stopped session = %#v, %v", stopped, err)
	}
	if _, err := os.Stat(gatorbrowser.TokenPath(store, session.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unavailable session token remained: %v", err)
	}
	if !strings.Contains(output.String(), "Revoked unavailable browser session") {
		t.Fatalf("browser stop output = %q", output.String())
	}
}
