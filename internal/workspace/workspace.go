package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	pawruntime "github.com/gongahkia/paw/internal/runtime"
)

var ErrPathEscapesRoot = errors.New("path escapes workspace root")

type Manifest struct {
	Root         string `json:"root"`
	GitRoot      string `json:"git_root,omitempty"`
	IsGit        bool   `json:"is_git"`
	WritableHint bool   `json:"writable_hint"`
}

func CanonicalRoot(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("workspace path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace is not a directory: %s", path)
	}
	return filepath.Clean(resolved), nil
}

func ResolvePath(root, rel string) (string, error) {
	base, err := CanonicalRoot(root)
	if err != nil {
		return "", err
	}
	if rel == "" || filepath.IsAbs(rel) {
		return "", fmt.Errorf("%w: %q", ErrPathEscapesRoot, rel)
	}
	clean := filepath.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", ErrPathEscapesRoot, rel)
	}
	full := filepath.Join(base, clean)
	if !inside(base, full) {
		return "", fmt.Errorf("%w: %q", ErrPathEscapesRoot, rel)
	}
	resolved, err := resolveFuturePath(full)
	if err != nil {
		return "", err
	}
	if !inside(base, resolved) {
		return "", fmt.Errorf("%w: %q", ErrPathEscapesRoot, rel)
	}
	return resolved, nil
}

func Inspect(path string) (Manifest, error) {
	return inspect(path, pawruntime.NativeRunner{})
}

func inspect(path string, runner pawruntime.Runner) (Manifest, error) {
	root, err := CanonicalRoot(path)
	if err != nil {
		return Manifest{}, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return Manifest{}, err
	}
	manifest := Manifest{Root: root, WritableHint: info.Mode().Perm()&0o222 != 0}
	result, err := runner.Run(context.Background(), pawruntime.Command{Path: "git", Args: []string{"rev-parse", "--show-toplevel"}, Dir: root})
	if err == nil {
		gitRoot, rootErr := CanonicalRoot(strings.TrimSpace(string(result.Stdout)))
		if rootErr != nil {
			return Manifest{}, rootErr
		}
		manifest.IsGit = true
		manifest.GitRoot = gitRoot
		return manifest, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) || errors.Is(err, exec.ErrNotFound) {
		return manifest, nil
	}
	return Manifest{}, fmt.Errorf("inspect git workspace: %w", err)
}

func inside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func resolveFuturePath(path string) (string, error) {
	var suffix []string
	current := path
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}
