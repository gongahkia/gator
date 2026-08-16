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
  gator doctor [--provider PROVIDER]
  gator run [--provider PROVIDER] [--model MODEL] [--base-url URL] [--max-steps N] [--allow-external-cli] --verify 'argv ...' TASK
  gator resume [--allow-external-cli] [--max-steps N] RUN_RECORD_PATH TASK
  gator export RUN_RECORD_PATH
  gator apply [--check] RUN_RECORD_PATH

Commands:
  tui       open the interactive terminal application (the default command)
  doctor    report local prerequisites and suggested verification commands
  run       propose a tested patch in an isolated Git worktree
  resume    continue a retained worktree from its local run record
  export    write a portable patch for a retained run to standard output
  apply     explicitly apply a retained patch to this clean checkout

Cloud providers use their own API-key environment variable. Supported native
providers are openai, azure-openai, anthropic, gemini, mistral, xai, groq,
openrouter, together, fireworks, deepseek, and openai-compatible. Codex, Claude Code,
GitHub Copilot, and Cursor use their already-authenticated local CLIs; select
one explicitly and pass --allow-external-cli in script mode. --verify is
repeatable and every listed command must pass before Gator accepts completion.`

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
