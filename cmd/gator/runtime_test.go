package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimePrivateRolesRequireHostMarkersBeforeOpeningProfile(t *testing.T) {
	for _, key := range []string{"GATOR_INTERNAL_COMMANDS", "GATOR_UI_BACKEND", "GATOR_DESKTOP_UPDATE_GUARD", "GATOR_COMPANION_CREATE"} {
		t.Setenv(key, "")
	}
	root := filepath.Join(t.TempDir(), "must-not-be-created")
	t.Setenv("GATOR_STATE_DIR", root)
	for _, args := range [][]string{{"ui-backend", "", ""}, {"desktop-update-guard"}, {"companion-create-chat", "stable-new-chat-request"}} {
		if err := runRuntime(args, io.Discard); err == nil {
			t.Fatalf("unmarked private role %v was accepted", args)
		}
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("private roles touched the profile: %v", err)
	}
}

func TestRuntimeHelpAndVersionWithoutOpeningProfile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "must-not-be-created")
	t.Setenv("GATOR_STATE_DIR", root)
	for _, arg := range []string{"--help", "-h", "--version", "-v"} {
		var out bytes.Buffer
		if err := runRuntime([]string{arg}, &out); err != nil || !strings.Contains(strings.ToLower(out.String()), "gator") || strings.Contains(out.String(), "desktop") {
			t.Fatalf("%s: %q (%v)", arg, out.String(), err)
		}
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("help or version touched the profile: %v", err)
	}
}

func TestRuntimePublicCommandsDoNotRequireDeveloperOptIn(t *testing.T) {
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_INTERNAL_COMMANDS", "")
	var out bytes.Buffer
	if err := runRuntime([]string{"note", "add", "--title", "Fixture", "--body", "Internal test data"}, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runRuntime([]string{"note", "list"}, &out); err != nil || !strings.Contains(out.String(), "Fixture") {
		t.Fatalf("internal note service: %q (%v)", out.String(), err)
	}
	t.Setenv("GATOR_INTERNAL_COMMANDS", "")
	// This role is used by app-owned background services, including old service
	// definitions. An invalid flag proves routing without starting a supervisor.
	if err := runRuntime([]string{"job", "supervisor", "--not-a-real-flag"}, io.Discard); err == nil || strings.Contains(err.Error(), "CLI is retired") {
		t.Fatalf("background service routing: %v", err)
	}
	if err := workInteractive(); err == nil || !strings.Contains(err.Error(), "interactive mode needs a terminal") {
		t.Fatalf("non-terminal launch: %v", err)
	}
}
