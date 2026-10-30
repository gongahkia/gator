//go:build darwin || linux

package cliagent

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Own the process group so cancellation also stops CLI-spawned commands.
func ownProcess(cmd *exec.Cmd) func() {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	kill := func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.Cancel = kill
	cmd.WaitDelay = 2 * time.Second
	return func() { _ = kill() }
}
