package snapshot

import (
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
	if z.SourcePath != a {
		t.Fatal("unexpected origin")
	}
}
