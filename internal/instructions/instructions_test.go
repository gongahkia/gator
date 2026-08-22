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
