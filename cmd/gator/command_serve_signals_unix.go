//go:build darwin || linux

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func serveSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

func detachServeProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func terminateServeProcess(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}
