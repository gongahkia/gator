package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

const agentFamilyUsage = `usage:
  gator agent list
  gator agent child list|show|batches|batch RUN_RECORD_PATH [CHILD_RUN_ID|BATCH_ID]
  gator agent delegate RUNTIME ACTION [OPTIONS]
  gator agent acp [--verify 'argv ...']
  gator agent rpc
  gator agent serve ACTION [OPTIONS]

short form:
  gator -a ...`

func configCommand(arguments []string, out io.Writer) error {
	if len(arguments) > 0 && arguments[0] == "hook" {
		return hookCommand(arguments[1:], out)
	}
	return configure(arguments, out)
}

func agentFamilyCommand(arguments []string, out io.Writer) error {
	if len(arguments) == 0 {
		return errors.New(agentFamilyUsage)
	}
	if isHelpArgument(arguments[0]) {
		_, err := fmt.Fprintln(out, agentFamilyUsage)
		return err
	}
	switch arguments[0] {
	case "child":
		return childCommand(arguments[1:], out)
	case "delegate":
		return delegate(arguments[1:], out)
	case "acp":
		return acpMode(arguments[1:], os.Stdin, out)
	case "rpc":
		return rpcMode(arguments[1:], os.Stdin, out)
	case "serve":
		return serveCommand(arguments[1:], out)
	case "list":
		return agentCommand(arguments, out)
	default:
		return errors.New(agentFamilyUsage)
	}
}

func workCommand(arguments []string, out io.Writer) error {
	if len(arguments) == 0 {
		return workTask(arguments, out)
	}
	if isHelpArgument(arguments[0]) {
		_, err := fmt.Fprintln(out, usage)
		return err
	}
	switch arguments[0] {
	case "inspect":
		return inspectTask(arguments[1:], out)
	case "code":
		return codeTask(arguments[1:], out)
	case "resume":
		return workResumeCommand(arguments[1:], out)
	case "eval":
		return workEvalCommand(arguments[1:], out)
	case "review":
		return reviewCommand(arguments[1:], out)
	case "export":
		return exportPatch(arguments[1:], out)
	case "apply":
		return applyPatch(arguments[1:], out)
	case "retry":
		return retryWorkDelivery(arguments[1:], out)
	case "snapshot":
		return snapshotCommand(arguments[1:], out)
	default:
		return workTask(arguments, out)
	}
}

func codeTask(arguments []string, out io.Writer) error {
	if len(arguments) == 1 && isHelpArgument(arguments[0]) {
		_, err := fmt.Fprintln(out, codeUsage)
		return err
	}
	if len(arguments) > 0 && (arguments[0] == "resume" || arguments[0] == "fork" || arguments[0] == "clone") {
		return errors.New("standalone Code sessions are retired; use 'gator work resume CONVERSATION' and Gator revision history")
	}
	return runWorkTask(append([]string{"--require-code"}, arguments...), os.Stdin, out, nativeWorkModel)
}

func workResumeCommand(arguments []string, out io.Writer) error {
	if len(arguments) == 0 {
		return unifiedResume(arguments, out)
	}
	if arguments[0] == "--refresh-source" || arguments[0] == "--parent" {
		return workTask(append([]string{"resume"}, arguments...), out)
	}
	return unifiedResume(arguments, out)
}

func movedCommand(command string) error {
	canonical := map[string]string{
		"rpc":       "gator agent rpc",
		"serve":     "gator agent serve",
		"acp":       "gator agent acp",
		"child":     "gator agent child",
		"delegate":  "gator agent delegate",
		"hook":      "gator config hook",
		"connector": "gator provider connector",
		"inbox":     "gator job inbox",
		"snapshot":  "gator work snapshot",
		"inspect":   "gator work inspect",
		"resume":    "gator work resume",
		"eval":      "gator work eval",
		"review":    "gator work review",
		"export":    "gator work export",
		"apply":     "gator work apply",
	}
	return fmt.Errorf("gator %s has moved; use '%s'", command, canonical[command])
}
