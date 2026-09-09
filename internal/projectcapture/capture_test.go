package projectcapture

import (
	"github.com/gongahkia/gator/internal/instructions"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureSelectedProfileRulesAndPinnedConfiguration(t *testing.T) {
	source := t.TempDir()
	write := func(path, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(source, path)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, path), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(".gator/agents.json", `{"version":1,"profiles":[{"name":"implementer","file":".gator/profiles/implementer.md"}]}`)
	write(".gator/profiles/implementer.md", "Keep amber constraints.")
	write(".gator/rules.json", `{"version":1,"rules":[{"id":"scoped","paths":["a.txt"],"file":".gator/rules/a.md"}]}`)
	write(".gator/rules/a.md", "Scoped guidance.")
	write(".gator/credentials.json", `{"secret":"do not capture"}`)
	bundle, err := Capture(source, []string{"a.txt"}, "implementer", nil)
	if err != nil {
		t.Fatal(err)
	}
	write(".gator/profiles/implementer.md", "Mutable replacement.")
	target := t.TempDir()
	if err := bundle.Install(target); err != nil {
		t.Fatal(err)
	}
	set, err := instructions.LoadWithProfile(target, []string{"a.txt"}, "implementer")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(set.Content, "amber") || !strings.Contains(set.Content, "Scoped guidance") {
		t.Fatal(set.Content)
	}
	if _, err := os.Stat(filepath.Join(target, ".gator/credentials.json")); !os.IsNotExist(err) {
		t.Fatal("credential file captured")
	}
	bundle.Files[0].Data = []byte("tamper")
	if err := bundle.Install(t.TempDir()); err == nil {
		t.Fatal("changed configuration installed")
	}
}
