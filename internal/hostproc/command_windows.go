package hostproc

import (
	"os/exec"
	"syscall"
)

func configure(command *exec.Cmd) *exec.Cmd {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return command
}
