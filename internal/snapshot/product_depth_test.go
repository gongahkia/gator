package snapshot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSameContentRetainsDistinctOriginsAndSharedTree(t *testing.T) {
	state, a, b := t.TempDir(), t.TempDir(), t.TempDir()
	for _, dir := range []string{a, b} {
		if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("identical"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	x, err := Create(a, state, Options{})
	if err != nil {
		t.Fatal(err)
	}
	y, err := Create(b, state, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if x.ID == y.ID || x.SHA256 != y.SHA256 || x.Materialized != y.Materialized {
		t.Fatal("expected distinct captures and shared content")
	}
	z, err := Open(state, x.ID)
	if err != nil {
		t.Fatal(err)
	}
	canonicalA, err := filepath.EvalSymlinks(a)
	if err != nil {
		t.Fatal(err)
	}
	if z.SourcePath != canonicalA {
		t.Fatal("unexpected origin")
	}
}

func TestVersionOneCaptureRemainsReadableAndSharedTreeSurvivesGC(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "notes.txt"), []byte("legacy bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	current, err := Create(source, state, Options{})
	if err != nil {
		t.Fatal(err)
	}
	legacy := current
	legacy.Version = 1
	legacy.ID = current.TreeID
	legacy.TreeID = ""
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "gator", "snapshots", "manifests", legacy.ID+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	canonicalSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := Open(state, legacy.ID)
	if err != nil || opened.SourcePath != canonicalSource || opened.Materialized != current.Materialized {
		t.Fatalf("legacy open: %+v %v", opened, err)
	}
	if _, _, err := GC(state, map[string]struct{}{legacy.ID: {}}); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(opened.Materialized, "notes.txt"))
	if err != nil || string(payload) != "legacy bytes" {
		t.Fatalf("shared tree GC: %s %v", payload, err)
	}
}
