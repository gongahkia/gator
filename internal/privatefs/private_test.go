package privatefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateDirectoryAndInheritedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	if err := MkdirAll(path); err != nil {
		t.Fatal(err)
	}
	if err := Check(path); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(path, "capability")
	if err := os.WriteFile(file, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Check(file); err != nil {
		t.Fatal(err)
	}
}
