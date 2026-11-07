package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gongahkia/gator/internal/lifecycle"
	"github.com/gongahkia/gator/internal/state"
)

// runRuntime routes public commands and authenticated application services.
func runRuntime(args []string, out io.Writer) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, err := fmt.Fprintln(out, usage)
		return err
	}
	if len(args) == 1 && (args[0] == "--version" || args[0] == "-v") {
		_, err := fmt.Fprintf(out, "gator %s (%s, %s)\n", version, commit, date)
		return err
	}
	if len(args) == 0 {
		return launchTerminal(nil)
	}
	if len(args) == 1 && args[0] == "desktop-speech" && os.Getenv("GATOR_DESKTOP_SPEECH") == "1" {
		if err := preparePlatformProfile(); err != nil {
			return err
		}
		return runDesktopSpeech(os.Stdin, out)
	}
	if len(args) == 1 && args[0] == "desktop-update-guard" && os.Getenv("GATOR_DESKTOP_UPDATE_GUARD") == "1" {
		return holdDesktopUpdateGuard(os.Stdin, out)
	}
	if len(args) == 4 && args[0] == "companion-catalog" && os.Getenv("GATOR_COMPANION_CATALOG") == "1" {
		return companionCatalog(args[1], args[2], args[3], out)
	}
	if len(args) >= 2 && args[0] == "external-tmux" && os.Getenv("GATOR_INTERNAL_COMMANDS") == "1" {
		return companionExternal(args[1:], true, os.Stdin, out)
	}
	if len(args) == 2 && args[0] == "companion-external-control" && os.Getenv("GATOR_COMPANION_EXTERNAL_CONTROL") == "1" {
		return companionExternalControl(args[1], os.Stdin, out)
	}
	if len(args) == 5 && args[0] == "companion-external" && os.Getenv("GATOR_COMPANION_EXTERNAL") == "1" {
		return companionExternal(args[1:], false, os.Stdin, out)
	}
	if len(args) == 4 && args[0] == "companion-inspect" && os.Getenv("GATOR_COMPANION_INSPECT") == "1" {
		return companionInspection(args[1:], false, out)
	}
	if len(args) == 2 && args[0] == "companion-control" && os.Getenv("GATOR_COMPANION_CONTROL") == "1" && (args[1] == "check" || args[1] == "dispatch" || args[1] == "receipt") {
		return companionControl(args[1], os.Stdin, out)
	}
	if len(args) == 2 && args[0] == "control-access" && os.Getenv("GATOR_INTERNAL_COMMANDS") == "1" {
		return companionControlGrant(args[1], os.Stdin, out)
	}
	if len(args) == 4 && args[0] == "inspection-access" && os.Getenv("GATOR_INTERNAL_COMMANDS") == "1" {
		return companionInspection(args[1:], true, out)
	}
	if len(args) == 2 && args[0] == "companion-create-chat" && os.Getenv("GATOR_COMPANION_CREATE") == "1" {
		return companionCreateChat(args[1], out)
	}
	// Private transport roles require their explicit host marker. Public CLI
	// commands do not require a developer opt-in.
	if args[0] == "ui-backend" && (len(args) != 3 || os.Getenv("GATOR_UI_BACKEND") != "1") {
		return errors.New("ui-backend is an internal Gator transport")
	}
	for _, role := range []string{"desktop-speech", "desktop-update-guard", "companion-catalog", "companion-external-control", "companion-external", "companion-inspect", "companion-control", "companion-create-chat", "control-access", "inspection-access", "external-tmux"} {
		if args[0] == role {
			return errors.New("this application service requires its host marker")
		}
	}
	supervisor := len(args) >= 2 && args[0] == "job" && args[1] == "supervisor"
	// A login supervisor selects its profile before opening configuration or
	// maintenance. Its own workers use the same private command routing.
	if supervisor {
		return run(args, out)
	}
	if err := preparePlatformProfile(); err != nil {
		return err
	}
	if err := maintainRuntimeProfile(); err != nil {
		return err
	}
	return run(args, out)
}

func maintainRuntimeProfile() error {
	if root, err := state.ResolveDir(""); err == nil {
		if _, err := lifecycle.Maintain(context.Background(), root, time.Now()); err != nil && !errors.Is(err, state.ErrBusy) {
			if errors.Is(err, lifecycle.ErrRecovery) {
				return err
			}
			fmt.Fprintln(os.Stderr, "Gator: retention cleanup deferred:", err)
		}
	}
	return nil
}
