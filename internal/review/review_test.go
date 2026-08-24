package review

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSeparatesCombinedStagedAndUnstagedChanges(t *testing.T) {
	repository := testRepository(t)
	write(t, repository, "alpha.txt", "one\nstaged\n")
	gitOK(t, repository, "add", "alpha.txt")
	write(t, repository, "beta.txt", "one\nworking\n")
	write(t, repository, "new.txt", "new\n")

	snapshot, err := Load(context.Background(), repository, "")
	if err != nil {
		t.Fatalf("load review: %v", err)
	}
	if got, want := snapshot.All.Stats.Files, 3; got != want {
		t.Fatalf("all files = %d, want %d: %#v", got, want, snapshot.All.Files)
	}
	if got, want := snapshot.Staged.Stats.Files, 1; got != want {
		t.Fatalf("staged files = %d, want %d: %#v", got, want, snapshot.Staged.Files)
	}
	if got, want := snapshot.Unstaged.Stats.Files, 2; got != want {
		t.Fatalf("unstaged files = %d, want %d: %#v", got, want, snapshot.Unstaged.Files)
	}
	if snapshot.Truncated {
		t.Fatal("small review unexpectedly truncated")
	}
	if !containsPath(snapshot.Unstaged.Files, "new.txt") {
		t.Fatalf("untracked file missing from working review: %#v", snapshot.Unstaged.Files)
	}
}

func TestStageAndUnstageFileStayInsideRetainedWorktree(t *testing.T) {
	repository := testRepository(t)
	write(t, repository, "alpha.txt", "one\nchanged\n")
	if err := StageFile(context.Background(), repository, "alpha.txt"); err != nil {
		t.Fatalf("stage file: %v", err)
	}
	if output := gitOutput(t, repository, "diff", "--cached", "--name-only"); strings.TrimSpace(output) != "alpha.txt" {
		t.Fatalf("staged paths = %q", output)
	}
	if err := UnstageFile(context.Background(), repository, "alpha.txt"); err != nil {
		t.Fatalf("unstage file: %v", err)
	}
	if output := gitOutput(t, repository, "diff", "--cached", "--name-only"); strings.TrimSpace(output) != "" {
		t.Fatalf("cached diff after unstage = %q", output)
	}
	if contents := read(t, repository, "alpha.txt"); contents != "one\nchanged\n" {
		t.Fatalf("working file changed while unstaging = %q", contents)
	}
	if err := StageFile(context.Background(), repository, "../outside.txt"); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("unsafe path error = %v", err)
	}
}

func TestStageAndUnstageOneHunkOnly(t *testing.T) {
	repository := testRepository(t)
	write(t, repository, "alpha.txt", strings.Join([]string{
		"one",
		"two changed",
		"three",
		"four",
		"five",
		"six",
		"seven",
		"eight",
		"nine",
		"ten changed",
		"eleven",
		"twelve",
	}, "\n")+"\n")

	snapshot, err := Load(context.Background(), repository, "")
	if err != nil {
		t.Fatalf("load review: %v", err)
	}
	file := onlyFile(t, snapshot.Unstaged)
	if len(file.Hunks) != 2 {
		t.Fatalf("working hunks = %d, want 2: %#v", len(file.Hunks), file.Hunks)
	}
	if err := StageHunk(context.Background(), repository, file.Hunks[0].ID); err != nil {
		t.Fatalf("stage first hunk: %v", err)
	}
	cached := gitOutput(t, repository, "diff", "--cached")
	working := gitOutput(t, repository, "diff")
	if !strings.Contains(cached, "two changed") || strings.Contains(cached, "ten changed") {
		t.Fatalf("cached diff did not isolate first hunk:\n%s", cached)
	}
	if strings.Contains(working, "two changed") || !strings.Contains(working, "ten changed") {
		t.Fatalf("working diff did not retain second hunk:\n%s", working)
	}

	staged, err := Load(context.Background(), repository, "")
	if err != nil {
		t.Fatalf("reload staged review: %v", err)
	}
	stagedFile := onlyFile(t, staged.Staged)
	if len(stagedFile.Hunks) != 1 {
		t.Fatalf("staged hunks = %d, want 1", len(stagedFile.Hunks))
	}
	if err := UnstageHunk(context.Background(), repository, stagedFile.Hunks[0].ID); err != nil {
		t.Fatalf("unstage selected hunk: %v", err)
	}
	if cached := gitOutput(t, repository, "diff", "--cached"); strings.TrimSpace(cached) != "" {
		t.Fatalf("cached diff after hunk unstage:\n%s", cached)
	}
	if working := gitOutput(t, repository, "diff"); !strings.Contains(working, "two changed") || !strings.Contains(working, "ten changed") {
		t.Fatalf("working diff after hunk unstage:\n%s", working)
	}
}

func TestParseChangeSetTracksLineNumbersAndRejectsUnsafePaths(t *testing.T) {
	set := parseChangeSet(strings.Join([]string{
		"diff --git a/pkg/demo.go b/pkg/demo.go",
		"--- a/pkg/demo.go",
		"+++ b/pkg/demo.go",
		"@@ -10,2 +10,3 @@ func demo() {",
		" line",
		"-removed()",
		"+added()",
		"+another()",
	}, "\n"))
	file := onlyFile(t, set)
	if file.Path != "pkg/demo.go" || len(file.Hunks) != 1 || file.Hunks[0].Lines[0].OldLine != 10 || file.Hunks[0].Lines[1].OldLine != 11 || file.Hunks[0].Lines[2].NewLine != 11 {
		t.Fatalf("parsed file = %#v", file)
	}
	if _, err := safePatchPath("../../secret"); err == nil {
		t.Fatal("unsafe patch path accepted")
	}
}

func onlyFile(t *testing.T, set ChangeSet) File {
	t.Helper()
	if len(set.Files) != 1 {
		t.Fatalf("files = %#v", set.Files)
	}
	return set.Files[0]
}

func containsPath(files []File, path string) bool {
	for _, file := range files {
		if file.Path == path {
			return true
		}
	}
	return false
}

func testRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	gitOK(t, repository, "init", "--quiet")
	gitOK(t, repository, "config", "user.name", "Gator Test")
	gitOK(t, repository, "config", "user.email", "gator@example.invalid")
	write(t, repository, "alpha.txt", "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\neleven\ntwelve\n")
	write(t, repository, "beta.txt", "one\ntwo\n")
	gitOK(t, repository, "add", ".")
	gitOK(t, repository, "commit", "--quiet", "-m", "base")
	return repository
}

func write(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, relative string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(root, relative))
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func gitOK(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
}

func gitOutput(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(arguments, " "), err)
	}
	return string(output)
}
