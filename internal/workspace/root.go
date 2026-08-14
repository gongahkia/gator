// Package workspace provides filesystem boundaries for one agent worktree.
package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Root is the canonical filesystem boundary for a run. It resolves every path
// component and rejects symlinks that leave the worktree.
type Root struct {
	path string
}

// Open validates and canonicalizes a workspace directory.
func Open(path string) (Root, error) {
	if strings.TrimSpace(path) == "" {
		return Root{}, errors.New("workspace path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Root{}, fmt.Errorf("absolute workspace path: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return Root{}, fmt.Errorf("resolve workspace path: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return Root{}, fmt.Errorf("stat workspace path: %w", err)
	}
	if !info.IsDir() {
		return Root{}, fmt.Errorf("workspace path %q is not a directory", path)
	}
	return Root{path: canonical}, nil
}

// Path returns the canonical workspace directory.
func (r Root) Path() string {
	return r.path
}

// ResolveFile returns a canonical file path inside the workspace. Relative
// paths are required; absolute paths and escaping symlinks are rejected.
func (r Root) ResolveFile(path string) (string, error) {
	if r.path == "" {
		return "", errors.New("workspace root is not initialized")
	}
	if strings.TrimSpace(path) == "" {
		return "", errors.New("workspace-relative path is required")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute path %q is not allowed", path)
	}

	cleaned := filepath.Clean(path)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the workspace", path)
	}

	current := r.path
	parts := strings.Split(cleaned, string(filepath.Separator))
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid path component in %q", path)
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				if err := r.assertInside(current); err != nil {
					return "", err
				}
				continue
			}
			return "", fmt.Errorf("inspect path %q: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", fmt.Errorf("resolve symlink in %q: %w", path, err)
			}
			current = resolved
		}
		if err := r.assertInside(current); err != nil {
			return "", err
		}
	}
	return current, nil
}

func (r Root) assertInside(path string) error {
	relative, err := filepath.Rel(r.path, path)
	if err != nil {
		return fmt.Errorf("compare workspace path: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("path %q escapes the workspace", path)
	}
	return nil
}
