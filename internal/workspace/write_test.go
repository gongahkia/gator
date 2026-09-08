package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteRegularFileAtomicCreatesAndReplacesPrivateFile(t *testing.T) {
	root, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := root.WriteRegularFileAtomic("reports/summary.md", []byte("first"), 32); err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := root.WriteRegularFileAtomic("reports/summary.md", []byte("second"), 32); err != nil {
		t.Fatalf("replace file: %v", err)
	}
	contents, err := root.ReadRegularFile("reports/summary.md", 32)
	if err != nil || string(contents) != "second" {
		t.Fatalf("contents = %q, %v", contents, err)
	}
	info, err := os.Stat(filepath.Join(root.Path(), "reports", "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file permissions = %o", info.Mode().Perm())
	}
}

func TestWriteRegularFileAtomicRejectsInvalidOrOversizedContent(t *testing.T) {
	root, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../secret", "/tmp/secret", "."} {
		if err := root.WriteRegularFileAtomic(path, []byte("x"), 1); err == nil {
			t.Fatalf("invalid path %q was accepted", path)
		}
	}
	if err := root.WriteRegularFileAtomic("large.txt", []byte("too large"), 2); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("oversized error = %v", err)
	}
}

func TestWriteRegularFileAtomicDoesNotFollowSymlinks(t *testing.T) {
	workspacePath := t.TempDir()
	externalPath := t.TempDir()
	secretPath := filepath.Join(externalPath, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secretPath, filepath.Join(workspacePath, "target.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalPath, filepath.Join(workspacePath, "outside")); err != nil {
		t.Fatal(err)
	}
	root, err := Open(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := root.WriteRegularFileAtomic("target.txt", []byte("changed"), 32); err == nil {
		t.Fatal("symlink target was accepted")
	}
	if err := root.WriteRegularFileAtomic("outside/new.txt", []byte("changed"), 32); err == nil {
		t.Fatal("symlinked parent outside workspace was accepted")
	}
	contents, err := os.ReadFile(secretPath)
	if err != nil || string(contents) != "untouched" {
		t.Fatalf("external contents = %q, %v", contents, err)
	}
}
