package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopRuntimeRejectsStandaloneCommandsBeforeOpeningProfile(t *testing.T) {
	t.Setenv("GATOR_INTERNAL_COMMANDS", "")
	t.Setenv("GATOR_UI_BACKEND", "")
	root := filepath.Join(t.TempDir(), "must-not-be-created")
	t.Setenv("GATOR_STATE_DIR", root)
	for _, args := range [][]string{nil, {"work", "write a report"}, {"note", "list"}, {"config"}, {"doctor"}, {"ui-backend", "", ""}} {
		if err := runDesktopRuntime(args, io.Discard); err == nil || !strings.Contains(err.Error(), "standalone Gator CLI is retired") {
			t.Fatalf("standalone %v: %v", args, err)
		}
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("retired commands touched the profile: %v", err)
	}
}

func TestDesktopRuntimeHelpAndVersionDescribeTheAppWithoutOpeningProfile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "must-not-be-created")
	t.Setenv("GATOR_STATE_DIR", root)
	for _, arg := range []string{"--help", "-h", "--version", "-v"} {
		var out bytes.Buffer
		if err := runDesktopRuntime([]string{arg}, &out); err != nil || !strings.Contains(out.String(), "desktop") || strings.Contains(out.String(), "gator work") {
			t.Fatalf("%s: %q (%v)", arg, out.String(), err)
		}
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("help or version touched the profile: %v", err)
	}
}

func TestDesktopRuntimeRetainsExplicitInternalServices(t *testing.T) {
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_INTERNAL_COMMANDS", "1")
	var out bytes.Buffer
	if err := runDesktopRuntime([]string{"note", "add", "--title", "Fixture", "--body", "Internal test data"}, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runDesktopRuntime([]string{"note", "list"}, &out); err != nil || !strings.Contains(out.String(), "Fixture") {
		t.Fatalf("internal note service: %q (%v)", out.String(), err)
	}
	t.Setenv("GATOR_INTERNAL_COMMANDS", "")
	// This role is used by app-owned background services, including old service
	// definitions. An invalid flag proves routing without starting a supervisor.
	if err := runDesktopRuntime([]string{"job", "supervisor", "--not-a-real-flag"}, io.Discard); err == nil || strings.Contains(err.Error(), "CLI is retired") {
		t.Fatalf("background service routing: %v", err)
	}
	if err := workInteractive(); err == nil || !strings.Contains(err.Error(), "CLI is retired") {
		t.Fatalf("terminal renderer was still launchable: %v", err)
	}
}
