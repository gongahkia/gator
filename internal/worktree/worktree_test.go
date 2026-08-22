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

func TestCreateWithOptionsUsesResolvedBaseReference(t *testing.T) {
	repository := initializedRepository(t)
	first, err := revisionForTest(repository, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("# second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "README.md")
	runGit(t, repository, "-c", "user.name=Gator Test", "-c", "user.email=gator@example.invalid", "commit", "--quiet", "-m", "second")

	created, err := CreateWithOptions(context.Background(), repository, "run-base-001", Options{BaseRef: first})
	if err != nil {
		t.Fatalf("create at explicit base: %v", err)
	}
	if created.BaseCommit != first {
		t.Fatalf("base = %s, want %s", created.BaseCommit, first)
	}
	contents, err := os.ReadFile(filepath.Join(created.Path, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "# fixture\n" {
		t.Fatalf("README = %q, want first revision", contents)
	}
}

func TestCreateWithOptionsCopiesExplicitIgnoredFiles(t *testing.T) {
	repository := initializedRepository(t)
	if err := os.WriteFile(filepath.Join(repository, ".gitignore"), []byte(".env\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repository, ".gator"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, ".gator", "worktreeinclude"), []byte("# explicit setup\n.env\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", ".gitignore", ".gator/worktreeinclude")
	runGit(t, repository, "-c", "user.name=Gator Test", "-c", "user.email=gator@example.invalid", "commit", "--quiet", "-m", "setup")
	if err := os.WriteFile(filepath.Join(repository, ".env"), []byte("SECRET=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	created, err := CreateWithOptions(context.Background(), repository, "run-setup-001", Options{CopyIgnoredFiles: true})
	if err != nil {
		t.Fatalf("create with ignored setup: %v", err)
	}
	contents, err := os.ReadFile(filepath.Join(created.Path, ".env"))
	if err != nil {
		t.Fatalf("read copied ignored file: %v", err)
	}
	if string(contents) != "SECRET=value\n" {
		t.Fatalf("copied contents = %q", contents)
	}
}

func TestCreateWithOptionsRejectsTrackedOrUnignoredSetup(t *testing.T) {
	repository := initializedRepository(t)
	if err := os.MkdirAll(filepath.Join(repository, ".gator"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, ".gator", "worktreeinclude"), []byte("README.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", ".gator/worktreeinclude")
	runGit(t, repository, "-c", "user.name=Gator Test", "-c", "user.email=gator@example.invalid", "commit", "--quiet", "-m", "setup")

	if _, err := CreateWithOptions(context.Background(), repository, "run-setup-reject-001", Options{CopyIgnoredFiles: true}); err == nil || !strings.Contains(err.Error(), "not ignored") {
		t.Fatalf("create error = %v, want unignored setup error", err)
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

func revisionForTest(directory, reference string) (string, error) {
	command := exec.Command("git", "rev-parse", reference)
	command.Dir = directory
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
