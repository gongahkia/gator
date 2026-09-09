package main

import (
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
  gator tui
  gator help
  gator version
  gator update [--check]
  gator rpc
  gator serve token ABSOLUTE_PATH
  gator serve start --token-file ABSOLUTE_PATH [--listen 127.0.0.1:PORT]
  gator serve status --token-file ABSOLUTE_PATH
  gator serve stop --token-file ABSOLUTE_PATH
  gator serve --token-file ABSOLUTE_PATH [--listen 127.0.0.1:PORT]
  gator acp [--verify 'argv ...']
  gator config [show]
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
  gator child list|show|batches|batch RUN_RECORD_PATH [CHILD_RUN_ID|BATCH_ID]
  gator hook status|trust|untrust
  gator lsp status|trust|untrust
  gator mcp status|trust|untrust|login|logout
  gator connector list
  gator connector add ID --kind json|webhook|slack|google|atlassian|notion|mcp [--url URL] [--name NAME] [--auth none|bearer|oauth]
  gator connector status|login|logout|test|remove ID [OPTIONS]
  gator connector permission ID OPERATION read|write allow|ask|deny|draft
  gator job add|list|show|edit|enable|disable|run|history|remove
  gator job supervisor [--notify=true|false]
  gator job status|stop
  gator inbox [--unread]
  gator inbox read ENTRY_ID
  gator snapshot list|show ID|gc --yes
  gator worktree list|prune|remove RUN_ID --yes
  gator extension list|status
  gator extension install [--replace] DIRECTORY
  gator provider list
  gator provider add ID --base-url URL --model MODEL [--model MODEL...] [--api-key-env NAME]
  gator local list|status|serve|pull|use|remove
  gator browser install|status|start|attach|tabs|select|origins|visual|allow-upload|artifacts|export|stop
  gator theme list
  gator theme set gator|contrast|mono
  gator connect PROVIDER [OPTIONS]
  gator login PROVIDER [--subscription | --api-key KEY | --from-env NAME | --bearer-token TOKEN | --bearer-token-from-env NAME]
  gator logout PROVIDER
  gator delegate RUNTIME ACTION [OPTIONS]
  gator doctor [--provider PROVIDER]
  gator work [--source DIRECTORY] [--connector ID] [--artifact PATH] [--image PATH] [--attach PATH] [--require-contains PATH=TEXT] [--mode inspect|draft|act] [--actions forbid|draft|approve] [--provider PROVIDER] [--model MODEL] [--base-url URL] [--max-steps N] [CODE POLICY] [--json] TASK
  gator work list
  gator work show|history|back CONVERSATION
  gator work forward CONVERSATION [REVISION]
  gator work resume [--refresh-source] [--parent REVISION] CONVERSATION TASK
  gator inspect [--source DIRECTORY] [--connector ID] [--provider PROVIDER] [--model MODEL] [--base-url URL] [--max-steps N] [--json] TASK
  gator code [CODE POLICY] TASK
  gator run [CODE POLICY] TASK
  gator resume [WORK_CONVERSATION_ID TASK]
  gator eval DIR [--run-id ID] [--report PATH] [--script PATH] [--live] [--provider PROVIDER] [--model MODEL] [--base-url URL] [--environment-id ID] [--require-resolved]
  gator eval suite DIR [--run-id ID] [--report-dir DIR] [--attempts N] --environment-id ID --live [--provider PROVIDER] [--model MODEL] [--base-url URL] [--require-resolved]
  gator transcript RUN_RECORD_PATH > transcript.html
  gator review WORK_BUNDLE [--preview] [--json]
  gator review RUN_RECORD_PATH [--listen 127.0.0.1:PORT] [--open]
  gator export WORK_BUNDLE [--to ARCHIVE] [--replace]
  gator export RUN_RECORD_PATH
  gator apply WORK_BUNDLE --to DIRECTORY [--check] [--replace] [--json]
  gator apply [--check] RUN_RECORD_PATH

Commands:
  tui       open the interactive terminal application (the default command)
  connect   start a provider-owned or API-key onboarding flow
  login     store a provider credential in Gator's private local auth file
  logout    remove a provider credential from Gator's private local auth file
  delegate  run an installed vendor or external agent in an isolated worktree
  serve     run the authenticated loopback HTTP/SSE app-server bridge
  acp       run a local Agent Client Protocol v1 stdio agent for an editor
  agent     list project-defined profiles and capability-bounded roles
  child     inspect durable manifests for retained isolated writer children
  lsp       trust and inspect local Language Server Protocol diagnostics
  connector configure connected sources/actions and resource-bound authentication
  job       manage recurring Work and its foreground supervisor
  inbox     inspect or mark scheduled-work results
  snapshot  inspect or explicitly collect immutable source snapshots
  extension install, enable, trust, or remove Gator extension bundles
  provider  configure a custom/local Chat Completions provider
  local     install, select, and manage curated Ollama coding models
  browser   start, attach, and explicitly control a local Playwright/Chromium session
  theme     list or choose Gator's terminal theme
  doctor    report local prerequisites and suggested verification commands
  work      turn a read-only folder into validated artifacts in isolated output
  inspect   analyze a folder without files, processes, or external actions
  code      route a coding request through Gator and its internal Code specialist
  run       compatibility alias for the same Gator orchestration route
  resume    reopen or continue a retained Gator conversation by ID
  eval      run a bounded headless evaluation fixture or real-model suite
  transcript export one retained private session as local HTML
  review    verify and inspect a Work bundle or retained coding run
  export    archive a Work bundle or write a retained coding patch
  apply     explicitly transfer verified artifacts or a retained coding patch

Cloud providers resolve credentials in this order: --api-key, Gator's private
local auth file, then the provider environment variable. Native runs keep
Gator's orchestration loop; gator delegate is an explicit installed-harness
boundary. The internal Code specialist is strict and offline by default.
--verify, --scope, --profile, --setup, --allow-command,
--allow-command-prefix, and --code-capability configure its bounded delegation.`

const codeUsage = `Gator coding — one Gator manager with an internal Code specialist

Usage:
  gator code [CODE POLICY] TASK
  gator run [CODE POLICY] TASK

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
and git diff --check verification. Use gator resume and Work revision history
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
	if len(args) == 0 || args[0] == "tui" {
		return workInteractive()
	}
	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(out, usage)
		return err
	}
	if len(args) == 2 && args[0] == "--mode" && args[1] == "rpc" {
		return rpcMode(nil, os.Stdin, out)
	}
	if len(args) == 2 && args[0] == "--mode" && args[1] == "acp" {
		return acpMode(nil, os.Stdin, out)
	}
	if args[0] == "version" || args[0] == "--version" || args[0] == "-v" {
		_, err := fmt.Fprintf(out, "gator %s (%s, %s)\n", version, commit, date)
		return err
	}

	switch args[0] {
	case "connect":
		return connect(args[1:], out)
	case "config":
		return configure(args[1:], out)
	case "agent":
		return agentCommand(args[1:], out)
	case "child":
		return childCommand(args[1:], out)
	case "update":
		return update(args[1:], out)
	case "rpc":
		return rpcMode(args[1:], os.Stdin, out)
	case "serve":
		return serveCommand(args[1:], out)
	case "acp":
		return acpMode(args[1:], os.Stdin, out)
	case "doctor":
		return doctor(args[1:], out)
	case "login":
		return login(args[1:], out)
	case "logout":
		return logout(args[1:], out)
	case "delegate":
		return delegate(args[1:], out)
	case "extension":
		return extensionCommand(args[1:], out)
	case "hook":
		return hookCommand(args[1:], out)
	case "lsp":
		return lspCommand(args[1:], out)
	case "mcp":
		return mcpCommand(args[1:], out)
	case "connector":
		return connectorCommand(args[1:], out)
	case "job":
		return jobCommand(args[1:], out)
	case "inbox":
		return inboxCommand(args[1:], out)
	case "snapshot":
		return snapshotCommand(args[1:], out)
	case "worktree":
		return worktreeCommand(args[1:], out)
	case "provider":
		return providerCommand(args[1:], out)
	case "local":
		return localCommand(args[1:], out)
	case "browser":
		return browserCommand(args[1:], out)
	case "theme":
		return themeCommand(args[1:], out)
	case "work":
		return workTask(args[1:], out)
	case "inspect":
		return inspectTask(args[1:], out)
	case "code":
		if len(args) == 1 || len(args) == 2 && args[1] == "--tui" {
			return workInteractive()
		}
		if len(args) == 2 && isHelpArgument(args[1]) {
			_, err := fmt.Fprintln(out, codeUsage)
			return err
		}
		if args[1] == "resume" || args[1] == "fork" || args[1] == "clone" {
			return errors.New("standalone Code sessions are retired; use 'gator resume CONVERSATION' and Gator revision history")
		}
		return runWorkTask(append([]string{"--require-code"}, args[1:]...), os.Stdin, out, nativeWorkModel)
	case "run":
		if len(args) == 2 && isHelpArgument(args[1]) {
			_, err := fmt.Fprintln(out, codeUsage)
			return err
		}
		return runWorkTask(append([]string{"--require-code"}, args[1:]...), os.Stdin, out, nativeWorkModel)
	case "resume":
		return unifiedResume(args[1:], out)
	case "fork":
		return errors.New("standalone Code forks are retired; branch a Gator conversation with 'gator work resume --parent REVISION CONVERSATION TASK'")
	case "clone":
		return errors.New("standalone Code clones are retired; continue or branch a retained Gator conversation instead")
	case "eval":
		return evalCommand(args[1:], out)
	case "transcript":
		return exportTranscript(args[1:], out)
	case "review":
		return reviewCommand(args[1:], out)
	case "export":
		return exportPatch(args[1:], out)
	case "apply":
		return applyPatch(args[1:], out)
	default:
		return fmt.Errorf("unknown command %q; run 'gator help'", args[0])
	}
}

func supportedPlatform(goos string) bool {
	return goos == "linux" || goos == "darwin"
}

func isHelpArgument(value string) bool {
	return value == "--help" || value == "-h" || value == "help"
}
