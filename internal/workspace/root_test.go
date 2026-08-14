package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootResolveFile(t *testing.T) {
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := Open(directory)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}

	resolved, err := root.ResolveFile("pkg/main.go")
	if err != nil {
		t.Fatalf("resolve file: %v", err)
	}
	if resolved != filepath.Join(directory, "pkg", "main.go") {
		t.Fatalf("resolved = %q", resolved)
	}
}

func TestRootRejectsEscapingPaths(t *testing.T) {
	root, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open root: %v", err)
	}

	for _, path := range []string{"../secret", "/etc/passwd", "."} {
		t.Run(path, func(t *testing.T) {
			_, err := root.ResolveFile(path)
			if err == nil || !strings.Contains(err.Error(), "path") {
				t.Fatalf("resolve %q error = %v", path, err)
			}
		})
	}
}

func TestRootRejectsSymlinkOutsideWorkspace(t *testing.T) {
	directory := t.TempDir()
	external := t.TempDir()
	if err := os.Symlink(external, filepath.Join(directory, "outside")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	root, err := Open(directory)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}

	_, err = root.ResolveFile("outside/secret.txt")
	if err == nil || !strings.Contains(err.Error(), "escapes the workspace") {
		t.Fatalf("resolve outside symlink error = %v", err)
	}
}
