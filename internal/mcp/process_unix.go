//go:build darwin || linux

package mcp

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func ownMCPProcess(command *exec.Cmd) func() {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Setpgid = true
	kill := func() error {
		if command.Process == nil {
			return nil
		}
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	command.Cancel = kill
	command.WaitDelay = 250 * time.Millisecond
	return func() { _ = kill() }
}
