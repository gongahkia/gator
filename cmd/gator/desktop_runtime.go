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

const desktopOnlyMessage = "the standalone Gator CLI is retired; open the Gator desktop application (development: make desktop)"

// runDesktopRuntime is the executable boundary. The command handlers in this
// package remain shared application services and an explicit developer fixture
// harness; they are not a second user interface. The environment opt-in is a
// routing convention, not an authorization boundary or a replacement for reviews.
func runDesktopRuntime(args []string, out io.Writer) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, err := fmt.Fprintln(out, usage)
		return err
	}
	if len(args) == 1 && (args[0] == "--version" || args[0] == "-v") {
		_, err := fmt.Fprintf(out, "Gator desktop runtime %s (%s, %s)\n", version, commit, date)
		return err
	}
	if len(args) == 0 {
		return errors.New(desktopOnlyMessage)
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
	desktop := args[0] == "ui-backend" && len(args) == 3 && os.Getenv("GATOR_UI_BACKEND") == "1"
	supervisor := len(args) >= 2 && args[0] == "job" && args[1] == "supervisor"
	internal := os.Getenv("GATOR_INTERNAL_COMMANDS") == "1"
	if !desktop && !supervisor && !internal {
		return errors.New(desktopOnlyMessage)
	}
	if desktop || supervisor {
		previous, present := os.LookupEnv("GATOR_INTERNAL_COMMANDS")
		if err := os.Setenv("GATOR_INTERNAL_COMMANDS", "1"); err != nil {
			return err
		}
		defer func() {
			if present {
				_ = os.Setenv("GATOR_INTERNAL_COMMANDS", previous)
			} else {
				_ = os.Unsetenv("GATOR_INTERNAL_COMMANDS")
			}
		}()
	}
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
