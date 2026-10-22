//go:build darwin || linux

package filelock

import (
	"errors"
	"os"
	"syscall"
)

const noFollow = syscall.O_NOFOLLOW

func Lock(file *os.File, mode int) error {
	flags := syscall.LOCK_SH
	if mode&Exclusive != 0 {
		flags = syscall.LOCK_EX
	}
	if mode&Nonblocking != 0 {
		flags |= syscall.LOCK_NB
	}
	err := syscall.Flock(int(file.Fd()), flags)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return ErrWouldBlock
	}
	return err
}

func Unlock(file *os.File) error { return syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }
