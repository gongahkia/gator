package tui

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/core"
)

func TestDashboardStartsAndQuits(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-q")
	git(t, root, "config", "user.email", "test@example.com")
	git(t, root, "config", "user.name", "Gator Test")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "README.md")
	git(t, root, "commit", "-qm", "fixture")
	engine, err := core.Open(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	var output, errors bytes.Buffer
	if err := Run(context.Background(), engine, strings.NewReader("quit\n"), &output, &errors); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "standalone terminal-native") || !strings.Contains(output.String(), "Recent runs") {
		t.Fatalf("TUI dashboard did not render useful state: %q", output.String())
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
