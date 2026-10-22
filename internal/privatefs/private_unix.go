//go:build darwin || linux

package privatefs

import (
	"errors"
	"os"
)

func Protect(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return os.Chmod(path, 0700)
	}
	if !info.Mode().IsRegular() {
		return errors.New("private path must be an ordinary file or directory")
	}
	return os.Chmod(path, 0600)
}

func Check(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if (!info.IsDir() && !info.Mode().IsRegular()) || info.Mode().Perm()&0077 != 0 {
		return errors.New("path is not private")
	}
	return nil
}

func SyncDir(directory *os.File) error { return directory.Sync() }
