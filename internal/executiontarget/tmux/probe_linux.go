//go:build linux

package tmux

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type OSProbe struct{}

func procRead(path string, max int64) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", ErrUnavailable
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, max+1))
	if e != nil || int64(len(b)) > max {
		return "", ErrUnavailable
	}
	return string(b), nil
}
func (OSProbe) Process(pid int) (string, error) {
	b, e := procRead(fmt.Sprintf("/proc/%d/stat", pid), 8192)
	if e != nil {
		return "", e
	}
	n := strings.LastIndex(b, ") ")
	if n < 0 {
		return "", ErrUnavailable
	}
	fields := strings.Fields(b[n+2:])
	if len(fields) < 20 {
		return "", ErrUnavailable
	}
	ticks, e := strconv.ParseUint(fields[19], 10, 64)
	if e != nil || ticks == 0 {
		return "", ErrUnavailable
	}
	boot, e := procRead("/proc/sys/kernel/random/boot_id", 64)
	if e != nil {
		return "", e
	}
	return strings.TrimSpace(boot) + ":" + fields[19], nil
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
	return fmt.Sprintf("%d:%d:%d:%d", s.Dev, s.Ino, s.Ctim.Sec, s.Ctim.Nsec), nil
}
