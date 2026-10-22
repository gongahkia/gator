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
