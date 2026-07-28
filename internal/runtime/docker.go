package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/gongahkia/norbot/internal/config"
)

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
	command := exec.CommandContext(ctx, c.Bin, args...)
	command.Env = append(os.Environ(), c.Env...)
	command.Stdin = strings.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s %s: %w", c.Bin, strings.Join(args, " "), err)
	}
	return output, nil
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
