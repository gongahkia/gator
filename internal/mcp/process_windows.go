package mcp

import "os/exec"

// Strict stdio sandboxing stays unavailable on Windows. No host-access fallback.
func ownMCPProcess(command *exec.Cmd) func() {
	return func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
	}
}
