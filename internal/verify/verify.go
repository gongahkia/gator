package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/paw/internal/envelope"
)

type Verify struct {
	Command        string
	Timeout        time.Duration
	MaxDigestBytes int
}

func New(command string) *Verify {
	return &Verify{Command: command, Timeout: 2 * time.Minute}
}

func (v *Verify) Name() string {
	return "verify"
}

func (v *Verify) Run(ctx context.Context, in *envelope.Envelope) (*envelope.Envelope, error) {
	out := *in
	out.Stage = v.Name()
	cmdText := v.command(in.Cwd)
	runCtx := ctx
	cancel := func() {}
	if v.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, v.Timeout)
	}
	defer cancel()
	cmd := exec.CommandContext(runCtx, "sh", "-c", cmdText)
	if in.Cwd != "" {
		cmd.Dir = in.Cwd
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	timedOut := runCtx.Err() == context.DeadlineExceeded
	exitCode := 0
	if err != nil {
		exitCode = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}
	if timedOut {
		exitCode = 124
	}
	output := combinedOutput(stdout.Bytes(), stderr.Bytes())
	if timedOut {
		output = append(output, []byte(fmt.Sprintf("\nverify timed out after %s\n", v.Timeout))...)
	}
	out.Verify = &envelope.VerifyResult{
		Passed:        err == nil && !timedOut,
		ExitCode:      exitCode,
		Command:       cmdText,
		FailureDigest: failureDigest(output, v.MaxDigestBytes),
		RawTailBytes:  len(output),
		StdoutBytes:   stdout.Len(),
		StderrBytes:   stderr.Len(),
		TimedOut:      timedOut,
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
	return ResolveCommand(cwd)
}

func ResolveCommand(cwd string) string {
	if hasMakefile(cwd) {
		return "make test"
	}
	if hasPackageJSONTest(cwd) {
		return "npm test"
	}
	if exists(filepath.Join(cwd, "go.mod")) {
		return "go test ./..."
	}
	if exists(filepath.Join(cwd, "Cargo.toml")) {
		return "cargo test"
	}
	if exists(filepath.Join(cwd, "pyproject.toml")) {
		return "python -m pytest"
	}
	return "true"
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasMakefile(cwd string) bool {
	return exists(filepath.Join(cwd, "Makefile")) || exists(filepath.Join(cwd, "makefile"))
}

func hasPackageJSONTest(cwd string) bool {
	data, err := os.ReadFile(filepath.Join(cwd, "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return false
	}
	test := strings.TrimSpace(pkg.Scripts["test"])
	return test != "" && !strings.Contains(test, "no test specified")
}

func combinedOutput(stdout, stderr []byte) []byte {
	if len(stdout) == 0 {
		return append([]byte(nil), stderr...)
	}
	if len(stderr) == 0 {
		return append([]byte(nil), stdout...)
	}
	out := make([]byte, 0, len(stdout)+1+len(stderr))
	out = append(out, stdout...)
	if stdout[len(stdout)-1] != '\n' {
		out = append(out, '\n')
	}
	out = append(out, stderr...)
	return out
}
