//go:build darwin

package lifecycle

import (
	"golang.org/x/sys/unix"
	"os"
)

// Publish a fully staged profile without replacing even an empty directory.
func publishProfile(parent *os.Root, from, to string) error {
	dir, err := parent.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	return unix.RenameatxNp(int(dir.Fd()), from, int(dir.Fd()), to, unix.RENAME_EXCL)
}
