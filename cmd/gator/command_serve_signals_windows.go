//go:build windows

package main

import (
	"os"
	"os/exec"
)

func serveSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

func detachServeProcess(_ *exec.Cmd) {}

func terminateServeProcess(process *os.Process) error {
	return process.Kill()
}
