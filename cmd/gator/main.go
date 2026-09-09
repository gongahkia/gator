package main

import (
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
  gator agent list
  gator child list|show|batches|batch RUN_RECORD_PATH [CHILD_RUN_ID|BATCH_ID]
  gator hook status|trust|untrust
  gator lsp status|trust|untrust
  gator mcp status|trust|untrust|login|logout
  gator connector list|add|status|login|logout|test|remove
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
  gator work [--source DIRECTORY] [--connector ID] [--artifact PATH] [--require-contains PATH=TEXT] [--mode inspect|draft|act] [--actions forbid|draft|approve] [--provider PROVIDER] [--model MODEL] [--base-url URL] [--max-steps N] [--json] TASK
  gator code [coding options] --verify 'argv ...' TASK
  gator run [--provider PROVIDER] [--model MODEL] [--base-url URL] [--image PATH] [--attach PATH] [--browser-session ID] [--max-steps N] [--sandbox strict|off] [--network deny|allow] [--base REF] [--copy-ignored] [--setup 'argv ...'] [--scope PATH] [--scout TASK] --verify 'argv ...' [--allow-command 'argv ...'] [--allow-command-prefix 'argv ...'] [--trust-commands] TASK
  gator resume [--all] [--last [TASK] | THREAD_ID [TASK] | RUN_RECORD_PATH [TASK]]
  gator fork [--all] [--last [TASK] | THREAD_ID [TASK] | RUN_RECORD_PATH [TASK]]
  gator clone [--all] [--last [TASK] | THREAD_ID [TASK] | RUN_RECORD_PATH [TASK]]
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
  connector configure explicit connected sources and resource-bound authentication
  extension install, enable, trust, or remove Gator extension bundles
  provider  configure a custom/local Chat Completions provider
  local     install, select, and manage curated Ollama coding models
  browser   start, attach, and explicitly control a local Playwright/Chromium session
  theme     list or choose Gator's terminal theme
  doctor    report local prerequisites and suggested verification commands
  work      turn a read-only folder into validated artifacts in isolated output
  code      propose a tested patch in an isolated Git worktree
  run       compatibility alias for code
  resume    select, reopen, or immediately continue a retained local thread
  eval      run a bounded headless evaluation fixture or real-model suite
  transcript export one retained private session as local HTML
  review    serve an authenticated loopback browser review for one retained run
  export    write a portable patch for a retained run to standard output
  apply     explicitly apply a retained patch to this clean checkout

Cloud providers resolve credentials in this order: --api-key, Gator's private
local auth file, then the provider environment variable. Native runs keep
Gator's tool loop; gator delegate is an explicit installed-harness boundary.
--verify is repeatable and every listed command must pass before Gator accepts
completion. Exploratory worktree commands wait for approval unless listed with
--allow-command, --allow-command-prefix, or auto-approved with --trust-commands (unsafe; not a sandbox).`

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
		return interactive()
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
	case "code", "run":
		return runTask(args[1:], out)
	case "resume":
		return resumeTask(args[1:], out)
	case "fork":
		return forkTask(args[1:], out)
	case "clone":
		return cloneTask(args[1:], out)
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
