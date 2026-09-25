package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
)

// These values are set for release builds with -ldflags. Development builds
// remain explicit rather than pretending to have a published version.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const usage = `Gator — terminal-native Work

Start Work:
  gator
  gator "research X and produce a report"
  gator work [OPTIONS] TASK
  gator work code [OPTIONS] TASK

Product areas:
  Work        gator work [TASK] · gator work resume CONVERSATION TASK
  History     gator work list · gator work history CONVERSATION · gator work show ID
  Learnings   gator learnings list [--status STATUS] [--scope SCOPE]
              gator learnings add|edit|approve|reject|enable|disable ...
  Jobs        gator job add|list|show|edit|enable|disable|run|history|remove
  Settings    gator config [show] · gator config set default-provider|default-model|sandbox|network VALUE

Common follow-up actions:
  gator work review WORK_ID
  gator work apply WORK_ID [OPTIONS]
  gator work retry WORK_ID [OPTIONS]
  gator work feedback WORK_ID accept|reject|correct|remember|dont-learn|list

Advanced and integration families remain available when needed:
  provider, config hook, agent, browser, desktop, doctor, extension, lsp,
  mcp, theme, update, work eval, and work snapshot.
  Run an advanced family with --help or consult its command-specific output.

For a free-form Work task that begins with a nested command name, use
gator work -- TASK to keep it a task rather than dispatching that command.`

const codeUsage = `Gator coding — one Gator manager with an internal Code specialist

Usage:
  gator work code [CODE POLICY] TASK

This command uses the main Gator orchestration path and requires retained Code
patch evidence. It never opens a standalone Code session or TUI.

Code policy:
  --code-max-steps N
  --verify 'argv ...'
  --scope PATH
  --profile NAME
  --setup 'argv ...'
  --allow-command 'argv ...'
  --allow-command-prefix 'argv ...'
  --sandbox strict|off
  --network deny|allow
  --code-capability lsp|mcp|extension|http|browser|terminal
  --code-browser-session ID
  --image PATH
  --attach PATH

Defaults are a strict sandbox, denied process network, no integration grants,
and git diff --check verification. Use gator work resume and Work revision history
for continuation and branching.`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "gator:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if !supportedPlatform(runtime.GOOS) {
		return fmt.Errorf("Gator supports only Linux and macOS; %s is not supported", runtime.GOOS)
	}
	if len(args) == 0 {
		return workInteractive()
	}
	if args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(out, usage)
		return err
	}
	if len(args) == 2 && args[0] == "--mode" && args[1] == "rpc" {
		return rpcMode(nil, os.Stdin, out)
	}
	if len(args) == 2 && args[0] == "--mode" && args[1] == "acp" {
		return acpMode(nil, os.Stdin, out)
	}
	if args[0] == "--version" || args[0] == "-v" {
		_, err := fmt.Fprintf(out, "gator %s (%s, %s)\n", version, commit, date)
		return err
	}

	switch args[0] {
	case "config", "-c":
		return configCommand(args[1:], out)
	case "agent", "-a":
		return agentFamilyCommand(args[1:], out)
	case "update", "-u":
		return update(args[1:], out)
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
	case "work-rpc":
		return workHeadless(context.Background(), os.Stdin, out)
	case "work", "-w":
		return workCommand(args[1:], out)
	case "rpc", "serve", "acp", "child", "delegate", "hook", "connector", "inbox", "snapshot", "inspect", "resume", "eval", "review", "export", "apply":
		return movedCommand(args[0])
	default:
		if isQuotedWorkRequest(args) {
			return workTask(args, out)
		}
		return fmt.Errorf("unknown command %q; run 'gator --help'", args[0])
	}
}

// isQuotedWorkRequest preserves useful unknown-command errors while making
// the documented, shell-quoted one-line Work request a direct entry point.
func isQuotedWorkRequest(args []string) bool {
	return len(args) == 1 && strings.ContainsAny(strings.TrimSpace(args[0]), " \t\r\n")
}

func supportedPlatform(goos string) bool {
	return goos == "linux" || goos == "darwin"
}

func isHelpArgument(value string) bool {
	return value == "--help" || value == "-h"
}
