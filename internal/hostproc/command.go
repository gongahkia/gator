// Package hostproc starts app-owned subprocesses without opening extra console
// windows. It does not grant permissions or change the execution sandbox.
package hostproc

import (
	"context"
	"os/exec"
)

func Command(name string, args ...string) *exec.Cmd { return configure(exec.Command(name, args...)) }
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return configure(exec.CommandContext(ctx, name, args...))
}
