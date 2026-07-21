package testrepo

import (
	"os"
	"path/filepath"
	"testing"
)

type Repository struct {
	Root string
}

func New(t testing.TB) Repository {
	t.Helper()
	return Repository{Root: t.TempDir()}
}

func (r Repository) Write(t testing.TB, rel, content string) string {
	t.Helper()
	path := filepath.Join(r.Root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	return path
}

func (r Repository) Mkdir(t testing.TB, rel string) string {
	t.Helper()
	path := filepath.Join(r.Root, rel)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	return path
}
