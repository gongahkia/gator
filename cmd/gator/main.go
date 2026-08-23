package main

import (
	"fmt"
	"io"
	"os"
)

// These values are set for release builds with -ldflags. Development builds
// remain explicit rather than pretending to have a published version.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const usage = `Gator — native, inspectable coding agent

Usage:
  gator
  gator tui
  gator help
  gator version
  gator update [--check]
  gator rpc
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
  gator worktree list|prune|remove RUN_ID --yes
  gator extension list|status
  gator extension install [--replace] DIRECTORY
  gator provider list
  gator provider add ID --base-url URL --model MODEL [--model MODEL...] [--api-key-env NAME]
  gator theme list
  gator theme set gator|contrast|mono
  gator connect PROVIDER [OPTIONS]
  gator login PROVIDER [--subscription | --api-key KEY | --from-env NAME | --bearer-token TOKEN | --bearer-token-from-env NAME]
  gator logout PROVIDER
  gator delegate RUNTIME ACTION [OPTIONS]
  gator doctor [--provider PROVIDER]
  gator run [--provider PROVIDER] [--model MODEL] [--base-url URL] [--max-steps N] [--sandbox strict|off] [--network deny|allow] [--base REF] [--copy-ignored] [--scope PATH] [--scout TASK] --verify 'argv ...' [--allow-command 'argv ...'] [--trust-commands] TASK
  gator resume [--all] [--last [TASK] | THREAD_ID [TASK] | RUN_RECORD_PATH [TASK]]
  gator fork [--all] [--last [TASK] | THREAD_ID [TASK] | RUN_RECORD_PATH [TASK]]
  gator clone [--all] [--last [TASK] | THREAD_ID [TASK] | RUN_RECORD_PATH [TASK]]
  gator transcript RUN_RECORD_PATH > transcript.html
  gator export RUN_RECORD_PATH
  gator apply [--check] RUN_RECORD_PATH

Commands:
  tui       open the interactive terminal application (the default command)
  connect   start a provider-owned or API-key onboarding flow
  login     store a provider credential in Gator's private local auth file
  logout    remove a provider credential from Gator's private local auth file
  delegate  run an installed vendor or external agent in an isolated worktree
  acp       run a local Agent Client Protocol v1 stdio agent for an editor
  agent     list project-defined, capability-bounded delegation roles
	 child     inspect durable manifests for retained isolated writer children
  lsp       trust and inspect local Language Server Protocol diagnostics
  extension install, enable, trust, or remove Gator extension bundles
  provider  configure a custom/local Chat Completions provider
  theme     list or choose Gator's terminal theme
  doctor    report local prerequisites and suggested verification commands
  run       propose a tested patch in an isolated Git worktree
  resume    select, reopen, or immediately continue a retained local thread
  transcript export one retained private session as local HTML
  export    write a portable patch for a retained run to standard output
  apply     explicitly apply a retained patch to this clean checkout

Cloud providers resolve credentials in this order: --api-key, Gator's private
local auth file, then the provider environment variable. Native runs keep
Gator's tool loop; gator delegate is an explicit installed-harness boundary.
--verify is repeatable and every listed command must pass before Gator accepts
completion. Exploratory worktree commands wait for approval unless listed with
--allow-command or auto-approved with --trust-commands (unsafe; not a sandbox).`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "gator:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
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
	case "worktree":
		return worktreeCommand(args[1:], out)
	case "provider":
		return providerCommand(args[1:], out)
	case "theme":
		return themeCommand(args[1:], out)
	case "run":
		return runTask(args[1:], out)
	case "resume":
		return resumeTask(args[1:], out)
	case "fork":
		return forkTask(args[1:], out)
	case "clone":
		return cloneTask(args[1:], out)
	case "transcript":
		return exportTranscript(args[1:], out)
	case "export":
		return exportPatch(args[1:], out)
	case "apply":
		return applyPatch(args[1:], out)
	default:
		return fmt.Errorf("unknown command %q; run 'gator help'", args[0])
	}
}
