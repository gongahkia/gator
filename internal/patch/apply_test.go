package patch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyFixtures(t *testing.T) {
	tests := []struct {
		name  string
		check func(*testing.T, string)
	}{
		{name: "single-hunk", check: checkFile("file.txt", "one\nTWO\nthree\nfour\nfive\nsix\nseven\neight\n")},
		{name: "multi-hunk", check: checkFile("file.txt", "one\nTWO\nthree\nfour\nfive\nsix\nSEVEN\neight\n")},
		{name: "create-new", check: checkFile("new.txt", "hello\nworld\n")},
		{name: "delete-file", check: checkMissing("delete.txt")},
		{name: "multi-file", check: func(t *testing.T, dir string) {
			checkFile("file.txt", "one\nTWO\nthree\nfour\nfive\nsix\nseven\neight\n")(t, dir)
			checkFile("extra.txt", "extra\n")(t, dir)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := baseRepo(t)
			if err := Apply(dir, fixture(t, tt.name)); err != nil {
				t.Fatalf("apply: %v", err)
			}
			tt.check(t, dir)
		})
	}
}

func TestApplyRejectsContextMismatch(t *testing.T) {
	dir := baseRepo(t)
	before := readFile(t, dir, "file.txt")
	err := Apply(dir, fixture(t, "context-mismatch"))
	if err == nil {
		t.Fatal("expected context mismatch")
	}
	if got := readFile(t, dir, "file.txt"); got != before {
		t.Fatalf("file changed after failed apply:\n%s", got)
	}
}

func TestApplyRejectsPathEscape(t *testing.T) {
	dir := baseRepo(t)
	err := Apply(dir, fixture(t, "path-escape"))
	if err == nil || !strings.Contains(err.Error(), "invalid patch path") {
		t.Fatalf("expected path escape error, got %v", err)
	}
}

func TestApplyRollsBackPartialFailure(t *testing.T) {
	dir := baseRepo(t)
	writeFile(t, dir, "other.txt", "alpha\nbeta\ngamma\n")
	diff := strings.Join([]string{
		"--- a/file.txt",
		"+++ b/file.txt",
		"@@ -1,3 +1,3 @@",
		" one",
		"-two",
		"+TWO",
		" three",
		"--- a/other.txt",
		"+++ b/other.txt",
		"@@ -1,3 +1,3 @@",
		" alpha",
		"-missing",
		"+MISSING",
		" gamma",
		"",
	}, "\n")
	beforeFile := readFile(t, dir, "file.txt")
	beforeOther := readFile(t, dir, "other.txt")
	if err := Apply(dir, diff); err == nil {
		t.Fatal("expected partial failure")
	}
	if got := readFile(t, dir, "file.txt"); got != beforeFile {
		t.Fatalf("file.txt not rolled back:\n%s", got)
	}
	if got := readFile(t, dir, "other.txt"); got != beforeOther {
		t.Fatalf("other.txt not rolled back:\n%s", got)
	}
}

func baseRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\n")
	writeFile(t, dir, "delete.txt", "remove\nme\n")
	return dir
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "patches", name+".diff"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(b)
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func readFile(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return string(b)
}

func checkFile(rel, want string) func(*testing.T, string) {
	return func(t *testing.T, dir string) {
		t.Helper()
		if got := readFile(t, dir, rel); got != want {
			t.Fatalf("%s got:\n%s\nwant:\n%s", rel, got, want)
		}
	}
}

func checkMissing(rel string) func(*testing.T, string) {
	return func(t *testing.T, dir string) {
		t.Helper()
		if _, err := os.Stat(filepath.Join(dir, rel)); !os.IsNotExist(err) {
			t.Fatalf("%s still exists or stat failed: %v", rel, err)
		}
	}
}
