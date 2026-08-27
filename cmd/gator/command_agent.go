package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/gongahkia/gator/internal/instructions"
)

const agentUsage = `usage:
  gator agent list`

// agentCommand makes project-defined profiles and roles inspectable before a
// model is allowed to select one. Profiles can only narrow run policy. Roles
// are non-executable prompt data; their kind remains enforced by the native
// delegation tool surface.
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
		return errors.New("project agent profiles and roles can be listed only from inside a Git checkout")
	}
	if _, err := fmt.Fprintf(out, "Project agents: %s\n", repository); err != nil {
		return err
	}
	profiles, err := instructions.ListProfiles(repository)
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		if _, err := fmt.Fprintln(out, "Profiles: none configured"); err != nil {
			return err
		}
	} else {
		sort.Slice(profiles, func(first, second int) bool { return profiles[first].Name < profiles[second].Name })
		if _, err := fmt.Fprintln(out, "Profiles (can only narrow policy):"); err != nil {
			return err
		}
		for _, profile := range profiles {
			omit := "none"
			if len(profile.Policy.Omit) > 0 {
				omit = strings.Join(profile.Policy.Omit, ",")
			}
			if _, err := fmt.Fprintf(out, "  %s  %s  omit=%s\n", profile.Name, profile.Description, omit); err != nil {
				return err
			}
		}
	}
	roles, err := instructions.LoadRoles(repository)
	if err != nil {
		return err
	}
	if len(roles) == 0 {
		_, err := fmt.Fprintln(out, "Roles: none configured")
		return err
	}
	sort.Slice(roles, func(first, second int) bool { return roles[first].Name < roles[second].Name })
	if _, err := fmt.Fprintln(out, "Roles (prompt-only specializations):"); err != nil {
		return err
	}
	for _, role := range roles {
		if _, err := fmt.Fprintf(out, "  %s  %s  %s\n", role.Name, role.Kind, role.Description); err != nil {
			return err
		}
	}
	return nil
}
