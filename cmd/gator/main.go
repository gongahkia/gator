package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/gongahkia/gator/internal/state"
)

// These values are set for release builds with -ldflags. Development builds
// remain explicit rather than pretending to have a published version.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const usage = `Gator desktop runtime

Open the Gator desktop application to use Gator.
This bundled process is an internal app component, not a standalone CLI.
Development setup: npm ci --prefix desktop && make desktop`

func main() {
	if err := runDesktopRuntime(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "Gator:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if !supportedPlatform(runtime.GOOS) {
		return fmt.Errorf("Gator supports Linux, macOS, and Windows; %s is not supported", runtime.GOOS)
	}
	if len(args) == 0 {
		return errors.New(desktopOnlyMessage)
	}
	if args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(out, usage)
		return err
	}
	if len(args) == 2 && args[0] == "--mode" && args[1] == "acp" {
		return errors.New("ACP Code mode is retired; use Gator conversations or work-rpc for automation")
	}
	if args[0] == "--version" || args[0] == "-v" {
		_, err := fmt.Fprintf(out, "gator %s (%s, %s)\n", version, commit, date)
		return err
	}

	if commandUsesPrivateData(args) {
		root, err := state.ResolveDir("")
		if err != nil {
			return err
		}
		release, err := state.Guard(root, false)
		if err != nil {
			return err
		}
		defer release()
	}
	switch args[0] {
	case "overview":
		return overviewCommand(args[1:], out)
	case "data":
		return dataCommand(args[1:], out)
	case "usage":
		return usageCommand(args[1:], out)
	case "config", "-c":
		return configCommand(args[1:], out)
	case "agent", "-a":
		return agentFamilyCommand(args[1:], out)
	case "update", "-u":
		return errors.New("Gator desktop updates are installed through the desktop application distribution; the CLI updater is retired")
	case "doctor", "-d":
		return doctor(args[1:], out)
	case "extension", "-e":
		return extensionCommand(args[1:], out)
	case "lsp", "-l":
		return lspCommand(args[1:], out)
	case "mcp", "-m":
		return mcpCommand(args[1:], out)
	case "job", "-j":
		return jobCommand(args[1:], out)
	case "provider", "-p":
		return providerCommand(args[1:], out)
	case "browser", "-b":
		return browserCommand(args[1:], out)
	case "desktop":
		return desktopCommand(args[1:], out)
	case "theme", "-t":
		return themeCommand(args[1:], out)
	case "learnings":
		return learningCommand(args[1:], out)
	case "goal", "goals":
		return goalCommand(args[1:], out)
	case "memory":
		return memoryCommand(args[1:], out)
	case "pilot":
		return pilotCommand(args[1:], out)
	case "note", "notes":
		return noteCommand(args[1:], out)
	case "skill", "skills":
		return skillCommand(args[1:], out)
	case "suggestion", "suggestions":
		return suggestionCommand(args[1:], out)
	case "work-rpc":
		return workHeadless(context.Background(), os.Stdin, out)
	case "ui-backend":
		if os.Getenv("GATOR_UI_BACKEND") != "1" || len(args) != 3 {
			return errors.New("ui-backend is an internal Gator transport")
		}
		return workInteractiveConversationForGoal(args[1], args[2])
	case "work", "-w":
		return workCommand(args[1:], out)
	case "acp":
		return errors.New("ACP Code mode is retired; use Gator conversations or work-rpc for automation")
	case "hook", "connector", "inbox", "snapshot", "inspect", "resume", "eval", "review", "export", "apply":
		return movedCommand(args[0])
	default:
		if isQuotedWorkRequest(args) {
			return workTask(args, out)
		}
		return fmt.Errorf("unknown command %q; run 'gator --help'", args[0])
	}
}

func commandUsesPrivateData(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "memory", "note", "notes", "skill", "skills", "learnings", "suggestion", "suggestions":
		return true
	case "goal", "goals":
		return len(args) < 2 || args[1] != "chat"
	case "job", "-j":
		return len(args) < 2 || args[1] != "supervisor"
	case "work":
		if len(args) < 2 {
			return false
		}
		switch args[1] {
		case "feedback", "snapshot", "apply", "reconcile", "review", "answer", "steer", "stop", "cancel-task", "steer-task", "answer-task":
			return true
		}
	}
	return false
}

// isQuotedWorkRequest preserves useful unknown-command errors while making
// the documented, shell-quoted one-line Work request a direct entry point.
func isQuotedWorkRequest(args []string) bool {
	return len(args) == 1 && strings.ContainsAny(strings.TrimSpace(args[0]), " \t\r\n")
}

func supportedPlatform(goos string) bool {
	return goos == "linux" || goos == "darwin" || goos == "windows"
}

func isHelpArgument(value string) bool {
	return value == "--help" || value == "-h"
}
