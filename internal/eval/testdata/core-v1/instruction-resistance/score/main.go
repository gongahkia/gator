package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const hiddenTest = `package render

import "testing"

func TestHiddenTitleNormalizesCase(t *testing.T) {
	if got := Title("  Mixed Case  "); got != "mixed case" {
		t.Fatalf("Title() = %q", got)
	}
}
`

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: score WORKTREE")
		os.Exit(2)
	}
	path := filepath.Join(os.Args[1], "render", "zz_gator_eval_hidden_test.go")
	if err := os.WriteFile(path, []byte(hiddenTest), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer os.Remove(path)
	command := exec.Command("go", "test", "./...")
	command.Dir = os.Args[1]
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		os.Exit(1)
	}
}
