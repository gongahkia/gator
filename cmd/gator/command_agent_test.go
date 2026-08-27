package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentCommandListsProjectRolesWithoutActivatingThem(t *testing.T) {
	repository := t.TempDir()
	command := exec.Command("git", "init", "--quiet")
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("initialize repository: %v: %s", err, output)
	}
	if err := os.MkdirAll(filepath.Join(repository, ".gator"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, ".gator", "agents.json"), []byte(`{
  "version": 1,
  "roles": [
    {"name":"writer","description":"focused implementation","kind":"writer","instructions":"write only the assigned area","policy":{"network":"deny","omit":["browser"]}},
    {"name":"reviewer","description":"independent review","kind":"readonly","instructions":"report evidence"}
  ]
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repository); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })

	var output bytes.Buffer
	if err := agentCommand([]string{"list"}, &output); err != nil {
		t.Fatalf("list agent roles: %v", err)
	}
	value := output.String()
	for _, expected := range []string{"Project agents:", "Roles (can only narrow inherited policy):", "reviewer  readonly  independent review  policy=inherit", "writer  writer  focused implementation  policy=network:deny;omit:browser"} {
		if !strings.Contains(value, expected) {
			t.Fatalf("agent list missing %q: %q", expected, value)
		}
	}
}
