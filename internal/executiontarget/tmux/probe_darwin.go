//go:build darwin

package tmux

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"syscall"
)

type OSProbe struct{}

func (OSProbe) Process(pid int) (string, error) {
	p, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || p.Proc.P_pid != int32(pid) || p.Proc.P_starttime.Sec <= 0 {
		return "", ErrUnavailable
	}
	return fmt.Sprintf("%d:%d", p.Proc.P_starttime.Sec, p.Proc.P_starttime.Usec), nil
}
func (OSProbe) Socket(path string) (string, error) {
	parent, pe := os.Lstat(filepath.Dir(path))
	if pe != nil || !parent.IsDir() || parent.Mode().Perm()&0077 != 0 {
		return "", ErrUnavailable
	}
	ps, ok := parent.Sys().(*syscall.Stat_t)
	if !ok || ps.Uid != uint32(os.Getuid()) {
		return "", ErrUnavailable
	}
	i, e := os.Lstat(path)
	if e != nil || i.Mode()&os.ModeSocket == 0 {
		return "", ErrUnavailable
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	if !ok || s.Uid != uint32(os.Getuid()) {
		return "", ErrUnavailable
	}
	return fmt.Sprintf("%d:%d:%d:%d", s.Dev, s.Ino, s.Ctimespec.Sec, s.Ctimespec.Nsec), nil
}
