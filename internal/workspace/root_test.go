package workspace

import (
	"crypto/sha256"
	"encoding/hex"
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
	canonical, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatalf("resolve temporary directory: %v", err)
	}
	if resolved != filepath.Join(canonical, "pkg", "main.go") {
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

func TestRootSetSeparatesPrimaryAndAdditionalRoots(t *testing.T) {
	primaryPath := t.TempDir()
	externalPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(primaryPath, "inside.txt"), []byte("primary"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(externalPath, "outside.txt"), []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	primary, err := Open(primaryPath)
	if err != nil {
		t.Fatal(err)
	}
	additional, err := OpenAdditionalRoots([]string{externalPath, externalPath}, 2)
	if err != nil {
		t.Fatal(err)
	}
	roots := NewRootSet(primary, additional)
	inside, err := roots.ResolveFile("inside.txt")
	if err != nil || roots.DisplayPath(inside) != "inside.txt" || !roots.IsPrimary(inside) {
		t.Fatalf("primary resolution path=%q err=%v", inside, err)
	}
	externalFile := filepath.Join(additional[0].Path(), "outside.txt")
	outside, err := roots.ResolveFile(externalFile)
	if err != nil || roots.DisplayPath(outside) != externalFile || roots.IsPrimary(outside) {
		t.Fatalf("external resolution path=%q err=%v", outside, err)
	}
	if _, err := roots.ResolveFile(primary.Path()); err == nil {
		t.Fatal("absolute primary path was accepted as an additional-root path")
	}
	if _, err := roots.ResolveFile(filepath.Join(t.TempDir(), "secret.txt")); err == nil {
		t.Fatal("outside absolute path was accepted")
	}
}

func TestNamedRootSetUsesStableVirtualPaths(t *testing.T) {
	primaryPath := t.TempDir()
	sourcePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(primaryPath, "report.md"), []byte("output"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "notes.txt"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	primary, err := Open(primaryPath)
	if err != nil {
		t.Fatal(err)
	}
	source, err := Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	roots, err := NewNamedRootSet(primary, nil, []NamedRoot{{Name: "source", Root: source}, {Name: "output", Root: primary}})
	if err != nil {
		t.Fatal(err)
	}

	resolvedSource, err := roots.ResolveFile("source/notes.txt")
	if err != nil || roots.DisplayPath(resolvedSource) != "source/notes.txt" || roots.IsPrimary(resolvedSource) {
		t.Fatalf("source resolution = %q, %v", resolvedSource, err)
	}
	resolvedOutput, err := roots.ResolveFile("output/report.md")
	if err != nil || roots.DisplayPath(resolvedOutput) != "output/report.md" || !roots.IsPrimary(resolvedOutput) {
		t.Fatalf("output resolution = %q, %v", resolvedOutput, err)
	}
	resolvedDirectory, err := roots.ResolveDirectory("source")
	if err != nil || resolvedDirectory != source.Path() {
		t.Fatalf("source directory resolution = %q, %v", resolvedDirectory, err)
	}
	if _, err := roots.ResolveFile("source"); err == nil {
		t.Fatal("named directory was resolved as a file")
	}
	if got := roots.Paths(); len(got) != 2 || got[0] != primary.Path() || got[1] != source.Path() {
		t.Fatalf("named root paths = %#v", got)
	}
}

func TestNamedRootSetRejectsAmbiguousNames(t *testing.T) {
	root, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, named := range [][]NamedRoot{
		{{Name: "Source", Root: root}},
		{{Name: "source/path", Root: root}},
		{{Name: "source", Root: root}, {Name: "source", Root: root}},
		{{Name: "source", Root: Root{}}},
	} {
		if _, err := NewNamedRootSet(root, nil, named); err == nil {
			t.Fatalf("invalid named roots were accepted: %#v", named)
		}
	}
}

func TestRootSetDescriptorWalkSkipsOutsideSymlinks(t *testing.T) {
	primaryPath := t.TempDir()
	externalPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(primaryPath, "inside.txt"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(externalPath, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalPath, filepath.Join(primaryPath, "outside")); err != nil {
		t.Fatal(err)
	}
	root, err := Open(primaryPath)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	if err := NewRootSet(root, nil).WalkRegularFiles(".", nil, func(file FileRef) error {
		paths = append(paths, file.Path)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "inside.txt" {
		t.Fatalf("walked paths = %#v", paths)
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

func TestRootSHA256RegularFile(t *testing.T) {
	directory := t.TempDir()
	contents := []byte("fixture contents\n")
	if err := os.WriteFile(filepath.Join(directory, "fixture.txt"), contents, 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := root.SHA256RegularFile("fixture.txt", 1024)
	if err != nil {
		t.Fatalf("hash regular file: %v", err)
	}
	expected := sha256.Sum256(contents)
	if hash != hex.EncodeToString(expected[:]) {
		t.Fatalf("hash = %q, want %x", hash, expected)
	}
	if _, err := root.SHA256RegularFile("fixture.txt", 2); err == nil {
		t.Fatal("oversized hash succeeded")
	}
}
