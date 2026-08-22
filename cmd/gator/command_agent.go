package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/gongahkia/gator/internal/instructions"
)

const agentUsage = `usage:
  gator agent list`

// agentCommand makes project-defined delegation roles inspectable before a
// model is allowed to select one. Roles are non-executable prompt data; their
// kind remains enforced by the native delegation tool surface.
func agentCommand(arguments []string, out io.Writer) error {
	if len(arguments) != 1 || arguments[0] != "list" {
		return errors.New(agentUsage)
	}
	directory, err := os.Getwd()
	if err != nil {
		return err
	}
	repository, err := gitRepositoryRoot(directory)
	if err != nil {
		return errors.New("project agent roles can be listed only from inside a Git checkout")
	}
	roles, err := instructions.LoadRoles(repository)
	if err != nil {
		return err
	}
	if len(roles) == 0 {
		_, err := fmt.Fprintln(out, "No project agent roles are configured.")
		return err
	}
	sort.Slice(roles, func(first, second int) bool { return roles[first].Name < roles[second].Name })
	if _, err := fmt.Fprintf(out, "Project agent roles: %s\n", repository); err != nil {
		return err
	}
	for _, role := range roles {
		if _, err := fmt.Fprintf(out, "  %s  %s  %s\n", role.Name, role.Kind, role.Description); err != nil {
			return err
		}
	}
	return nil
}
