package main

import (
	"fmt"
	"io"
	"os"
)

const usage = `Gator — native, inspectable coding agent

Usage:
  gator
  gator tui
  gator help
  gator login PROVIDER [--subscription | --api-key KEY | --from-env NAME | --bearer-token TOKEN | --bearer-token-from-env NAME]
  gator logout PROVIDER
  gator doctor [--provider PROVIDER]
  gator run [--provider PROVIDER] [--model MODEL] [--base-url URL] [--max-steps N] --verify 'argv ...' TASK
  gator resume [--all] [--last [TASK] | THREAD_ID [TASK] | RUN_RECORD_PATH [TASK]]
  gator export RUN_RECORD_PATH
  gator apply [--check] RUN_RECORD_PATH

Commands:
  tui       open the interactive terminal application (the default command)
  login     store a provider credential in Gator's private local auth file
  logout    remove a provider credential from Gator's private local auth file
  doctor    report local prerequisites and suggested verification commands
  run       propose a tested patch in an isolated Git worktree
  resume    select, reopen, or immediately continue a retained local thread
  export    write a portable patch for a retained run to standard output
  apply     explicitly apply a retained patch to this clean checkout

Cloud providers resolve credentials in this order: --api-key, Gator's private
local auth file, then the provider environment variable. Gator never delegates
its tool loop to a vendor CLI. --verify is repeatable and every listed command
must pass before Gator accepts completion.`

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

	switch args[0] {
	case "doctor":
		return doctor(args[1:], out)
	case "login":
		return login(args[1:], out)
	case "logout":
		return logout(args[1:], out)
	case "run":
		return runTask(args[1:], out)
	case "resume":
		return resumeTask(args[1:], out)
	case "export":
		return exportPatch(args[1:], out)
	case "apply":
		return applyPatch(args[1:], out)
	default:
		return fmt.Errorf("unknown command %q; run 'gator help'", args[0])
	}
}
