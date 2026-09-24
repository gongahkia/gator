package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/gongahkia/gator/internal/desktop"
	"github.com/gongahkia/gator/internal/journal"
)

const desktopUsage = `usage:
  gator desktop start --app BUNDLE_ID [--app BUNDLE_ID...] [--retain-provider-state]
  gator desktop status
  gator desktop stop SESSION_ID`

func desktopCommand(arguments []string, out io.Writer) error {
	if runtime.GOOS != "darwin" {
		return errors.New("desktop control is currently available only on macOS")
	}
	if len(arguments) == 0 {
		return errors.New(desktopUsage)
	}
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	store, err := desktop.Open(stateDir)
	if err != nil {
		return err
	}
	switch arguments[0] {
	case "start":
		return startDesktopSession(arguments[1:], out, store)
	case "status":
		if len(arguments) != 1 {
			return errors.New("usage: gator desktop status")
		}
		sessions, err := store.List()
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			_, err := fmt.Fprintln(out, "No local desktop sessions.")
			return err
		}
		for _, session := range sessions {
			apps := make([]string, 0, len(session.Apps))
			for _, app := range session.Apps {
				apps = append(apps, app.BundleID)
			}
			if _, err := fmt.Fprintf(out, "%s  %s  apps: %s  provider-state: %t\n", session.ID, session.State, strings.Join(apps, ", "), session.RetainProviderState); err != nil {
				return err
			}
		}
		return nil
	case "stop":
		if len(arguments) != 2 {
			return errors.New("usage: gator desktop stop SESSION_ID")
		}
		session, err := store.Stop(arguments[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Stopped desktop session %s\n", session.ID)
		return err
	default:
		return errors.New(desktopUsage)
	}
}

func startDesktopSession(arguments []string, out io.Writer, store *desktop.Store) error {
	flags := flag.NewFlagSet("desktop start", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var appIDs stringFlags
	flags.Var(&appIDs, "app", "macOS application bundle identifier to approve (repeatable)")
	retain := flags.Bool("retain-provider-state", false, "consent to OpenAI receiving approved-window screenshots and retaining this session's response-chain state")
	if err := flags.Parse(arguments); err != nil || len(flags.Args()) != 0 || len(appIDs) == 0 {
		return errors.New("usage: gator desktop start --app BUNDLE_ID [--app BUNDLE_ID...] [--retain-provider-state]")
	}
	runtime := desktop.DefaultRuntime()
	trusted, err := runtime.AccessibilityTrusted()
	if err != nil {
		return err
	}
	if !trusted {
		return errors.New("macOS Accessibility permission is required; grant Gator access in Privacy & Security before starting a desktop session")
	}
	apps := make([]desktop.Application, 0, len(appIDs))
	for _, bundleID := range appIDs {
		apps = append(apps, desktop.Application{BundleID: bundleID})
	}
	session, err := store.Start(desktop.StartOptions{Apps: apps, RetainProviderState: *retain})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Started local desktop session %s\n  approved apps: %s\n  CUA provider-state retention: %t\nWith retention enabled, OpenAI may receive screenshots of the approved foreground window to continue this local desktop run; Gator never stores those screenshot bytes in Work replay. Every click, key press, typed value, and app activation still needs a fresh approval. Gator blocks system settings, terminal, Keychain, Finder, clipboard shortcuts, and secure text fields.\n", session.ID, strings.Join(appIDs, ", "), session.RetainProviderState)
	return err
}
