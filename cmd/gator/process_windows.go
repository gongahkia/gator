package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureDetached(command *exec.Cmd, _ bool) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
}

func processAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return true
	}
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var code uint32
	return windows.GetExitCodeProcess(handle, &code) == nil && code == 259 // STILL_ACTIVE
}

func shutdownSignals() []os.Signal { return []os.Signal{os.Interrupt} }
