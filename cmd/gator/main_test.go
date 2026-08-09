package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHeadlessCLISurface(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-q")
	git(t, root, "config", "user.email", "test@example.com")
	git(t, root, "config", "user.name", "Gator Test")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "README.md")
	git(t, root, "commit", "-qm", "fixture")
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	var output, stderr bytes.Buffer
	if err := run(context.Background(), []string{"policy", "init"}, nil, &output, &stderr); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := run(context.Background(), []string{"run", "--provider", "codex", "--no-diff", "headless plan"}, nil, &output, &stderr); err != nil {
		t.Fatal(err)
	}
	fields := strings.Split(strings.TrimSpace(output.String()), "\t")
	if len(fields) < 2 || !strings.HasPrefix(fields[0], "run-") || fields[1] != "planned" {
		t.Fatalf("unexpected run output: %q", output.String())
	}
	runID := fields[0]
	output.Reset()
	if err := run(context.Background(), []string{"events", runID}, nil, &output, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "run.planned") {
		t.Fatalf("events did not expose dry run: %q", output.String())
	}
	output.Reset()
	if err := run(context.Background(), []string{"health"}, nil, &output, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "workspace\tready") || !strings.Contains(output.String(), "policy\tready") {
		t.Fatalf("health was incomplete: %q", output.String())
	}
}

func TestHelpDoesNotRequireWorkspace(t *testing.T) {
	var output, stderr bytes.Buffer
	if err := run(context.Background(), []string{"help"}, nil, &output, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "gator run") {
		t.Fatalf("help was incomplete: %q", output.String())
	}
}

func git(t *testing.T, cwd string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = cwd
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}
