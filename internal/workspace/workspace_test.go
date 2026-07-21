package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	pawruntime "github.com/gongahkia/paw/internal/runtime"
)

type runnerFunc func(context.Context, pawruntime.Command) (pawruntime.Result, error)

func (f runnerFunc) Run(ctx context.Context, command pawruntime.Command) (pawruntime.Result, error) {
	return f(ctx, command)
}

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

func TestResolvePathRejectsDanglingEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "missing")
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := ResolvePath(root, "link/secret.txt"); !errors.Is(err, ErrPathEscapesRoot) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
}

func TestCanonicalRootResolvesWorkspaceSymlink(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "workspace")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	got, err := CanonicalRoot(alias)
	if err != nil {
		t.Fatal(err)
	}
	want, err := CanonicalRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("canonical root = %q, want %q", got, want)
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

func TestInspectUsesProcessRunnerForGitDetection(t *testing.T) {
	root := t.TempDir()
	var got pawruntime.Command
	manifest, err := inspect(root, runnerFunc(func(_ context.Context, command pawruntime.Command) (pawruntime.Result, error) {
		got = command
		return pawruntime.Result{Stdout: []byte(root + "\n")}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.IsGit || manifest.GitRoot == "" {
		t.Fatalf("manifest = %#v", manifest)
	}
	if got.Path != "git" || got.Dir == "" || len(got.Args) != 2 || got.Args[0] != "rev-parse" {
		t.Fatalf("command = %#v", got)
	}
}

func TestInspectReturnsUnexpectedRunnerError(t *testing.T) {
	_, err := inspect(t.TempDir(), runnerFunc(func(context.Context, pawruntime.Command) (pawruntime.Result, error) {
		return pawruntime.Result{}, errors.New("runner failed")
	}))
	if err == nil || err.Error() != "inspect git workspace: runner failed" {
		t.Fatalf("inspect error = %v", err)
	}
}
