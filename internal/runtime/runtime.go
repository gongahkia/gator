package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
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

type WorkspaceBackend interface {
	Ensure(context.Context, string) error
	RunPath(string) string
	WriteArtifact(string, string, []byte) (string, error)
	MirrorToVolume(context.Context, string, string) error
	MirrorGeneratedApp(context.Context, string) error
	SyncGeneratedApp(context.Context, string) error
	RunCLI(context.Context, string, string, string, []string, string, string, string, string) (string, error)
	Cleanup(context.Context, string) error
}

type ArtifactWorkspace interface {
	RunPath(string) string
	WriteArtifact(string, string, []byte) (string, error)
}

type DeploymentBackend interface {
	Target() domain.DeploymentTarget
	Deploy(context.Context, domain.Run, string) (string, error)
	Stop(context.Context, string, string) error
	Start(context.Context, string, string) error
	Delete(context.Context, string, string) error
	Status(context.Context, string, string) (DeploymentStatus, error)
	Logs(context.Context, string, string, int) (string, error)
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

func (w Workspace) MirrorGeneratedApp(ctx context.Context, runID string) error {
	source := filepath.Join(w.RunPath(runID), "generated-app")
	if _, err := os.Stat(source); err != nil {
		return err
	}
	if _, err := w.Runner.Run(ctx, w.DockerBin, "exec", w.Name(runID), "mkdir", "-p", "/workspace/generated-app"); err != nil {
		return err
	}
	_, err := w.Runner.Run(ctx, w.DockerBin, "cp", source+"/.", w.Name(runID)+":/workspace/generated-app")
	return err
}

func (w Workspace) SyncGeneratedApp(ctx context.Context, runID string) error {
	target := filepath.Join(w.RunPath(runID), "generated-app")
	if err := os.MkdirAll(target, 0o750); err != nil {
		return err
	}
	_, err := w.Runner.Run(ctx, w.DockerBin, "cp", w.Name(runID)+":/workspace/generated-app/.", target)
	return err
}

func (w Workspace) RunCLI(ctx context.Context, runID, image, network string, command []string, prompt, credentialEnv, credentialSecret, credentialSecretKey string) (string, error) {
	if len(command) == 0 {
		return "", fmt.Errorf("empty cli command")
	}
	if image == "" {
		return "", fmt.Errorf("cli runner image is required")
	}
	if network == "" {
		network = "bridge"
	}
	args := []string{"run", "--rm", "-i", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=256", "--memory=4g", "--cpus=2", "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m", "--label", "norbot.run_id=" + runID, "--label", "norbot.role=agent", "--network", network, "-v", w.Volume(runID) + ":/workspace", "-w", "/workspace"}
	if credentialEnv != "" {
		if value, ok := os.LookupEnv(credentialEnv); ok {
			args = append(args, "-e", credentialEnv+"="+value)
		}
	}
	args = append(args, image)
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
	CPUs               int           `json:"cpus"`
	MemoryBytes        int64         `json:"memory_bytes"`
	DockerAvailable    bool          `json:"docker_available"`
	ConfiguredWorkers  int           `json:"configured_workers"`
	RecommendedWorkers int           `json:"recommended_workers"`
	Recommendation     string        `json:"recommendation"`
	QuotaWorkers       int           `json:"quota_workers,omitempty"`
	QuotaFactors       []QuotaFactor `json:"quota_factors,omitempty"`
}

type QuotaFactor struct {
	ProviderID                  string `json:"provider_id"`
	ConfiguredLimit             int    `json:"configured_limit,omitempty"`
	ConfiguredRequestsPerMinute int    `json:"configured_requests_per_minute,omitempty"`
	ObservedRemaining           *int   `json:"observed_remaining,omitempty"`
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

func (c *Capacity) ApplyProviderQuotas(factors []QuotaFactor) {
	c.QuotaFactors = factors
	quota := 0
	for _, factor := range factors {
		limit := factor.ConfiguredLimit
		if factor.ConfiguredRequestsPerMinute > 0 && (limit == 0 || factor.ConfiguredRequestsPerMinute < limit) {
			limit = factor.ConfiguredRequestsPerMinute
		}
		if factor.ObservedRemaining != nil && (*factor.ObservedRemaining < limit || limit == 0) {
			limit = *factor.ObservedRemaining
		}
		if limit > 0 && (quota == 0 || limit < quota) {
			quota = limit
		}
	}
	c.QuotaWorkers = quota
	if quota > 0 && quota < c.RecommendedWorkers {
		c.RecommendedWorkers = quota
		c.Recommendation = "Provider quota limits the local worker recommendation; operator confirmation is required before increasing workers."
	}
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

func (d Deployment) Target() domain.DeploymentTarget { return domain.DeploymentDocker }

type DeploymentStatus struct {
	Target    domain.DeploymentTarget `json:"target"`
	Project   string                  `json:"project"`
	Namespace string                  `json:"namespace,omitempty"`
	Workload  string                  `json:"workload,omitempty"`
	Image     string                  `json:"image,omitempty"`
	Services  []map[string]any        `json:"services"`
}

func ProjectName(runID string) string {
	return "norbot-" + strings.ToLower(runID)
}

func (d Deployment) Deploy(ctx context.Context, run domain.Run, root string) (string, error) {
	port, err := ReservePort()
	if err != nil {
		return "", err
	}
	project := ProjectName(run.ID)
	args := []string{"compose", "-p", project, "--project-directory", filepath.Join(root, "generated-app"), "up", "--build", "-d", "--wait", "--wait-timeout", "90"}
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
	_, err := d.Runner.Run(ctx, d.DockerBin, "compose", "-p", ProjectName(runID), "--project-directory", filepath.Join(root, "generated-app"), "stop")
	return err
}

func (d Deployment) Start(ctx context.Context, runID, root string) error {
	_, err := d.Runner.Run(ctx, d.DockerBin, "compose", "-p", ProjectName(runID), "--project-directory", filepath.Join(root, "generated-app"), "start", "--wait", "--wait-timeout", "90")
	return err
}

func (d Deployment) Delete(ctx context.Context, runID, root string) error {
	_, err := d.Runner.Run(ctx, d.DockerBin, "compose", "-p", ProjectName(runID), "--project-directory", filepath.Join(root, "generated-app"), "down", "--remove-orphans", "--volumes")
	return err
}

func (d Deployment) Status(ctx context.Context, runID, root string) (DeploymentStatus, error) {
	project := ProjectName(runID)
	output, err := d.Runner.Run(ctx, d.DockerBin, "compose", "-p", project, "--project-directory", filepath.Join(root, "generated-app"), "ps", "--format", "json")
	if err != nil {
		return DeploymentStatus{}, err
	}
	services := []map[string]any{}
	trimmed := strings.TrimSpace(string(output))
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &services); err != nil {
			return DeploymentStatus{}, fmt.Errorf("decode compose status: %w", err)
		}
		return DeploymentStatus{Target: d.Target(), Project: project, Services: services}, nil
	}
	for _, line := range strings.Split(trimmed, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var service map[string]any
		if err := json.Unmarshal([]byte(line), &service); err != nil {
			return DeploymentStatus{}, fmt.Errorf("decode compose status: %w", err)
		}
		services = append(services, service)
	}
	return DeploymentStatus{Target: d.Target(), Project: project, Services: services}, nil
}

func (d Deployment) Logs(ctx context.Context, runID, root string, lines int) (string, error) {
	if lines < 1 || lines > 10000 {
		return "", fmt.Errorf("log line limit must be 1-10000")
	}
	output, err := d.Runner.Run(ctx, d.DockerBin, "compose", "-p", ProjectName(runID), "--project-directory", filepath.Join(root, "generated-app"), "logs", "--no-color", "--tail", strconv.Itoa(lines))
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func ReservePort() (int, error) {
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
