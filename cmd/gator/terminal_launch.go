package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/charmbracelet/x/term"
)

var terminalIdentifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)

func validIdentifier(value string) bool { return terminalIdentifier.MatchString(value) }

// The Go executable also serves scripts and workers. Interactive callers enter
// the same OpenTUI frontend as the public launcher, never a second renderer.
func launchTerminal(args []string) error {
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		return errors.New("interactive mode needs a terminal; use gator --help, gator work \"task\", or gator work-rpc for scripts")
	}
	var candidates []string
	if launcher := os.Getenv("GATOR_TERMINAL_LAUNCHER"); launcher != "" {
		candidates = append(candidates, launcher)
	}
	if executable, err := os.Executable(); err == nil {
		root := filepath.Dir(filepath.Dir(executable))
		candidates = append(candidates, filepath.Join(root, "bin", "gator"), filepath.Join(root, "ui", "launcher", "gator.mjs"))
	}
	if root, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(root, "ui", "launcher", "gator.mjs"))
	}
	for _, launcher := range candidates {
		if info, err := os.Stat(launcher); err == nil && !info.IsDir() {
			command := exec.Command("bun", append([]string{launcher}, args...)...)
			command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
			if err := command.Run(); err != nil {
				return fmt.Errorf("terminal frontend: %w", err)
			}
			return nil
		}
	}
	return errors.New("terminal frontend is missing; run make build in a checkout or install the complete Gator terminal package")
}
