package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

type Command struct {
	Path  string
	Args  []string
	Dir   string
	Env   []string
	Stdin []byte
}

type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type Runner interface {
	Run(context.Context, Command) (Result, error)
}

type NativeRunner struct{}

func (NativeRunner) Run(ctx context.Context, command Command) (Result, error) {
	if command.Path == "" {
		return Result{}, fmt.Errorf("command path is required")
	}
	cmd := exec.CommandContext(ctx, command.Path, command.Args...)
	cmd.Dir = command.Dir
	if command.Env != nil {
		cmd.Env = append([]string(nil), command.Env...)
	}
	cmd.Stdin = bytes.NewReader(command.Stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err == nil {
		return result, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		return result, err
	}
	return result, err
}
