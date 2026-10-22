package lifecycle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishProfileNeverReplacesDirectory(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"stage/profile", "existing"} {
		if err := os.MkdirAll(filepath.Join(directory, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, "stage/profile/proof"), []byte("saved"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := publishProfile(root, filepath.Join("stage", "profile"), "existing"); err == nil {
		t.Fatal("replaced an existing empty directory")
	}
	if err := publishProfile(root, filepath.Join("stage", "profile"), "fresh"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "fresh", "proof"))
	if err != nil || string(data) != "saved" {
		t.Fatalf("published profile: %q, %v", data, err)
	}
}
