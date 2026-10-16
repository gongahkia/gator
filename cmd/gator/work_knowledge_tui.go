package main

import (
	"bytes"
	"errors"
	"strings"
)

func workTUINoteAction(command, goalID string) (string, error) {
	command = strings.TrimSpace(command)
	var args []string
	switch {
	case strings.HasPrefix(command, "add "):
		parts := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(command, "add ")), "|", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return "", errors.New("usage: /note add TITLE | BODY")
		}
		args = []string{"add", "--title", strings.TrimSpace(parts[0]), "--body", strings.TrimSpace(parts[1])}
		if goalID != "" {
			args = append(args, "--goal", goalID)
		}
	case strings.HasPrefix(command, "edit "):
		parts := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(command, "edit ")), "|", 2)
		if len(parts) != 2 || len(strings.Fields(parts[0])) != 1 || strings.TrimSpace(parts[1]) == "" {
			return "", errors.New("usage: /note edit ID | BODY")
		}
		args = []string{"edit", strings.TrimSpace(parts[0]), "--body", strings.TrimSpace(parts[1])}
	case strings.HasPrefix(command, "search "):
		args = []string{"search"}
		if goalID != "" {
			args = append(args, "--goal", goalID)
		}
		args = append(args, strings.TrimSpace(strings.TrimPrefix(command, "search ")))
	default:
		args = strings.Fields(command)
		if goalID != "" && len(args) == 1 && args[0] == "list" {
			args = append(args, "--goal", goalID)
		}
	}
	var output bytes.Buffer
	err := noteCommand(args, &output)
	return strings.TrimSpace(output.String()), err
}

func workTUISkillAction(command string) (string, error) {
	command = strings.TrimSpace(command)
	var args []string
	for _, action := range []string{"create", "edit"} {
		if strings.HasPrefix(command, action+" ") {
			parts := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(command, action+" ")), "|", 3)
			if len(parts) != 3 || len(strings.Fields(parts[0])) != 1 || strings.TrimSpace(parts[1]) == "" || strings.TrimSpace(parts[2]) == "" {
				return "", errors.New("usage: /skill " + action + " NAME | WHEN_TO_USE | BODY")
			}
			args = []string{action, strings.TrimSpace(parts[0]), "--description", strings.TrimSpace(parts[1]), "--body", strings.TrimSpace(parts[2])}
			break
		}
	}
	if args == nil {
		args = strings.Fields(command)
	}
	var output bytes.Buffer
	err := skillCommand(args, &output)
	return strings.TrimSpace(output.String()), err
}
