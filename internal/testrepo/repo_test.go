package testrepo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewFixtureCopiesFixtureTree(t *testing.T) {
	repo := NewFixture(t, "basic")
	data, err := os.ReadFile(filepath.Join(repo.Root, "src", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "package basic\n" {
		t.Fatalf("fixture = %q", data)
	}
}

func TestNewGitCreatesCommitReadyRepository(t *testing.T) {
	repo := NewGit(t)
	repo.Write(t, "README.md", "fixture\n")
	repo.Commit(t, "fixture")
	if got := strings.TrimSpace(repo.Git(t, "rev-parse", "--is-inside-work-tree")); got != "true" {
		t.Fatalf("inside work tree = %q", got)
	}
	if got := strings.TrimSpace(repo.Git(t, "log", "-1", "--format=%s")); got != "fixture" {
		t.Fatalf("commit subject = %q", got)
	}
}

func TestFixturePathRejectsTraversal(t *testing.T) {
	if _, err := fixturePath("../outside"); err == nil {
		t.Fatal("expected traversal rejection")
	}
}

func TestResolveRejectsTraversal(t *testing.T) {
	if _, err := New(t).resolve("../outside"); err == nil {
		t.Fatal("expected traversal rejection")
	}
}
