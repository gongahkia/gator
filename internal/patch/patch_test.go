package patch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportAndApplyTransfersTrackedAndUntrackedFiles(t *testing.T) {
	source, target := pairedRepositories(t)
	writeFile(t, source, "tracked.txt", "changed\n")
	writeFile(t, source, "nested/new feature.txt", "new file\n")

	exported, err := Export(context.Background(), source)
	if err != nil {
		t.Fatalf("export patch: %v", err)
	}
	if !strings.Contains(string(exported), "tracked.txt") || !strings.Contains(string(exported), "nested/new feature.txt") {
		t.Fatalf("exported patch = %s", exported)
	}
	checked, err := Check(context.Background(), source, target)
	if err != nil {
		t.Fatalf("check patch: %v", err)
	}
	if checked.Bytes != len(exported) {
		t.Fatalf("check result = %#v, want %d bytes", checked, len(exported))
	}
	applied, err := Apply(context.Background(), source, target)
	if err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if applied.Bytes != len(exported) {
		t.Fatalf("apply result = %#v, want %d bytes", applied, len(exported))
	}
	assertFile(t, target, "tracked.txt", "changed\n")
	assertFile(t, target, "nested/new feature.txt", "new file\n")
}

func TestApplyRejectsDirtyTargetBeforeCheckingPatchCompatibility(t *testing.T) {
	source, target := pairedRepositories(t)
	writeFile(t, source, "tracked.txt", "changed\n")
	writeFile(t, target, "local.txt", "keep me\n")
	if _, err := Apply(context.Background(), source, target); err == nil || !strings.Contains(err.Error(), "must be clean") {
		t.Fatalf("apply error = %v", err)
	}
	assertFile(t, target, "local.txt", "keep me\n")
	assertFile(t, target, "tracked.txt", "base\n")
}

func TestApplyRejectsIncompatibleCleanTarget(t *testing.T) {
	source, target := pairedRepositories(t)
	writeFile(t, source, "tracked.txt", "changed\n")
	writeFile(t, target, "tracked.txt", "other base\n")
	runGit(t, target, "add", "tracked.txt")
	runGit(t, target, "commit", "--quiet", "-m", "different target")
	if _, err := Check(context.Background(), source, target); err == nil || !strings.Contains(err.Error(), "not compatible") {
		t.Fatalf("check error = %v", err)
	}
	assertFile(t, target, "tracked.txt", "other base\n")
}

func pairedRepositories(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	source := filepath.Join(base, "source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "init", "--quiet")
	runGit(t, source, "config", "user.name", "Gator Test")
	runGit(t, source, "config", "user.email", "gator@example.invalid")
	writeFile(t, source, "tracked.txt", "base\n")
	runGit(t, source, "add", "tracked.txt")
	runGit(t, source, "commit", "--quiet", "-m", "base")
	target := filepath.Join(base, "target")
	runGit(t, base, "clone", "--quiet", source, target)
	runGit(t, target, "config", "user.name", "Gator Test")
	runGit(t, target, "config", "user.email", "gator@example.invalid")
	return source, target
}

func writeFile(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, root, relative, want string) {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(root, relative))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != want {
		t.Fatalf("%s = %q, want %q", relative, contents, want)
	}
}

func runGit(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
}
