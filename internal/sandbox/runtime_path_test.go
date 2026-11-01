package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimePathOnlyKeepsAlreadyReadableAbsoluteDirectories(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	runtimeRoot := filepath.Join(root, "runtime")
	secretRoot := filepath.Join(root, "private-config")
	for _, path := range []string{runtimeRoot, secretRoot} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	original := []string{"PATH=" + strings.Join([]string{secretRoot, ".", runtimeRoot, runtimeRoot}, string(filepath.ListSeparator)), "HOME=private-scratch"}
	filtered := runtimeEnvironment(original, []string{runtimeRoot})
	if filtered[0] != "PATH="+runtimeRoot || filtered[1] != original[1] || !strings.Contains(original[0], secretRoot) {
		t.Fatalf("runtime environment=%v original=%v", filtered, original)
	}
}
