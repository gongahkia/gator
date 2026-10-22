package state

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/gongahkia/gator/internal/filelock"
)

var ErrBusy = errors.New("work is using private data; finish or stop it before exporting or deleting data")

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
	mode := filelock.Shared
	if exclusive {
		mode = filelock.Exclusive | filelock.Nonblocking
	}
	if err = filelock.Lock(f, mode); err != nil {
		f.Close()
		if exclusive && (errors.Is(err, filelock.ErrWouldBlock)) {
			return nil, ErrBusy
		}
		return nil, err
	}
	return func() { _ = filelock.Unlock(f); _ = f.Close() }, nil
}
