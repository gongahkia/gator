package cliagent

import "os/exec"

// Production execution fails closed in sandbox.Prepare on Windows. No unsafe
// host-process fallback is introduced by this adapter.
func ownProcess(cmd *exec.Cmd) func() {
	return func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
}
