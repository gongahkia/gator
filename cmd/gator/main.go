package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/lifecycle"
	"github.com/gongahkia/gator/internal/state"
)

// These values are set for release builds with -ldflags. Development builds
// remain explicit rather than pretending to have a published version.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const usage = `Gator — terminal-native personal agent

Start:
  gator
  gator goal add "Plan the offsite"
  gator work --goal GOAL_ID "What should I do next?"
  gator "research X and produce a report"
  gator work [OPTIONS] TASK

Inside Gator:
  /followup new                 review a project check before saving
  /memory                       set up recall and inspect data destinations
  /context                      review sources and saved context
  /recovery                     reconnect, retry, and review offline coverage
  /usage                        review shared usage and limits
  /data                         manage retention, backups, and deletion

Product areas:
  Work        gator work [TASK] · gator work resume CONVERSATION TASK
  History     gator work list · gator work history CONVERSATION · gator work show ID
  Activity    gator work activity [--json]
  Goals       gator goal add|chat|list|show|context|pause|resume|complete
              gator goal schedule|watch|schedules|unschedule
  Memory      gator memory add|list|search|show|correct|forget
  Notes       gator note add|list|search|show|edit|remove
  Skills      gator skill create|list|show|edit|enable|disable|remove
  Learnings   gator learnings list [--status STATUS] [--scope SCOPE]
              gator learnings add|edit|approve|reject|enable|disable ...
  Jobs        gator job add|list|show|edit|enable|disable|run|history|remove
              gator job inbox show|read|rate|report
              gator job quiet-audit JOB_ID [ATTEMPT_ID missed|correct|clear [--note TEXT]]
  Settings    gator config [show] · gator config set default-provider|default-model|sandbox|network VALUE
  Usage       gator usage status|limits|pause|resume
  Data        gator data status|retention|cleanup|preview|apply|export|verify

Common follow-up actions:
  gator work review WORK_ID
  gator work reconcile WORK_ID CORRELATION_ID
  gator work decisions [RUN_ID] [--json]
  gator work answer RUN_ID DECISION_ID
  gator work steer RUN_ID TEXT · gator work stop RUN_ID
  gator work cancel-task RUN_ID TASK_ID
  gator work steer-task RUN_ID TASK_ID TEXT
  gator work apply WORK_ID [OPTIONS]
  gator work retry WORK_ID [OPTIONS]
  gator work feedback WORK_ID accept|reject|correct|remember|dont-learn|list

Advanced and integration families remain available when needed:
  provider, config hook, agent, browser, desktop, doctor, extension, lsp,
  mcp, pilot guide|record|report, theme, update, work eval, and work snapshot.
  Run an advanced family with --help or consult its command-specific output.

For a free-form Work task that begins with a nested command name, use
gator work -- TASK to keep it a task rather than dispatching that command.`

func main() {
	if len(os.Args) == 1 || (os.Args[1] != "--help" && os.Args[1] != "-h" && os.Args[1] != "--version" && os.Args[1] != "-v") {
		if root, err := state.ResolveDir(""); err == nil {
			if _, err := lifecycle.Maintain(context.Background(), root, time.Now()); err != nil && !errors.Is(err, state.ErrBusy) {
				fmt.Fprintln(os.Stderr, "gator: retention cleanup deferred:", err)
			}
		}
	}
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
	case "data":
		return dataCommand(args[1:], out)
	case "usage":
		return usageCommand(args[1:], out)
	case "config", "-c":
		return configCommand(args[1:], out)
	case "agent", "-a":
		return agentFamilyCommand(args[1:], out)
	case "update", "-u":
		return update(args[1:], out)
	case "doctor", "-d":
		return launchDoctorUI(args[1:], out)
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
	case "ui-doctor-backend":
		if os.Getenv("GATOR_UI_DOCTOR_BACKEND") != "1" {
			return errors.New("ui-doctor-backend is an internal Gator transport")
		}
		if len(args) > 1 && args[1] == "--json" {
			result, err := doctorResult(args[2:])
			if err != nil {
				return err
			}
			settings, err := loadSettings()
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(struct {
				Theme  string `json:"theme"`
				Result any    `json:"result"`
			}{Theme: settings.Theme, Result: result})
		}
		return doctor(args[1:], out)
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
		case "feedback", "snapshot", "apply", "reconcile", "review", "answer", "steer", "stop", "cancel-task", "steer-task":
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
	return goos == "linux" || goos == "darwin"
}

func isHelpArgument(value string) bool {
	return value == "--help" || value == "-h"
}
