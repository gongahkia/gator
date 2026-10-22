//go:build darwin || linux

package hostproc

import "os/exec"

func configure(command *exec.Cmd) *exec.Cmd { return command }
