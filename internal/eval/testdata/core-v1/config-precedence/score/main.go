package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const hiddenTest = `package config

import "testing"

func TestHiddenLookupEnvironmentWins(t *testing.T) {
	got := Lookup(map[string]string{"theme": "light"}, map[string]string{"theme": "dark"}, "theme", "system")
	if got != "dark" {
		t.Fatalf("Lookup() = %q, want environment value", got)
	}
}
`

func main() {
	arguments := os.Args[1:]
	if len(arguments) == 2 && arguments[0] == "--" {
		arguments = arguments[1:]
	}
	if len(arguments) != 1 {
		fmt.Fprintln(os.Stderr, "usage: score WORKTREE")
		os.Exit(2)
	}
	path := filepath.Join(arguments[0], "config", "zz_gator_eval_hidden_test.go")
	if err := os.WriteFile(path, []byte(hiddenTest), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer os.Remove(path)
	command := exec.Command("go", "test", "./...")
	command.Dir = arguments[0]
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		os.Exit(1)
	}
}
