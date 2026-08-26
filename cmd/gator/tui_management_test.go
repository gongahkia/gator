package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/extension"
	"github.com/gongahkia/gator/internal/journal"
)

func TestTUIManagementPersistsValidatedPolicyAndDefaults(t *testing.T) {
	root := t.TempDir()
	settings, err := config.New(filepath.Join(root, "config"))
	if err != nil {
		t.Fatal(err)
	}
	extensions, err := extension.NewStore(filepath.Join(root, "data"))
	if err != nil {
		t.Fatal(err)
	}
	backend := &tuiManagementBackend{repository: root, stateDir: filepath.Join(root, "state"), settings: settings, extensions: extensions}
	if err := backend.SetExecutionPolicy("off", "allow"); err != nil {
		t.Fatal(err)
	}
	if err := backend.SetDefaults("openai", "gpt-test"); err != nil {
		t.Fatal(err)
	}
	saved, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Execution.Mode != "off" || saved.Execution.Network != "allow" {
		t.Fatalf("execution policy = %#v", saved.Execution)
	}
	if saved.Defaults.Provider != "openai" || saved.Defaults.Model != "gpt-test" {
		t.Fatalf("defaults = %#v", saved.Defaults)
	}
	if err := backend.SetExecutionPolicy("invalid", "allow"); err == nil {
		t.Fatal("invalid sandbox mode was accepted")
	}
	if err := backend.SetDefaults("bad\nprovider", "model"); err == nil {
		t.Fatal("newline-bearing provider was accepted")
	}
}

func TestTUIManagementExportsPrivatelyChecksAndAppliesRetainedPatch(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	if err := os.Mkdir(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	runManagementGit(t, repository, "init", "--quiet")
	if err := os.WriteFile(filepath.Join(repository, "message.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runManagementGit(t, repository, "add", "message.txt")
	runManagementGit(t, repository, "-c", "user.name=Gator Test", "-c", "user.email=gator@example.invalid", "commit", "--quiet", "-m", "base")
	base := strings.TrimSpace(runManagementGit(t, repository, "rev-parse", "HEAD"))
	worktree := filepath.Join(root, "worktree")
	runManagementGit(t, repository, "worktree", "add", "--quiet", "--detach", worktree, base)
	if err := os.WriteFile(filepath.Join(worktree, "message.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stateDir := filepath.Join(root, "state")
	entry, record, err := journal.Open(repository, "run-1", worktree, stateDir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := entry.SaveSession(journal.Session{
		Version: 2, Repository: repository, WorktreePath: worktree, BaseCommit: base,
		Provider: "openai", Model: "test-model", Task: "change the message",
	}); err != nil {
		t.Fatal(err)
	}
	if err := entry.Finish("completed", "done", time.Now()); err != nil {
		t.Fatal(err)
	}
	settings, err := config.New(filepath.Join(root, "config"))
	if err != nil {
		t.Fatal(err)
	}
	extensions, err := extension.NewStore(filepath.Join(root, "data"))
	if err != nil {
		t.Fatal(err)
	}
	backend := &tuiManagementBackend{repository: repository, stateDir: stateDir, settings: settings, extensions: extensions}

	patchPath, err := backend.ExportArtifact("patch", record.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(patchPath, filepath.Join(stateDir, "gator", "exports")+string(os.PathSeparator)) {
		t.Fatalf("patch export escaped private state: %s", patchPath)
	}
	if info, err := os.Stat(patchPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("patch export mode = %v, err = %v", info, err)
	}
	transcriptPath, err := backend.ExportArtifact("transcript", record.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	transcript, err := os.ReadFile(transcriptPath)
	if err != nil || !strings.Contains(string(transcript), "Gator transcript") {
		t.Fatalf("transcript export = %q, err = %v", transcript, err)
	}
	if bytes, err := backend.CheckPatch(record.StatePath); err != nil || bytes == 0 {
		t.Fatalf("check patch = %d, %v", bytes, err)
	}
	otherRepository := filepath.Join(root, "other-repository")
	if err := os.Mkdir(otherRepository, 0o755); err != nil {
		t.Fatal(err)
	}
	otherBackend := *backend
	otherBackend.repository = otherRepository
	if _, err := otherBackend.CheckPatch(record.StatePath); err == nil || !strings.Contains(err.Error(), "not managed") {
		t.Fatalf("cross-repository retained run check = %v", err)
	}
	if bytes, err := backend.ApplyPatch(record.StatePath); err != nil || bytes == 0 {
		t.Fatalf("apply patch = %d, %v", bytes, err)
	}
	contents, err := os.ReadFile(filepath.Join(repository, "message.txt"))
	if err != nil || string(contents) != "after\n" {
		t.Fatalf("applied contents = %q, err = %v", contents, err)
	}
}

func runManagementGit(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}
