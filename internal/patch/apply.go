package patch

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

type ApplyError struct {
	File string
	Hunk int
	Err  error
}

func (e *ApplyError) Error() string {
	if e.Hunk > 0 {
		return fmt.Sprintf("apply %s hunk %d: %v", e.File, e.Hunk, e.Err)
	}
	return fmt.Sprintf("apply %s: %v", e.File, e.Err)
}

func (e *ApplyError) Unwrap() error {
	return e.Err
}

func Apply(cwd string, diff string) error {
	files, _, err := gitdiff.Parse(strings.NewReader(diff))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("patch contains no files")
	}
	backups := map[string]backup{}
	for _, file := range files {
		if err := applyFile(cwd, file, backups); err != nil {
			rollback(backups)
			return err
		}
	}
	return nil
}

func applyFile(cwd string, file *gitdiff.File, backups map[string]backup) error {
	sourceRel, targetRel, err := patchPaths(file)
	if err != nil {
		return err
	}
	target, err := resolvePath(cwd, targetRel)
	if err != nil {
		return err
	}
	if sourceRel != "" {
		if _, err := resolvePath(cwd, sourceRel); err != nil {
			return err
		}
	}
	src, mode, exists, err := readSource(cwd, sourceRel, file.IsNew)
	if err != nil {
		return err
	}
	if _, ok := backups[target]; !ok {
		backups[target] = backupFile(target)
	}
	var out bytes.Buffer
	if err := gitdiff.Apply(&out, bytes.NewReader(src), file); err != nil {
		return wrapApplyError(targetRel, err)
	}
	if file.IsDelete {
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return wrapApplyError(targetRel, err)
		}
		return nil
	}
	if !exists {
		mode = 0o644
	}
	return writeAtomic(target, out.Bytes(), mode)
}

func patchPaths(file *gitdiff.File) (string, string, error) {
	source := cleanDiffPath(file.OldName)
	target := cleanDiffPath(file.NewName)
	if file.IsNew {
		source = ""
	}
	if file.IsDelete {
		target = source
	}
	if target == "" {
		return "", "", fmt.Errorf("patch target path is empty")
	}
	if err := validateRelPath(target); err != nil {
		return "", "", err
	}
	if source != "" {
		if err := validateRelPath(source); err != nil {
			return "", "", err
		}
	}
	return source, target, nil
}

func cleanDiffPath(path string) string {
	if path == "/dev/null" {
		return ""
	}
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	return filepath.Clean(path)
}

func validateRelPath(path string) error {
	if path == "." || path == "" || filepath.IsAbs(path) {
		return fmt.Errorf("invalid patch path %q", path)
	}
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if part == ".." {
			return fmt.Errorf("invalid patch path %q", path)
		}
	}
	return nil
}

func resolvePath(cwd, rel string) (string, error) {
	if err := validateRelPath(rel); err != nil {
		return "", err
	}
	base := filepath.Clean(cwd)
	full := filepath.Clean(filepath.Join(base, rel))
	if full != base && !strings.HasPrefix(full, base+string(os.PathSeparator)) {
		return "", fmt.Errorf("patch path escapes cwd: %q", rel)
	}
	return full, nil
}

func readSource(cwd, rel string, isNew bool) ([]byte, fs.FileMode, bool, error) {
	if isNew || rel == "" {
		return nil, 0o644, false, nil
	}
	path, err := resolvePath(cwd, rel)
	if err != nil {
		return nil, 0, false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, false, err
	}
	return data, info.Mode(), true, nil
}

func writeAtomic(path string, data []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".paw-patch-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func wrapApplyError(file string, err error) error {
	var applyErr *gitdiff.ApplyError
	if errors.As(err, &applyErr) {
		return &ApplyError{File: file, Hunk: applyErr.Fragment, Err: err}
	}
	return &ApplyError{File: file, Err: err}
}

type backup struct {
	exists bool
	data   []byte
	mode   fs.FileMode
}

func backupFile(path string) backup {
	info, err := os.Stat(path)
	if err != nil {
		return backup{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return backup{}
	}
	return backup{exists: true, data: data, mode: info.Mode()}
}

func rollback(backups map[string]backup) {
	for path, b := range backups {
		if !b.exists {
			_ = os.Remove(path)
			continue
		}
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, b.data, b.mode)
	}
}
