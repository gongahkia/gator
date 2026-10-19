package state

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// Guard prevents data export or removal from racing a writer. Exclusive
// maintenance never interrupts existing work; callers can retry when it ends.
func Guard(stateDir string, exclusive bool) (func(), error) {
	root := filepath.Join(stateDir, "gator")
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(root, ".data-lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	mode := syscall.LOCK_SH
	if exclusive {
		mode = syscall.LOCK_EX | syscall.LOCK_NB
	}
	if err = syscall.Flock(int(f.Fd()), mode); err != nil {
		f.Close()
		if exclusive && (errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)) {
			return nil, errors.New("work is using private data; finish or stop it before exporting or deleting data")
		}
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
