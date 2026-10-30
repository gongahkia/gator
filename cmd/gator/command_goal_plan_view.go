package main

import (
	"bytes"
	"errors"
	"strings"

	"github.com/gongahkia/gator/internal/goal"
)

const tuiPlanUsage = `usage:
  /plan
  /plan outcome TEXT
  /plan add TITLE [| RFC3339_DUE [| EVIDENCE]]
  /plan step STEP_ID pending|doing|blocked|done [| EVIDENCE]
  /plan review-at RFC3339|clear
  /plan occurrence STEP_ID YYYY-MM-DD pending|doing|blocked|done|skipped|clear
  /plan reviewed
  /plan remove STEP_ID --yes`

func workTUIPlanAction(store goal.Store, goalID, command string) (string, error) {
	command = strings.TrimSpace(command)
	arguments := []string{goalID}
	switch {
	case command == "":
	case command == "help":
		return tuiPlanUsage, nil
	case strings.HasPrefix(command, "outcome "):
		arguments = append(arguments, "outcome", strings.TrimSpace(strings.TrimPrefix(command, "outcome ")))
	case strings.HasPrefix(command, "add "):
		parts := strings.SplitN(strings.TrimPrefix(command, "add "), "|", 3)
		title := strings.TrimSpace(parts[0])
		if title == "" {
			return "", errors.New(tuiPlanUsage)
		}
		arguments = append(arguments, "add", "--title", title)
		if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
			arguments = append(arguments, "--due", strings.TrimSpace(parts[1]))
		}
		if len(parts) > 2 && strings.TrimSpace(parts[2]) != "" {
			arguments = append(arguments, "--evidence", strings.TrimSpace(parts[2]))
		}
	case strings.HasPrefix(command, "step "):
		parts := strings.SplitN(strings.TrimPrefix(command, "step "), "|", 2)
		fields := strings.Fields(parts[0])
		if len(fields) != 2 {
			return "", errors.New(tuiPlanUsage)
		}
		arguments = append(arguments, "step", fields[0], fields[1])
		if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
			arguments = append(arguments, "--evidence", strings.TrimSpace(parts[1]))
		}
	case strings.HasPrefix(command, "review-at "):
		arguments = append(arguments, "review-at", strings.TrimSpace(strings.TrimPrefix(command, "review-at ")))
	case strings.HasPrefix(command, "occurrence "):
		arguments = append(arguments, strings.Fields(command)...)
	case command == "reviewed":
		arguments = append(arguments, "reviewed")
	case strings.HasPrefix(command, "remove "):
		fields := strings.Fields(command)
		if len(fields) != 3 || fields[2] != "--yes" {
			return "", errors.New(tuiPlanUsage)
		}
		arguments = append(arguments, fields...)
	default:
		return "", errors.New(tuiPlanUsage)
	}
	var output bytes.Buffer
	if err := goalPlanCommand(store, arguments, &output); err != nil {
		return "", err
	}
	return strings.TrimSpace(output.String()), nil
}
