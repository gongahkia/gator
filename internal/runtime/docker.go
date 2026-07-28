package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/norbot/internal/config"
)

var commandSecret = regexp.MustCompile(`(?i)(authorization:\s*bearer\s+|(?:api[_-]?key|token|password|secret)=)([^\s'"&]+)`)

type CommandRecord struct {
	Command         string   `json:"command"`
	Args            []string `json:"args"`
	DurationMS      int64    `json:"duration_ms"`
	ExitCode        int      `json:"exit_code"`
	Output          string   `json:"output"`
	OutputTruncated bool     `json:"output_truncated,omitempty"`
	fullOutput      string
}

type CommandRecorder struct {
	mu      sync.Mutex
	records []CommandRecord
}

func (r *CommandRecorder) Records() []CommandRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]CommandRecord, len(r.records))
	copy(result, r.records)
	return result
}

func (r *CommandRecorder) FullRecords() []CommandRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]CommandRecord, len(r.records))
	copy(result, r.records)
	for i := range result {
		result[i].Output = result[i].fullOutput
		result[i].OutputTruncated = false
		result[i].fullOutput = ""
	}
	return result
}

type recordingRunner struct {
	inner    CommandRunner
	recorder *CommandRecorder
}

func (r recordingRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	started := time.Now()
	output, err := r.inner.Run(ctx, name, args...)
	r.record(name, args, output, err, time.Since(started))
	return output, err
}
func (r recordingRunner) RunWithEnv(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	started := time.Now()
	var output []byte
	var err error
	if runner, ok := r.inner.(EnvCommandRunner); ok {
		output, err = runner.RunWithEnv(ctx, env, name, args...)
	} else {
		output, err = r.inner.Run(ctx, name, args...)
	}
	r.record(name, args, output, err, time.Since(started))
	return output, err
}
func (r recordingRunner) RunInputWithEnv(ctx context.Context, env []string, name, input string, args ...string) ([]byte, error) {
	started := time.Now()
	var output []byte
	var err error
	if runner, ok := r.inner.(InputCommandRunner); ok {
		output, err = runner.RunInputWithEnv(ctx, env, name, input, args...)
	} else {
		output, err = runInput(ctx, env, name, input, args...)
	}
	r.record(name, args, output, err, time.Since(started))
	return output, err
}
func (r recordingRunner) record(name string, args []string, output []byte, err error, elapsed time.Duration) {
	fullOutput := redactCommandOutput(string(output))
	value, truncated := trimCommandOutput(fullOutput)
	r.recorder.mu.Lock()
	defer r.recorder.mu.Unlock()
	r.recorder.records = append(r.recorder.records, CommandRecord{Command: name, Args: append([]string(nil), args...), DurationMS: elapsed.Milliseconds(), ExitCode: commandExitCode(err), Output: value, OutputTruncated: truncated, fullOutput: fullOutput})
}

func (c DockerClient) WithRecorder(recorder *CommandRecorder) DockerClient {
	if recorder == nil {
		return c
	}
	c.Runner = recordingRunner{inner: c.Runner, recorder: recorder}
	return c
}

func runInput(ctx context.Context, env []string, name, input string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = append(os.Environ(), env...)
	command.Stdin = strings.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return output, nil
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exited *exec.ExitError
	if errors.As(err, &exited) {
		return exited.ExitCode()
	}
	return -1
}

func trimCommandOutput(value string) (string, bool) {
	const limit = 64 << 10
	if len(value) <= limit {
		return value, false
	}
	return value[len(value)-limit:], true
}

func redactCommandOutput(value string) string {
	return commandSecret.ReplaceAllString(value, "$1[REDACTED]")
}

type EnvCommandRunner interface {
	RunWithEnv(context.Context, []string, string, ...string) ([]byte, error)
}

type InputCommandRunner interface {
	RunInputWithEnv(context.Context, []string, string, string, ...string) ([]byte, error)
}

type DockerClient struct {
	Bin    string
	Env    []string
	Mode   string
	Runner CommandRunner
	err    error
}

func NewDockerClient(bin string, settings config.Docker, runner CommandRunner) DockerClient {
	if bin == "" {
		bin = "docker"
	}
	if runner == nil {
		runner = OSRunner{}
	}
	settings = settings.Normalized()
	client := DockerClient{Bin: bin, Mode: settings.Mode, Runner: runner}
	if err := settings.ValidateEnvironment(os.Getenv); err != nil {
		client.err = err
		return client
	}
	if settings.Mode == config.DockerModeRootlessRemoteTLS {
		client.Env = []string{
			settings.HostEnv + "=" + os.Getenv(settings.HostEnv),
			settings.TLSVerifyEnv + "=" + os.Getenv(settings.TLSVerifyEnv),
			settings.CertPathEnv + "=" + os.Getenv(settings.CertPathEnv),
		}
	}
	return client
}

func LegacyDockerClient(bin string, runner CommandRunner) DockerClient {
	if bin == "" {
		bin = "docker"
	}
	if runner == nil {
		runner = OSRunner{}
	}
	return DockerClient{Bin: bin, Runner: runner}
}

func (c DockerClient) Run(ctx context.Context, args ...string) ([]byte, error) {
	if c.err != nil {
		return nil, c.err
	}
	if runner, ok := c.Runner.(EnvCommandRunner); ok {
		return runner.RunWithEnv(ctx, c.Env, c.Bin, args...)
	}
	return c.Runner.Run(ctx, c.Bin, args...)
}

func (c DockerClient) RunInput(ctx context.Context, input string, args ...string) ([]byte, error) {
	if c.err != nil {
		return nil, c.err
	}
	if runner, ok := c.Runner.(InputCommandRunner); ok {
		return runner.RunInputWithEnv(ctx, c.Env, c.Bin, input, args...)
	}
	return runInput(ctx, c.Env, c.Bin, input, args...)
}

func (c DockerClient) Validate(ctx context.Context) error {
	if c.err != nil {
		return c.err
	}
	if c.Mode != config.DockerModeRootlessRemoteTLS {
		return nil
	}
	output, err := c.Run(ctx, "info", "--format", "{{json .SecurityOptions}}")
	if err != nil {
		return fmt.Errorf("inspect rootless Docker daemon: %w", err)
	}
	if !strings.Contains(strings.ToLower(string(output)), "rootless") {
		return fmt.Errorf("configured Docker daemon is not rootless")
	}
	return nil
}

func (OSRunner) RunWithEnv(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = append(os.Environ(), env...)
	output, err := command.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return output, nil
}

func (OSRunner) RunInputWithEnv(ctx context.Context, env []string, name, input string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = append(os.Environ(), env...)
	command.Stdin = strings.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return output, nil
}
