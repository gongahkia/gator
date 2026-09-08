package workspace

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// WriteRegularFileAtomic replaces a bounded regular file inside the workspace.
// Parent directories and files are private by default. Descriptor-rooted I/O
// keeps every operation beneath the workspace even if path components change
// concurrently.
func (r Root) WriteRegularFileAtomic(path string, contents []byte, maxBytes int64) error {
	if r.path == "" {
		return errors.New("workspace root is not initialized")
	}
	if maxBytes < 1 {
		return errors.New("maximum file size must be positive")
	}
	if int64(len(contents)) > maxBytes {
		return fileSizeLimitError(path, maxBytes)
	}
	cleaned, err := cleanRelativePath(path)
	if err != nil {
		return err
	}

	root, err := os.OpenRoot(r.path)
	if err != nil {
		return fmt.Errorf("open workspace root: %w", err)
	}
	defer root.Close()

	parent := filepath.Dir(cleaned)
	if parent != "." {
		if err := root.MkdirAll(parent, 0o700); err != nil {
			return fmt.Errorf("create parent directories for %q: %w", path, err)
		}
	}
	if info, err := root.Lstat(cleaned); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("path %q is not a regular file", path)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect %q: %w", path, err)
	}

	temporary, file, err := createTemporaryFile(root, parent)
	if err != nil {
		return fmt.Errorf("create temporary file for %q: %w", path, err)
	}
	keepTemporary := true
	defer func() {
		_ = file.Close()
		if keepTemporary {
			_ = root.Remove(temporary)
		}
	}()
	if _, err := file.ReadFrom(bytes.NewReader(contents)); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %q: %w", path, err)
	}
	if err := root.Rename(temporary, cleaned); err != nil {
		return fmt.Errorf("replace %q: %w", path, err)
	}
	keepTemporary = false
	if directory, err := root.Open(parent); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func createTemporaryFile(root *os.Root, parent string) (string, *os.File, error) {
	for range 10 {
		entropy := make([]byte, 8)
		if _, err := rand.Read(entropy); err != nil {
			return "", nil, err
		}
		name := filepath.Join(parent, ".gator-write-"+hex.EncodeToString(entropy))
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return name, file, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", nil, err
		}
	}
	return "", nil, errors.New("could not allocate a unique temporary file")
}
