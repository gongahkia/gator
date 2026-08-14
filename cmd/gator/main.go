package main

import (
	"fmt"
	"io"
	"os"
)

const usage = `Gator — native, inspectable coding agent

Usage:
  gator help
  gator doctor

Commands:
  doctor    report local prerequisites for a future agent run

The model-backed run command is not available yet. See README.md for the
current milestone.`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "gator:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(out, usage)
		return err
	}

	switch args[0] {
	case "doctor":
		_, err := fmt.Fprintln(out, "Gator core is installed. Model and workspace checks arrive with the first runnable milestone.")
		return err
	default:
		return fmt.Errorf("unknown command %q; run 'gator help'", args[0])
	}
}
