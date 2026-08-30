package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const hiddenTest = `package parser

import "testing"

func TestHiddenParseListRejectsEmptyItems(t *testing.T) {
	for _, input := range []string{"", "alpha,,beta", "alpha, ,beta"} {
		if _, err := ParseList(input); err == nil {
			t.Fatalf("ParseList(%q) accepted an empty item", input)
		}
	}
}
`

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: score WORKTREE")
		os.Exit(2)
	}
	path := filepath.Join(os.Args[1], "parser", "zz_gator_eval_hidden_test.go")
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
