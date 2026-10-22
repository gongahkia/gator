//go:build darwin || linux

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureDetached(command *exec.Cmd, session bool) {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: session, Setpgid: !session}
}

func processAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}
