package run

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gongahkia/gator/internal/instructions"
)

// rolesForKind exposes only roles which match a pre-existing delegation
// capability. Project configuration can specialize a worker's prompt, but it
// cannot turn a read-only scout into a writer or grant a writer new tools.
func rolesForKind(roles []instructions.Role, kind string) map[string]instructions.Role {
	result := make(map[string]instructions.Role)
	for _, role := range roles {
		if role.Kind == kind {
			result[role.Name] = role
		}
	}
	return result
}

func resolveRole(roles map[string]instructions.Role, name string) (instructions.Role, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return instructions.Role{}, nil
	}
	role, found := roles[name]
	if !found {
		return instructions.Role{}, fmt.Errorf("agent role %q is not available for this delegation", name)
	}
	return role, nil
}

func roleSchemaProperty(roles map[string]instructions.Role) string {
	if len(roles) == 0 {
		return ""
	}
	names := roleNames(roles)
	encoded, err := json.Marshal(names)
	if err != nil {
		return ""
	}
	return `,"role":{"type":"string","enum":` + string(encoded) + `,"description":"Optional project-defined specialist role. It changes instructions only, never capabilities."}`
}

func roleCatalog(roles map[string]instructions.Role) string {
	if len(roles) == 0 {
		return ""
	}
	parts := make([]string, 0, len(roles))
	for _, name := range roleNames(roles) {
		role := roles[name]
		parts = append(parts, name+" — "+role.Description)
	}
	return strings.Join(parts, "; ")
}

func roleNames(roles map[string]instructions.Role) []string {
	names := make([]string, 0, len(roles))
	for name := range roles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func rolePrompt(role instructions.Role) string {
	if role.Name == "" {
		return ""
	}
	return "Selected project role " + role.Name + ":\n" + role.Instructions
}
