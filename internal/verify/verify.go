package verify

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gongahkia/paw/internal/envelope"
)

type Verify struct {
	Command        string
	MaxDigestBytes int
}

func New(command string) *Verify {
	return &Verify{Command: command}
}

func (v *Verify) Name() string {
	return "verify"
}

func (v *Verify) Run(ctx context.Context, in *envelope.Envelope) (*envelope.Envelope, error) {
	out := *in
	out.Stage = v.Name()
	cmdText := v.command(in.Cwd)
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdText)
	if in.Cwd != "" {
		cmd.Dir = in.Cwd
	}
	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		exitCode = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}
	out.Verify = &envelope.VerifyResult{
		Passed:        err == nil,
		ExitCode:      exitCode,
		Command:       cmdText,
		FailureDigest: string(output),
		RawTailBytes:  len(output),
	}
	return &out, nil
}

func (v *Verify) command(cwd string) string {
	if env := os.Getenv("PAW_VERIFY_CMD"); env != "" {
		return env
	}
	if v.Command != "" {
		return v.Command
	}
	if exists(filepath.Join(cwd, "Makefile")) {
		return "make test"
	}
	if exists(filepath.Join(cwd, "go.mod")) {
		return "go test ./..."
	}
	return "true"
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
