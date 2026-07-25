package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectRejectsExecutableAndRequiresManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("# test"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skill.json"), []byte(`{"id":"test","version":"1.0.0","name":"Test","tools":[{"id":"read","kind":"http"}]}`), 0o640); err != nil {
		t.Fatal(err)
	}
	pkg, findings, err := inspect(root)
	if err != nil || pkg.Digest == "" || findings["status"] != "pass" {
		t.Fatalf("pkg=%#v findings=%#v err=%v", pkg, findings, err)
	}
	if err := os.WriteFile(filepath.Join(root, "run.sh"), []byte("#!/bin/sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := inspect(root); err == nil {
		t.Fatal("executable bundle accepted")
	}
}
