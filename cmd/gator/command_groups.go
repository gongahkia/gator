package main

import (
	"errors"
	"fmt"
	"io"
)

const agentFamilyUsage = `usage:
  gator agent list
  gator agent cli list|enable|disable|doctor ...

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
	case "acp":
		return errors.New("ACP Code mode is retired; use Gator conversations or work-rpc for automation")
	case "list", "cli":
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
		return errors.New("direct Code mode is retired; describe the outcome in a Gator conversation")
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
	case "feedback":
		return workFeedbackCommand(arguments[1:], out)
	case "snapshot":
		return snapshotCommand(arguments[1:], out)
	default:
		return workTask(arguments, out)
	}
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
