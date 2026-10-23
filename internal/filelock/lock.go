// Package filelock provides process-held advisory leases on every supported OS.
// Closing the file or exiting the process releases its locks.
package filelock

import (
	"errors"
	"os"
)

const (
	Shared = 1 << iota
	Exclusive
	Nonblocking
)

var ErrWouldBlock = errors.New("file is locked by another process")

// OpenRootFile opens an ordinary file without accepting a final symlink. Root
// confines traversal; SameFile also rejects a replacement during the open.
// Callers must not use the file until this validation has completed.
func OpenRootFile(root *os.Root, name string, flag int, perm os.FileMode) (*os.File, error) {
	f, err := root.OpenFile(name, flag|noFollow, perm)
	// Root.OpenFile can race another creator during its rooted traversal on
	// macOS. Retry only when a create-capable open lost that race to an ordinary
	// file. The opened descriptor still receives the same identity validation.
	for attempt := 0; attempt < 3 && err != nil && flag&os.O_CREATE != 0 && (errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrExist)); attempt++ {
		linked, statErr := root.Lstat(name)
		if statErr != nil || !linked.Mode().IsRegular() {
			break
		}
		f, err = root.OpenFile(name, flag|noFollow, perm)
	}
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	linked, linkErr := root.Lstat(name)
	if err != nil || linkErr != nil || !opened.Mode().IsRegular() || !linked.Mode().IsRegular() || !os.SameFile(opened, linked) {
		f.Close()
		return nil, errors.New("lock must be an ordinary file")
	}
	return f, nil
}
