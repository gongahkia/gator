package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathAllowsNestedFutureFile(t *testing.T) {
	root := t.TempDir()
	got, err := ResolvePath(root, "nested/new.txt")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	canonical, err := CanonicalRoot(root)
	if err != nil {
		t.Fatalf("canonical root: %v", err)
	}
	want := filepath.Join(canonical, "nested", "new.txt")
	if got != want {
		t.Fatalf("path = %q want %q", got, want)
	}
}

func TestResolvePathRejectsTraversal(t *testing.T) {
	if _, err := ResolvePath(t.TempDir(), "../escape"); !errors.Is(err, ErrPathEscapesRoot) {
		t.Fatalf("err = %v", err)
	}
}

func TestResolvePathRejectsEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := ResolvePath(root, "link/secret.txt"); !errors.Is(err, ErrPathEscapesRoot) {
		t.Fatalf("err = %v", err)
	}
}

func TestInspectFindsGitWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest, err := Inspect(root)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if manifest.IsGit {
		t.Fatal("empty .git directory must not be treated as a git worktree")
	}
}
