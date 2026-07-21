package testrepo

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type Repository struct {
	Root string
}

func New(t testing.TB) Repository {
	t.Helper()
	return Repository{Root: t.TempDir()}
}

func NewFixture(t testing.TB, name string) Repository {
	t.Helper()
	repo := New(t)
	fixture, err := fixturePath(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := copyTree(fixture, repo.Root); err != nil {
		t.Fatal(err)
	}
	return repo
}

func NewGit(t testing.TB) Repository {
	t.Helper()
	repo := New(t)
	repo.Git(t, "init", "--quiet")
	repo.Git(t, "config", "user.name", "paw test")
	repo.Git(t, "config", "user.email", "paw@example.test")
	return repo
}

func (r Repository) Write(t testing.TB, rel, content string) string {
	t.Helper()
	path, err := r.resolve(rel)
	if err != nil {
		t.Fatal(err)
	}
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
	path, err := r.resolve(rel)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	return path
}

func (r Repository) Git(t testing.TB, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.Root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func (r Repository) Commit(t testing.TB, message string) {
	t.Helper()
	r.Git(t, "add", "--all")
	r.Git(t, "commit", "--quiet", "-m", message)
}

func (r Repository) resolve(rel string) (string, error) {
	if strings.TrimSpace(rel) == "" || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid repository path %q", rel)
	}
	clean := filepath.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid repository path %q", rel)
	}
	return filepath.Join(r.Root, clean), nil
}

func fixturePath(name string) (string, error) {
	if strings.TrimSpace(name) == "" || filepath.IsAbs(name) {
		return "", fmt.Errorf("invalid fixture %q", name)
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid fixture %q", name)
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("locate test fixtures")
	}
	path := filepath.Join(filepath.Dir(file), "testdata", clean)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("fixture is not a directory: %s", name)
	}
	return path, nil
}

func copyTree(source, target string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(source, entry.Name())
		targetPath := filepath.Join(target, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := os.MkdirAll(targetPath, info.Mode().Perm()); err != nil {
				return err
			}
			if err := copyTree(sourcePath, targetPath); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported fixture entry %s", sourcePath)
		}
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		if err := os.WriteFile(targetPath, data, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}
