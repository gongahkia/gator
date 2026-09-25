package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
)

// These values are set for release builds with -ldflags. Development builds
// remain explicit rather than pretending to have a published version.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const usage = `Gator — terminal-native, inspectable work agent

Usage:
  gator
  gator --help | -h
  gator --version | -v
  gator config [show]
  gator -c [show]
  gator config hook status|trust|untrust
  gator config set default-provider PROVIDER
  gator config set default-model MODEL
  gator config set sandbox strict|off
  gator config set network deny|allow
  gator config set snapshot-max-files N
  gator config set snapshot-max-total-mib N
  gator config set snapshot-max-file-mib N
  gator config set snapshot-exclude PATTERN
  gator config set desktop-notifications on|off
  gator config set job-timezone IANA_TIMEZONE
  gator config set job-missed skip|run_once
  gator agent list
  gator -a list
  gator agent child list|show|batches|batch RUN_RECORD_PATH [CHILD_RUN_ID|BATCH_ID]
  gator agent delegate RUNTIME ACTION [OPTIONS]
  gator agent acp [--verify 'argv ...']
  gator agent rpc
  gator agent serve token ABSOLUTE_PATH
  gator agent serve start --token-file ABSOLUTE_PATH [--listen 127.0.0.1:PORT]
  gator agent serve status --token-file ABSOLUTE_PATH
  gator agent serve stop --token-file ABSOLUTE_PATH
  gator agent serve --token-file ABSOLUTE_PATH [--listen 127.0.0.1:PORT]
  gator lsp status|trust|untrust
  gator -l status|trust|untrust
  gator mcp status|trust|untrust|login|logout
  gator -m status|trust|untrust|login|logout
  gator extension list|status|install|enable|disable|remove|trust|untrust
  gator -e ...
  gator theme list|set
  gator -t list|set
  gator learnings list|show|add|enable|disable|edit|remove|reject
  gator provider connector list
  gator provider connector add ID --kind json|webhook|slack|google|atlassian|notion|mcp [--url URL] [--name NAME] [--auth none|bearer|oauth]
  gator provider connector status|login|logout|test|remove ID [OPTIONS]
  gator provider connector permission ID OPERATION read|write allow|ask|deny|draft
  gator job add|list|show|edit|enable|disable|run|history|remove|launchagent
  gator -j ...
  gator job supervisor [--notify=true|false]
  gator job status|stop
  gator job inbox [--unread]
  gator job inbox read ENTRY_ID
  gator provider PROVIDER [OPTIONS]
  gator provider login PROVIDER [--subscription | --prompt | --api-key KEY | --from-env NAME | --bearer-token TOKEN | --bearer-token-from-env NAME]
  gator provider logout PROVIDER
  gator provider list
  gator provider add ID --base-url URL --model MODEL [--model MODEL...] [--api-key-env NAME]
  gator provider discover ID [--apply]
  gator provider remove ID --yes
  gator -p ...
  gator browser install|status|start|attach|tabs|select|origins|visual|allow-upload|artifacts|export|stop
  gator -b ...
  gator desktop start|status|stop
  gator doctor [--provider PROVIDER]
  gator -d [--provider PROVIDER]
  gator update [--check]
  gator -u [--check]
  gator work [--source DIRECTORY] [--connector ID] [--artifact PATH] [--image PATH] [--attach PATH] [--require-contains PATH=TEXT] [--mode inspect|draft|act] [--actions forbid|draft|approve] [--provider PROVIDER] [--model MODEL] [--base-url URL] [--max-steps N] [CODE POLICY] [--json] TASK
  gator -w [OPTIONS] TASK
  gator work list
  gator work show WORK_ID|CONVERSATION_ID
  gator work history|back CONVERSATION_ID
  gator work forward CONVERSATION [REVISION]
  gator work resume [--refresh-source] [--parent REVISION] CONVERSATION TASK
  gator work inspect [OPTIONS] TASK
  gator work code [CODE POLICY] TASK
  gator work eval validate|run|show|compare DATASET_OR_REPORT [OPTIONS]
  gator work eval learning validate|run|show DATASET_OR_REPORT [OPTIONS]
  gator work review WORK_BUNDLE|WORK_ID [OPTIONS]
  gator work export WORK_BUNDLE|WORK_ID [OPTIONS]
  gator work apply WORK_BUNDLE|WORK_ID [OPTIONS]
  gator work retry WORK_ID [--delivery DELIVERY_ID] [--json]
  gator work feedback WORK_ID accept|reject|correct|remember|dont-learn|list [OPTIONS]
  gator work snapshot list|show ID|gc --yes

Commands:
  agent, -a      profiles, child work, delegated agents, and local agent interfaces
  browser, -b    explicitly control a local Playwright/Chromium session
  desktop         explicitly control approved macOS application windows
  config, -c     persistent settings and project hook trust
  doctor, -d     local prerequisites and suggested verification commands
  extension, -e  extension bundles
  job, -j        scheduled Work and its inbox
  lsp, -l        local Language Server Protocol diagnostics
  learnings       inspect and control scoped user guidance for future Work
  mcp, -m        project MCP configuration and OAuth
  provider, -p   providers, credentials, custom endpoints, and connectors
  theme, -t      terminal themes
  update, -u     release checks and self-update
  work, -w       source-to-deliverable workflows and retained outputs

For a free-form Work task that begins with a nested command name, use
gator work -- TASK to keep it a task rather than dispatching that command.

Cloud providers resolve credentials in this order: --api-key, Gator's private
local auth file, then the provider environment variable. Native runs keep
Gator's orchestration loop; gator agent delegate is an explicit installed-
harness boundary. The internal Code specialist is strict and offline by default.
--verify, --scope, --profile, --setup, --allow-command,
--allow-command-prefix, and --code-capability configure its bounded delegation.`

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
	case "local":
		return errors.New("local model management is available in the TUI; start gator, open /model, and choose Local")
	case "browser", "-b":
		return browserCommand(args[1:], out)
	case "desktop":
		return desktopCommand(args[1:], out)
	case "theme", "-t":
		return themeCommand(args[1:], out)
	case "learning", "learnings":
		return learningCommand(args[1:], out)
	case "work-rpc":
		return workHeadless(context.Background(), os.Stdin, out)
	case "work", "-w":
		return workCommand(args[1:], out)
	case "rpc", "serve", "acp", "child", "delegate", "hook", "connector", "inbox", "snapshot", "inspect", "resume", "eval", "review", "export", "apply":
		return movedCommand(args[0])
	default:
		return fmt.Errorf("unknown command %q; run 'gator --help'", args[0])
	}
}

func supportedPlatform(goos string) bool {
	return goos == "linux" || goos == "darwin"
}

func isHelpArgument(value string) bool {
	return value == "--help" || value == "-h"
}
