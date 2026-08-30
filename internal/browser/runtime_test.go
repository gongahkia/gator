package browser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeFilesCurrentRequiresExactEmbeddedControllerFiles(t *testing.T) {
	directory := t.TempDir()
	for _, file := range []struct {
		name     string
		contents []byte
	}{
		{"package.json", runtimePackageJSON},
		{"package-lock.json", runtimePackageLock},
		{"driver.mjs", runtimeDriver},
	} {
		if err := os.WriteFile(filepath.Join(directory, file.name), file.contents, 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", file.name, err)
		}
	}
	if !runtimeFilesCurrent(directory) {
		t.Fatal("exact embedded runtime files were not accepted")
	}
	if err := os.WriteFile(filepath.Join(directory, "driver.mjs"), []byte("stale"), 0o600); err != nil {
		t.Fatalf("WriteFile stale driver: %v", err)
	}
	if runtimeFilesCurrent(directory) {
		t.Fatal("stale driver was accepted")
	}
}
