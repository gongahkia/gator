package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateIsolatedWorktree(t *testing.T) {
	repository := initializedRepository(t)
	worktree, err := Create(context.Background(), repository, "run_feature_001")
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	canonicalRepository, err := filepath.EvalSymlinks(repository)
	if err != nil {
		t.Fatalf("resolve repository: %v", err)
	}
	if worktree.Repository != canonicalRepository {
		t.Fatalf("repository = %q, want %q", worktree.Repository, canonicalRepository)
	}
	if worktree.Path == repository {
		t.Fatal("worktree reused active repository path")
	}
	if worktree.BaseCommit == "" {
		t.Fatal("created worktree did not record its base commit")
	}
	if _, err := os.Stat(filepath.Join(worktree.Path, "README.md")); err != nil {
		t.Fatalf("created worktree did not contain committed file: %v", err)
	}
	if !strings.Contains(worktree.Path, "repository-gator-runs") {
		t.Fatalf("worktree path = %q", worktree.Path)
	}
}

func TestCreateRejectsInvalidRunID(t *testing.T) {
	_, err := Create(context.Background(), t.TempDir(), "../escape")
	if err == nil || !strings.Contains(err.Error(), "invalid run id") {
		t.Fatalf("invalid id error = %v", err)
	}
}

func TestOpenExistingWorktree(t *testing.T) {
	repository := initializedRepository(t)
	created, err := Create(context.Background(), repository, "run_resume_001")
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	opened, err := OpenExisting(context.Background(), repository, created.Path, "run_resume_002")
	if err != nil {
		t.Fatalf("open existing worktree: %v", err)
	}
	if opened.Path != created.Path || opened.Repository != repository {
		t.Fatalf("opened worktree = %#v", opened)
	}
}

func initializedRepository(t *testing.T) string {
	t.Helper()
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "init", "--quiet")
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "README.md")
	runGit(t, repository, "-c", "user.name=Gator Test", "-c", "user.email=gator@example.invalid", "commit", "--quiet", "-m", "fixture")
	return repository
}

func runGit(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
}
