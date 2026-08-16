// Package workspace provides filesystem boundaries for one agent worktree.
package workspace

import (
	"errors"
	"fmt"
	"io"
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
	cleaned, err := cleanRelativePath(path)
	if err != nil {
		return "", err
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

// ReadRegularFile reads a bounded regular file through a descriptor-rooted
// path. On supported platforms os.Root keeps symlink resolution beneath this
// workspace even if another process changes path components concurrently.
func (r Root) ReadRegularFile(path string, maxBytes int64) ([]byte, error) {
	if r.path == "" {
		return nil, errors.New("workspace root is not initialized")
	}
	if maxBytes < 1 {
		return nil, errors.New("maximum file size must be positive")
	}
	cleaned, err := cleanRelativePath(path)
	if err != nil {
		return nil, err
	}
	directory, err := os.OpenRoot(r.path)
	if err != nil {
		return nil, fmt.Errorf("open workspace root: %w", err)
	}
	defer directory.Close()
	file, err := directory.Open(cleaned)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path %q is not a regular file", path)
	}
	if info.Size() > maxBytes {
		return nil, fileSizeLimitError(path, maxBytes)
	}
	contents, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	if int64(len(contents)) > maxBytes {
		return nil, fileSizeLimitError(path, maxBytes)
	}
	return contents, nil
}

func fileSizeLimitError(path string, maxBytes int64) error {
	if maxBytes%(1024*1024) == 0 {
		return fmt.Errorf("file %q exceeds the %d MiB limit; attachments must be no larger", path, maxBytes/(1024*1024))
	}
	return fmt.Errorf("file %q exceeds the %d-byte limit; attachments must be no larger", path, maxBytes)
}

func cleanRelativePath(path string) (string, error) {
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
	return cleaned, nil
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
