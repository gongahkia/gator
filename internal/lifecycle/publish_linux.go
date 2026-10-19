//go:build linux

package lifecycle

import (
	"golang.org/x/sys/unix"
	"os"
)

func publishProfile(parent *os.Root, from, to string) error {
	dir, err := parent.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	return unix.Renameat2(int(dir.Fd()), from, int(dir.Fd()), to, unix.RENAME_NOREPLACE)
}
