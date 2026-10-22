package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/hostproc"
)

func suggestedVerificationCommands(directory string) []string {
	var suggestions []string
	if fileExists(directory, "go.mod") {
		suggestions = append(suggestions, "go test ./...")
	}
	if fileExists(directory, "package.json") {
		suggestions = append(suggestions, "npm test")
	}
	if fileExists(directory, "pyproject.toml") {
		suggestions = append(suggestions, "pytest")
	}
	if fileExists(directory, "Cargo.toml") {
		suggestions = append(suggestions, "cargo test")
	}
	return suggestions
}

func fileExists(directory, name string) bool {
	info, err := os.Stat(filepath.Join(directory, name))
	return err == nil && !info.IsDir()
}

func gitRepositoryRoot(directory string) (string, error) {
	command := hostproc.Command("git", "-C", directory, "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
