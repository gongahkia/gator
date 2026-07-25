package runtime

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/gongahkia/norbot/internal/domain"
)

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type OSRunner struct{}

func (OSRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return output, nil
}

type Workspace struct {
	DockerBin    string
	ArtifactsDir string
	Runner       CommandRunner
}

func (w Workspace) Name(runID string) string   { return "norbot-ws-" + runID }
func (w Workspace) Volume(runID string) string { return "norbot_workspace_" + runID }

func (w Workspace) Ensure(ctx context.Context, runID string) error {
	if err := os.MkdirAll(w.RunPath(runID), 0o750); err != nil {
		return fmt.Errorf("create artifact directory: %w", err)
	}
	if _, err := w.Runner.Run(ctx, w.DockerBin, "volume", "create", w.Volume(runID)); err != nil {
		return fmt.Errorf("create run volume: %w", err)
	}
	_, err := w.Runner.Run(ctx, w.DockerBin, "container", "inspect", w.Name(runID))
	if err == nil {
		return nil
	}
	_, err = w.Runner.Run(ctx, w.DockerBin, "run", "-d", "--name", w.Name(runID), "--label", "norbot.run_id="+runID, "--label", "norbot.role=workspace", "-v", w.Volume(runID)+":/workspace", "alpine:3.21", "sleep", "infinity")
	if err != nil {
		return fmt.Errorf("start workspace: %w", err)
	}
	return nil
}

func (w Workspace) RunPath(runID string) string { return filepath.Join(w.ArtifactsDir, runID) }

func (w Workspace) WriteArtifact(runID, relative string, content []byte) (string, error) {
	if strings.HasPrefix(relative, "/") || strings.Contains(relative, "..") {
		return "", fmt.Errorf("unsafe artifact path")
	}
	path := filepath.Join(w.RunPath(runID), relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, content, 0o640); err != nil {
		return "", err
	}
	return path, nil
}

func (w Workspace) MirrorToVolume(ctx context.Context, runID, relative string) error {
	if strings.HasPrefix(relative, "/") || strings.Contains(relative, "..") {
		return fmt.Errorf("unsafe artifact path")
	}
	source := filepath.Join(w.RunPath(runID), relative)
	if _, err := os.Stat(source); err != nil {
		return err
	}
	_, err := w.Runner.Run(ctx, w.DockerBin, "cp", source, w.Name(runID)+":/workspace/"+relative)
	return err
}

func (w Workspace) RunCLI(ctx context.Context, runID string, command []string, prompt string, credentialEnv string) (string, error) {
	if len(command) == 0 {
		return "", fmt.Errorf("empty cli command")
	}
	args := []string{"exec", "-i"}
	if credentialEnv != "" {
		if value, ok := os.LookupEnv(credentialEnv); ok {
			args = append(args, "-e", credentialEnv+"="+value)
		}
	}
	args = append(args, w.Name(runID))
	args = append(args, command...)
	execCommand := exec.CommandContext(ctx, w.DockerBin, args...)
	execCommand.Stdin = strings.NewReader(prompt)
	output, err := execCommand.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("workspace cli: %w", err)
	}
	return string(output), nil
}

func (w Workspace) Cleanup(ctx context.Context, runID string) error {
	_, _ = w.Runner.Run(ctx, w.DockerBin, "rm", "-f", w.Name(runID))
	_, err := w.Runner.Run(ctx, w.DockerBin, "volume", "rm", w.Volume(runID))
	return err
}

type Capacity struct {
	CPUs               int    `json:"cpus"`
	MemoryBytes        int64  `json:"memory_bytes"`
	DockerAvailable    bool   `json:"docker_available"`
	ConfiguredWorkers  int    `json:"configured_workers"`
	RecommendedWorkers int    `json:"recommended_workers"`
	Recommendation     string `json:"recommendation"`
}

func DetectCapacity(ctx context.Context, dockerBin string, configuredWorkers, maxWorkers int, runner CommandRunner) Capacity {
	capacity := Capacity{CPUs: runtime.NumCPU(), ConfiguredWorkers: configuredWorkers}
	capacity.MemoryBytes = memoryBytes(ctx, runner)
	if _, err := runner.Run(ctx, dockerBin, "info", "--format", "{{.ServerVersion}}"); err == nil {
		capacity.DockerAvailable = true
	}
	byCPU := max(1, capacity.CPUs/2)
	byMemory := 1
	if capacity.MemoryBytes > 0 {
		byMemory = max(1, int(capacity.MemoryBytes/(4<<30)))
	}
	capacity.RecommendedWorkers = min(maxWorkers, min(byCPU, byMemory))
	if !capacity.DockerAvailable {
		capacity.Recommendation = "Docker unavailable; run execution cannot start."
		return capacity
	}
	if configuredWorkers > capacity.RecommendedWorkers {
		capacity.Recommendation = "Configured workers exceed the local recommendation; reduce workers or explicitly accept contention."
	} else {
		capacity.Recommendation = "Configured workers are within the local CPU/RAM recommendation."
	}
	return capacity
}

func memoryBytes(ctx context.Context, runner CommandRunner) int64 {
	if runtime.GOOS == "darwin" {
		output, err := runner.Run(ctx, "sysctl", "-n", "hw.memsize")
		if err == nil {
			value, parseErr := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
			if parseErr == nil {
				return value
			}
		}
	}
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/meminfo")
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				fields := strings.Fields(line)
				if len(fields) >= 2 && fields[0] == "MemTotal:" {
					value, parseErr := strconv.ParseInt(fields[1], 10, 64)
					if parseErr == nil {
						return value * 1024
					}
				}
			}
		}
	}
	return 0
}

type Deployment struct {
	DockerBin string
	Runner    CommandRunner
}

func ProjectName(runID string) string {
	return "norbot-" + strings.ToLower(runID)
}

func (d Deployment) Deploy(ctx context.Context, run domain.Run, root string) (string, error) {
	port, err := reservePort()
	if err != nil {
		return "", err
	}
	project := ProjectName(run.ID)
	args := []string{"compose", "-p", project, "--project-directory", filepath.Join(root, "generated-app"), "up", "--build", "-d"}
	command := exec.CommandContext(ctx, d.DockerBin, args...)
	command.Dir = filepath.Join(root, "generated-app")
	command.Env = append(os.Environ(), "NORBOT_PUBLIC_PORT="+strconv.Itoa(port), "NORBOT_RUN_ID="+run.ID)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("deploy %s: %w: %s", project, err, tail(string(output), 1000))
	}
	return "http://127.0.0.1:" + strconv.Itoa(port), nil
}

func (d Deployment) Stop(ctx context.Context, runID, root string) error {
	_, err := d.Runner.Run(ctx, d.DockerBin, "compose", "-p", ProjectName(runID), "--project-directory", filepath.Join(root, "generated-app"), "down", "--remove-orphans")
	return err
}

func reservePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func ArtifactDigest(content []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(content)) }

func tail(value string, maxLength int) string {
	if len(value) <= maxLength {
		return value
	}
	return value[len(value)-maxLength:]
}
