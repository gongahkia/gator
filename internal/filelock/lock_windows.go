package filelock

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

const noFollow = 0 // OpenRootFile validates the opened handle against Root.Lstat.

// Windows locks are mandatory byte-range locks. Reserve a byte far past the
// contents so that probing a lease never prevents reading its metadata.
const lockOffset = 0x7fffffff

func Lock(file *os.File, mode int) error {
	var flags uint32
	if mode&Exclusive != 0 {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	if mode&Nonblocking != 0 {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	err := windows.LockFileEx(windows.Handle(file.Fd()), flags, 0, 1, 0, &windows.Overlapped{Offset: lockOffset})
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrWouldBlock
	}
	return err
}

func Unlock(file *os.File) error {
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &windows.Overlapped{Offset: lockOffset})
}
