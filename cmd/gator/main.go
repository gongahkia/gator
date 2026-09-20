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
  gator provider connector list
  gator provider connector add ID --kind json|webhook|slack|google|atlassian|notion|mcp [--url URL] [--name NAME] [--auth none|bearer|oauth]
  gator provider connector status|login|logout|test|remove ID [OPTIONS]
  gator provider connector permission ID OPERATION read|write allow|ask|deny|draft
  gator job add|list|show|edit|enable|disable|run|history|remove
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
  gator doctor [--provider PROVIDER]
  gator -d [--provider PROVIDER]
  gator update [--check]
  gator -u [--check]
  gator work [--source DIRECTORY] [--connector ID] [--artifact PATH] [--image PATH] [--attach PATH] [--require-contains PATH=TEXT] [--mode inspect|draft|act] [--actions forbid|draft|approve] [--provider PROVIDER] [--model MODEL] [--base-url URL] [--max-steps N] [CODE POLICY] [--json] TASK
  gator -w [OPTIONS] TASK
  gator work list
  gator work show|history|back CONVERSATION
  gator work forward CONVERSATION [REVISION]
  gator work resume [--refresh-source] [--parent REVISION] CONVERSATION TASK
  gator work inspect [OPTIONS] TASK
  gator work code [CODE POLICY] TASK
  gator work run [CODE POLICY] TASK
  gator work eval DIR [OPTIONS]
  gator work eval suite DIR [OPTIONS]
  gator work transcript RUN_RECORD_PATH > transcript.html
  gator work review WORK_BUNDLE|RUN_RECORD_PATH [OPTIONS]
  gator work export WORK_BUNDLE|RUN_RECORD_PATH [OPTIONS]
  gator work apply WORK_BUNDLE|RUN_RECORD_PATH [OPTIONS]
  gator work snapshot list|show ID|gc --yes
  gator work worktree list|prune|remove RUN_ID --yes

Commands:
  agent, -a      profiles, child work, delegated agents, and local agent interfaces
  browser, -b    explicitly control a local Playwright/Chromium session
  config, -c     persistent settings and project hook trust
  doctor, -d     local prerequisites and suggested verification commands
  extension, -e  extension bundles
  job, -j        scheduled Work and its inbox
  lsp, -l        local Language Server Protocol diagnostics
  mcp, -m        project MCP configuration and OAuth
  provider, -p   providers, credentials, custom endpoints, and connectors
  theme, -t      terminal themes
  update, -u     release checks and self-update
  work, -w       source-to-deliverable workflows and retained outputs

Cloud providers resolve credentials in this order: --api-key, Gator's private
local auth file, then the provider environment variable. Native runs keep
Gator's orchestration loop; gator agent delegate is an explicit installed-
harness boundary. The internal Code specialist is strict and offline by default.
--verify, --scope, --profile, --setup, --allow-command,
--allow-command-prefix, and --code-capability configure its bounded delegation.`

const codeUsage = `Gator coding — one Gator manager with an internal Code specialist

Usage:
  gator work code [CODE POLICY] TASK
  gator work run [CODE POLICY] TASK

Both commands use the main Gator orchestration path and require retained Code
patch evidence. They never open a standalone Code session or TUI.

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
  --browser-session ID
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
	case "theme", "-t":
		return themeCommand(args[1:], out)
	case "work-rpc":
		return workHeadless(context.Background(), os.Stdin, out)
	case "work", "-w":
		return workCommand(args[1:], out)
	case "rpc", "serve", "acp", "child", "delegate", "hook", "connector", "inbox", "snapshot", "worktree", "inspect", "code", "run", "resume", "eval", "transcript", "review", "export", "apply":
		return movedCommand(args[0])
	case "fork":
		return errors.New("standalone Code forks are retired; branch a Gator conversation with 'gator work resume --parent REVISION CONVERSATION TASK'")
	case "clone":
		return errors.New("standalone Code clones are retired; continue or branch a retained Gator conversation instead")
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
