// Package state resolves Gator's local state root. It deliberately owns only
// the process-wide location policy, not any execution or history model.
package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveDir returns the local state root. Callers retain ownership of the
// subdirectory and data model they create below it.
func ResolveDir(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return absoluteDirectory(override)
	}
	if configured := os.Getenv("GATOR_STATE_DIR"); strings.TrimSpace(configured) != "" {
		return absoluteDirectory(configured)
	}
	if configured := os.Getenv("XDG_STATE_HOME"); strings.TrimSpace(configured) != "" {
		return absoluteDirectory(configured)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user state directory: %w", err)
	}
	return filepath.Join(home, ".local", "state"), nil
}

func absoluteDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve state directory: %w", err)
	}
	return abs, nil
}
