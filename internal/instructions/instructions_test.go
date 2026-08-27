package instructions

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadLayersRootScopedAndMatchingRules(t *testing.T) {
	repository := t.TempDir()
	writeInstructionFile(t, repository, "AGENTS.md", "root guidance")
	writeInstructionFile(t, repository, "AGENTS.override.md", "root override")
	writeInstructionFile(t, repository, ".gator/AGENTS.md", "gator guidance")
	writeInstructionFile(t, repository, "cmd/AGENTS.md", "cmd guidance")
	writeInstructionFile(t, repository, "cmd/tool/AGENTS.md", "tool guidance")
	writeInstructionFile(t, repository, ".gator/rules/cmd.md", "rule file guidance")
	writeInstructionFile(t, repository, ".gator/rules.json", `{
  "version": 1,
  "rules": [
    {"id": "command", "paths": ["cmd/**/*.go"], "instructions": "inline command guidance"},
    {"id": "command-file", "paths": ["cmd/**"], "file": ".gator/rules/cmd.md"},
    {"id": "internal-only", "paths": ["internal/**"], "instructions": "must not load"}
  ]
}`)

	set, err := Load(repository, []string{"cmd/tool/main.go"})
	if err != nil {
		t.Fatalf("load instructions: %v", err)
	}
	for _, want := range []string{
		"Instructions from AGENTS.override.md:\nroot override",
		"Instructions from .gator/AGENTS.md:\ngator guidance",
		"Instructions from cmd/AGENTS.md:\ncmd guidance",
		"Instructions from cmd/tool/AGENTS.md:\ntool guidance",
		"Instructions from project rule command:\ninline command guidance",
		"Instructions from project rule command-file from .gator/rules/cmd.md:\nrule file guidance",
	} {
		if !strings.Contains(set.Content, want) {
			t.Errorf("content does not contain %q:\n%s", want, set.Content)
		}
	}
	if strings.Contains(set.Content, "root guidance") || strings.Contains(set.Content, "must not load") {
		t.Fatalf("unexpected instructions loaded:\n%s", set.Content)
	}
	wantFiles := []string{
		"AGENTS.override.md", ".gator/AGENTS.md", "cmd/AGENTS.md", "cmd/tool/AGENTS.md",
		".gator/rules.json", ".gator/rules/cmd.md",
	}
	if !reflect.DeepEqual(set.Files, wantFiles) {
		t.Fatalf("loaded files = %#v, want %#v", set.Files, wantFiles)
	}
}

func TestLoadRejectsEscapingScopeAndRuleFile(t *testing.T) {
	repository := t.TempDir()
	if _, err := Load(repository, []string{"../outside.go"}); err == nil {
		t.Fatal("Load accepted an escaping instruction scope")
	}
	writeInstructionFile(t, repository, ".gator/rules.json", `{
  "version": 1,
  "rules": [{"paths": ["**"], "file": "../outside.md"}]
}`)
	if _, err := Load(repository, []string{"main.go"}); err == nil || !strings.Contains(err.Error(), "stay below .gator") {
		t.Fatalf("Load rule file error = %v, want containment error", err)
	}
}

func TestLoadRejectsUnknownRuleFields(t *testing.T) {
	repository := t.TempDir()
	writeInstructionFile(t, repository, ".gator/rules.json", `{
  "version": 1,
  "rules": [],
  "unexpected": true
}`)
	if _, err := Load(repository, []string{"main.go"}); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Load rules error = %v, want unknown-field error", err)
	}
}

func TestLoadWithProfileAppendsOnlySelectedProfile(t *testing.T) {
	repository := t.TempDir()
	writeInstructionFile(t, repository, "AGENTS.md", "base guidance")
	writeInstructionFile(t, repository, ".gator/profiles/review.md", "review the diff and report findings")
	writeInstructionFile(t, repository, ".gator/agents.json", `{
  "version": 1,
  "profiles": [
    {"name": "implementer", "instructions": "make the smallest coherent change"},
    {"name": "reviewer", "file": ".gator/profiles/review.md"}
  ]
}`)
	set, err := LoadWithProfile(repository, nil, "reviewer")
	if err != nil {
		t.Fatalf("load profile: %v", err)
	}
	if !strings.Contains(set.Content, "base guidance") || !strings.Contains(set.Content, "selected agent profile reviewer") || strings.Contains(set.Content, "smallest coherent") {
		t.Fatalf("profile content = %q", set.Content)
	}
	if _, err := LoadWithProfile(repository, nil, "missing"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("missing profile error = %v", err)
	}
}

