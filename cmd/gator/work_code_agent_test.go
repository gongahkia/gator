package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareCodeSnapshotRepositoryCreatesIndependentCleanGitSource(t *testing.T) {
	source, scratch := t.TempDir(), t.TempDir()
	path := filepath.Join(source, "script.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho source\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	repository, err := prepareCodeSnapshotRepository(context.Background(), source, scratch, "subagent-001")
	if err != nil {
		t.Fatal(err)
	}
	if repository == source || filepath.Dir(repository) != scratch {
		t.Fatalf("repository = %q", repository)
	}
	contents, err := os.ReadFile(filepath.Join(repository, "script.sh"))
	if err != nil || string(contents) != "#!/bin/sh\necho source\n" {
		t.Fatalf("snapshot contents = %q, %v", contents, err)
	}
	info, err := os.Stat(filepath.Join(repository, "script.sh"))
	if err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("snapshot mode = %v, %v", info, err)
	}
	command := exec.Command("git", "status", "--porcelain")
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "" {
		t.Fatalf("git status = %q, %v", output, err)
	}
	if err := os.WriteFile(filepath.Join(repository, "script.sh"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil || string(original) != "#!/bin/sh\necho source\n" {
		t.Fatalf("live source changed = %q, %v", original, err)
	}
}

func TestCopyCodeSnapshotRejectsSymlinks(t *testing.T) {
	source, destination := t.TempDir(), t.TempDir()
	if err := os.Symlink("missing", filepath.Join(source, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := copyCodeSnapshot(source, destination); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink error = %v", err)
	}
}
