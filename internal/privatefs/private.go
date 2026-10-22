// Package privatefs adapts private-file checks and directory durability to the
// host filesystem. Unix permission bits and Windows DACLs are not interchangeable.
package privatefs

import "os"

func MkdirAll(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return Protect(path)
}
