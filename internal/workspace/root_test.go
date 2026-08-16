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

func TestRootReadRegularFileStaysInsideWorkspace(t *testing.T) {
	directory := t.TempDir()
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "inside.txt"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "secret.txt"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(external, "secret.txt"), filepath.Join(directory, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	root, err := Open(directory)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	contents, err := root.ReadRegularFile("inside.txt", 64)
	if err != nil || string(contents) != "inside" {
		t.Fatalf("read regular file = %q, %v", contents, err)
	}
	if _, err := root.ReadRegularFile("escape.txt", 64); err == nil {
		t.Fatal("read through outside symlink succeeded")
	}
}