func TestListProfilesReturnsPolicyWithoutActivatingInstructions(t *testing.T) {
	repository := t.TempDir()
	writeInstructionFile(t, repository, ".gator/agents.json", `{
  "version": 1,
  "profiles": [
    {"name": "reviewer", "description": "read-only review", "instructions": "report findings", "policy": {"mode": "plan", "omit": ["lsp", "run_command"]}}
  ]
}`)
	profiles, err := ListProfiles(repository)
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}
	if len(profiles) != 1 || profiles[0].Name != "reviewer" || !profiles[0].Policy.HasOmit(OmitLSP) || profiles[0].Policy.Mode != "plan" {
		t.Fatalf("profiles = %#v", profiles)
	}
}

func TestLoadRolesRestrictsDefinitionsToPromptSpecialization(t *testing.T) {
	repository := t.TempDir()
	writeInstructionFile(t, repository, ".gator/roles/reviewer.md", "identify regressions and evidence gaps")
	writeInstructionFile(t, repository, ".gator/agents.json", `{
  "version": 1,
  "profiles": [{"name": "implementer", "instructions": "make focused changes"}],
  "roles": [
    {"name": "reviewer", "description": "independent code review", "kind": "readonly", "file": ".gator/roles/reviewer.md"},
    {"name": "test-fixer", "description": "implement a focused failing-test fix", "kind": "writer", "instructions": "modify only the delegated test area"}
  ]
}`)
	roles, err := LoadRoles(repository)
	if err != nil {
		t.Fatalf("load roles: %v", err)
	}
	if len(roles) != 2 || roles[0].Name != "reviewer" || roles[0].Kind != RoleReadOnly || !strings.Contains(roles[0].Instructions, "regressions") || roles[1].Name != "test-fixer" || roles[1].Kind != RoleWriter {
		t.Fatalf("roles = %#v", roles)
	}
}

func TestLoadRolesRejectsCapabilityEscalationAndMalformedRoles(t *testing.T) {
	repository := t.TempDir()
	writeInstructionFile(t, repository, ".gator/agents.json", `{
  "version": 1,
  "roles": [
    {"name": "shell", "description": "run unrestricted commands", "kind": "command", "instructions": "do anything"}
  ]
}`)
	if _, err := LoadRoles(repository); err == nil || !strings.Contains(err.Error(), "unsupported kind") {
		t.Fatalf("capability-escalating role error = %v", err)
	}
	writeInstructionFile(t, repository, ".gator/agents.json", `{
  "version": 1,
  "roles": [
    {"name": "reviewer", "description": "one\ntwo", "kind": "readonly", "instructions": "review"}
  ]
}`)
	if _, err := LoadRoles(repository); err == nil || !strings.Contains(err.Error(), "one-line description") {
		t.Fatalf("multiline role description error = %v", err)
	}
	writeInstructionFile(t, repository, ".gator/roles/empty.md", "   ")
	writeInstructionFile(t, repository, ".gator/agents.json", `{
  "version": 1,
  "roles": [
    {"name": "reviewer", "description": "review changes", "kind": "readonly", "file": ".gator/roles/empty.md"}
  ]
}`)
	if _, err := LoadRoles(repository); err == nil || !strings.Contains(err.Error(), "empty instructions") {
		t.Fatalf("empty role instructions error = %v", err)
	}
}

func writeInstructionFile(t *testing.T, repository, relative, contents string) {
	t.Helper()
	path := filepath.Join(repository, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create %s directory: %v", relative, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", relative, err)
	}
}
