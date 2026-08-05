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
	"time"

	"github.com/gongahkia/norbot/internal/config"
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
	RunCLI(context.Context, string, domain.Stage, string, string, []string, string, string, string, string) (string, error)
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

type AgentInvocation struct {
	SessionID      string           `json:"session_id"`
	ExternalID     string           `json:"external_id"`
	Role           string           `json:"role"`
	Prompt         string           `json:"prompt"`
	IdempotencyKey string           `json:"idempotency_key"`
	Attachments    []map[string]any `json:"attachments,omitempty"`
	Memory         string           `json:"memory,omitempty"`
	Provider       AgentProvider    `json:"provider"`
}

type AgentProvider struct {
	Kind          string `json:"kind"`
	BaseURL       string `json:"base_url"`
	Model         string `json:"model"`
	CredentialEnv string `json:"credential_env"`
}

type AgentResponse struct {
	Final       string         `json:"final"`
	State       string         `json:"state"`
	Status      string         `json:"status"`
	Summary     string         `json:"summary,omitempty"`
	Diagnostics map[string]any `json:"diagnostics,omitempty"`
}

type AgentBackend interface {
	InvokeAgent(context.Context, domain.Run, string, AgentInvocation) (AgentResponse, error)
}

type SandboxRequest struct {
	Image         string            `json:"image,omitempty"`
	Command       []string          `json:"command"`
	AllowedHosts  []string          `json:"allowed_hosts,omitempty"`
	ReadOnlyPaths []string          `json:"read_only_paths,omitempty"`
	ProxyHeaders  map[string]string `json:"proxy_headers,omitempty"`
}
type SandboxResult struct {
	Output     string `json:"output"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
	Network    string `json:"network"`
}
type SandboxBackend interface {
	RunSandbox(context.Context, string, SandboxRequest, config.Sandbox) (SandboxResult, error)
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
	Docker       DockerClient
	DockerBin    string
	ArtifactsDir string
	Runner       CommandRunner
}

func (w Workspace) Client() DockerClient {
	if w.Docker.Bin != "" || w.Docker.Runner != nil || w.Docker.err != nil {
		return w.Docker
	}
	return LegacyDockerClient(w.DockerBin, w.Runner)
}

func (w Workspace) Validate(ctx context.Context) error { return w.Client().Validate(ctx) }

func (w Workspace) Name(runID string) string   { return "norbot-ws-" + runID }
func (w Workspace) Volume(runID string) string { return "norbot_workspace_" + runID }

func (w Workspace) Ensure(ctx context.Context, runID string) error {
	if err := w.Validate(ctx); err != nil {
		return err
	}
	docker := w.Client()
	if err := os.MkdirAll(w.RunPath(runID), 0o750); err != nil {
		return fmt.Errorf("create artifact directory: %w", err)
	}
	if _, err := docker.Run(ctx, "volume", "create", w.Volume(runID)); err != nil {
		return fmt.Errorf("create run volume: %w", err)
	}
	_, err := docker.Run(ctx, "container", "inspect", w.Name(runID))
	if err == nil {
		return nil
	}
	_, err = docker.Run(ctx, "run", "-d", "--name", w.Name(runID), "--label", "norbot.run_id="+runID, "--label", "norbot.role=workspace", "-v", w.Volume(runID)+":/workspace", "alpine:3.21", "sleep", "infinity")
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
	targetDir := filepath.ToSlash(filepath.Dir(relative))
	docker := w.Client()
	if _, err := docker.Run(ctx, "exec", w.Name(runID), "mkdir", "-p", "/workspace/"+targetDir); err != nil {
		return err
	}
	_, err := docker.Run(ctx, "cp", source, w.Name(runID)+":/workspace/"+relative)
	return err
}

func (w Workspace) MirrorGeneratedApp(ctx context.Context, runID string) error {
	source := filepath.Join(w.RunPath(runID), "generated-app")
	if _, err := os.Stat(source); err != nil {
		return err
	}
	docker := w.Client()
	if _, err := docker.Run(ctx, "exec", w.Name(runID), "mkdir", "-p", "/workspace/generated-app"); err != nil {
		return err
	}
	_, err := docker.Run(ctx, "cp", source+"/.", w.Name(runID)+":/workspace/generated-app")
	return err
}

func (w Workspace) SyncGeneratedApp(ctx context.Context, runID string) error {
	target := filepath.Join(w.RunPath(runID), "generated-app")
	if err := os.MkdirAll(target, 0o750); err != nil {
		return err
	}
	_, err := w.Client().Run(ctx, "cp", w.Name(runID)+":/workspace/generated-app/.", target)
	return err
}

func (w Workspace) RunCLI(ctx context.Context, runID string, stage domain.Stage, image, network string, command []string, prompt, credentialEnv, credentialSecret, credentialSecretKey string) (string, error) {
	if len(command) == 0 {
		return "", fmt.Errorf("empty cli command")
	}
	if image == "" {
		return "", fmt.Errorf("cli runner image is required")
	}
	if network == "" {
		network = "none"
	}
	workspaceMount := w.Volume(runID) + ":/workspace:ro"
	if stage == domain.StageBuilder {
		workspaceMount = w.Volume(runID) + ":/workspace"
	}
	args := []string{"run", "--rm", "-i", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=256", "--memory=4g", "--cpus=2", "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m", "--label", "norbot.run_id=" + runID, "--label", "norbot.role=agent", "--label", "norbot.stage=" + string(stage), "--network", network, "-v", workspaceMount, "-w", "/workspace"}
	if credentialEnv != "" {
		if value, ok := os.LookupEnv(credentialEnv); ok {
			args = append(args, "-e", credentialEnv+"="+value)
		}
	}
	args = append(args, image)
	args = append(args, command...)
	output, err := w.Client().RunInput(ctx, prompt, args...)
	if err != nil {
		return string(output), fmt.Errorf("workspace cli: %w", err)
	}
	return string(output), nil
}

func (w Workspace) RunSandbox(ctx context.Context, runID string, request SandboxRequest, policy config.Sandbox) (SandboxResult, error) {
	if len(request.Command) == 0 {
		return SandboxResult{}, fmt.Errorf("sandbox command is required")
	}
	p := policy.Normalized()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.TimeoutS)*time.Second)
	defer cancel()
	started := time.Now()
	network := "none"
	args := []string{"run", "--rm", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=128", "--memory", fmt.Sprintf("%dm", p.MemoryMiB), "--cpus", fmt.Sprintf("%.3f", float64(p.CPUMilli)/1000), "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m", "--tmpfs", "/scratch:rw,noexec,nosuid,size=64m", "-v", w.Volume(runID) + ":/workspace:ro", "-w", "/workspace"}
	if len(request.AllowedHosts) > 0 {
		if p.EgressProxyURL == "" || p.EgressProxySecret == "" {
			return SandboxResult{}, fmt.Errorf("sandbox egress requires configured managed proxy")
		}
		network = "bridge"
		args = append(args, "--network", network, "-e", "HTTPS_PROXY="+p.EgressProxyURL, "-e", "HTTP_PROXY="+p.EgressProxyURL, "-e", "NO_PROXY=")
	} else {
		args = append(args, "--network", "none")
	}
	image := p.Image
	if request.Image != "" {
		image = request.Image
	}
	args = append(args, image)
	args = append(args, request.Command...)
	out, err := w.Client().Run(ctx, args...)
	result := SandboxResult{Output: string(out), DurationMS: time.Since(started).Milliseconds(), Network: network}
	if err != nil {
		return result, err
	}
	return result, nil
}

func (w Workspace) Cleanup(ctx context.Context, runID string) error {
	docker := w.Client()
	_, _ = docker.Run(ctx, "rm", "-f", w.Name(runID))
	if _, err := docker.Run(ctx, "volume", "inspect", w.Volume(runID)); err != nil {
		return nil
	}
	_, err := docker.Run(ctx, "volume", "rm", w.Volume(runID))
	return err
}

type Capacity struct {
	CPUs                int                     `json:"cpus"`
	MemoryBytes         int64                   `json:"memory_bytes"`
	DockerAvailable     bool                    `json:"docker_available"`
	KubernetesAvailable bool                    `json:"kubernetes_available"`
	Target              domain.DeploymentTarget `json:"target"`
	ConfiguredWorkers   int                     `json:"configured_workers"`
	RecommendedWorkers  int                     `json:"recommended_workers"`
	Recommendation      string                  `json:"recommendation"`
	QuotaWorkers        int                     `json:"quota_workers,omitempty"`
	QuotaFactors        []QuotaFactor           `json:"quota_factors,omitempty"`
}

type QuotaFactor struct {
	ProviderID                  string `json:"provider_id"`
	ConfiguredLimit             int    `json:"configured_limit,omitempty"`
	ConfiguredRequestsPerMinute int    `json:"configured_requests_per_minute,omitempty"`
	ObservedRemaining           *int   `json:"observed_remaining,omitempty"`
}

func DetectCapacity(ctx context.Context, docker DockerClient, configuredWorkers, maxWorkers int, runner CommandRunner) Capacity {
	capacity := Capacity{CPUs: runtime.NumCPU(), ConfiguredWorkers: configuredWorkers, Target: domain.DeploymentDocker}
	capacity.MemoryBytes = memoryBytes(ctx, runner)
	if err := docker.Validate(ctx); err == nil {
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
	Docker       DockerClient
	DockerBin    string
	Runner       CommandRunner
	BrowserImage string
}

func (d Deployment) Client() DockerClient {
	if d.Docker.Bin != "" || d.Docker.Runner != nil || d.Docker.err != nil {
		return d.Docker
	}
	return LegacyDockerClient(d.DockerBin, d.Runner)
}

func (d Deployment) Validate(ctx context.Context) error { return d.Client().Validate(ctx) }

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
	var value strings.Builder
	for _, character := range strings.ToLower(runID) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			value.WriteRune(character)
		} else {
			value.WriteByte('-')
		}
	}
	name := strings.Trim(value.String(), "-")
	if name == "" {
		name = "app"
	}
	if len(name) > 48 {
		name = name[:48]
	}
	return "norbot-" + name
}

func ApplicationID(run domain.Run) string {
	if run.AppID != "" {
		return run.AppID
	}
	return run.ID
}

func (d Deployment) Deploy(ctx context.Context, run domain.Run, root string) (string, error) {
	if err := d.Validate(ctx); err != nil {
		return "", err
	}
	if _, err := PrepareDeploymentSource(root, run); err != nil {
		return "", err
	}
	port, err := ReservePort()
	if err != nil {
		return "", err
	}
	docker := d.Client()
	appID := ApplicationID(run)
	project := ProjectName(appID)
	appRoot := filepath.Join(root, "generated-app")
	images, err := d.buildImages(ctx, docker, run, appRoot)
	if err != nil {
		return "", err
	}
	if err := d.deleteApplication(ctx, docker, appID, true); err != nil {
		return "", err
	}
	network := project + "-network"
	ingressNetwork := project + "-ingress"
	if _, err := docker.Run(ctx, "network", "create", "--internal", "--label", "norbot.managed=true", "--label", "norbot.app_id="+appID, network); err != nil && !strings.Contains(strings.ToLower(err.Error()), "already exists") {
		return "", fmt.Errorf("create deployment network: %w", err)
	}
	if _, err := docker.Run(ctx, "network", "create", "--label", "norbot.managed=true", "--label", "norbot.app_id="+appID, ingressNetwork); err != nil && !strings.Contains(strings.ToLower(err.Error()), "already exists") {
		_, _ = docker.Run(context.Background(), "network", "rm", network)
		return "", fmt.Errorf("create deployment ingress network: %w", err)
	}
	started := []string{}
	fail := func(cause error) (string, error) {
		for _, container := range started {
			_, _ = docker.Run(context.Background(), "rm", "-f", container)
		}
		_, _ = docker.Run(context.Background(), "network", "rm", ingressNetwork)
		_, _ = docker.Run(context.Background(), "network", "rm", network)
		return "", cause
	}
	if run.Profile != domain.ProfileFrontend {
		name, err := d.runContainer(ctx, docker, run, network, "backend", images["backend"], 0)
		if err != nil {
			return fail(err)
		}
		started = append(started, name)
		if err := d.waitHTTP(ctx, docker, network, "http://backend:8000/api/health"); err != nil {
			return fail(err)
		}
	}
	name, err := d.runContainer(ctx, docker, run, network, "frontend", images["frontend"], 0)
	if err != nil {
		return fail(err)
	}
	started = append(started, name)
	if err := d.waitHTTP(ctx, docker, network, "http://frontend:8080/"); err != nil {
		return fail(err)
	}
	ingress, err := d.runIngress(ctx, docker, run, ingressNetwork, images["ingress"], port)
	if err != nil {
		return fail(err)
	}
	started = append(started, ingress)
	if _, err := docker.Run(ctx, "network", "connect", "--alias", "ingress", network, ingress); err != nil {
		return fail(fmt.Errorf("connect deployment ingress: %w", err))
	}
	if err := d.waitHTTP(ctx, docker, network, "http://ingress:8080/"); err != nil {
		return fail(err)
	}
	return "http://127.0.0.1:" + strconv.Itoa(port), nil
}

func (d Deployment) Verify(ctx context.Context, run domain.Run, root string) error {
	_, err := d.VerifyAcceptance(ctx, run, root, domain.AcceptanceContract{})
	return err
}

func (d Deployment) VerifyAcceptance(ctx context.Context, run domain.Run, root string, acceptance domain.AcceptanceContract) (map[string]any, error) {
	if err := d.Validate(ctx); err != nil {
		return nil, err
	}
	if _, err := PrepareDeploymentSource(root, run); err != nil {
		return nil, err
	}
	verificationRun := run
	verificationRun.AppID = "verify-" + run.ID
	docker := d.Client()
	appRoot := filepath.Join(root, "generated-app")
	images, err := d.buildImages(ctx, docker, verificationRun, appRoot)
	if err != nil {
		return nil, err
	}
	appID := ApplicationID(verificationRun)
	project := ProjectName(appID)
	if err := d.deleteApplication(ctx, docker, appID, false); err != nil {
		return nil, err
	}
	network := project + "-network"
	if _, err := docker.Run(ctx, "network", "create", "--internal", "--label", "norbot.managed=true", "--label", "norbot.app_id="+appID, network); err != nil && !strings.Contains(strings.ToLower(err.Error()), "already exists") {
		return nil, fmt.Errorf("create verification network: %w", err)
	}
	defer d.deleteApplication(context.Background(), docker, appID, false)
	if verificationRun.Profile != domain.ProfileFrontend {
		if _, err := d.runContainer(ctx, docker, verificationRun, network, "backend", images["backend"], 0); err != nil {
			return nil, err
		}
		if err := d.waitHTTP(ctx, docker, network, "http://backend:8000/api/health"); err != nil {
			return nil, err
		}
	}
	if _, err := d.runContainer(ctx, docker, verificationRun, network, "frontend", images["frontend"], 0); err != nil {
		return nil, err
	}
	if err := d.waitHTTP(ctx, docker, network, "http://frontend:8080/"); err != nil {
		return nil, err
	}
	if acceptance.Empty() {
		return map[string]any{"status": "skipped", "reason": "no acceptance contract"}, nil
	}
	return d.runAcceptance(ctx, docker, verificationRun, network, acceptance)
}

func (d Deployment) Stop(ctx context.Context, appID, _ string) error {
	docker := d.Client()
	ids, err := d.applicationContainers(ctx, docker, appID, false, "")
	if err != nil || len(ids) == 0 {
		return err
	}
	_, err = docker.Run(ctx, append([]string{"stop"}, ids...)...)
	return err
}

func (d Deployment) Start(ctx context.Context, appID, _ string) error {
	docker := d.Client()
	ids, err := d.applicationContainers(ctx, docker, appID, true, "")
	if err != nil || len(ids) == 0 {
		return err
	}
	_, err = docker.Run(ctx, append([]string{"start"}, ids...)...)
	return err
}

func (d Deployment) Delete(ctx context.Context, appID, _ string) error {
	return d.deleteApplication(ctx, d.Client(), appID, true)
}

func (d Deployment) Status(ctx context.Context, appID, _ string) (DeploymentStatus, error) {
	project := ProjectName(appID)
	output, err := d.Client().Run(ctx, "ps", "-a", "--filter", "label=norbot.managed=true", "--filter", "label=norbot.app_id="+appID, "--format", "{{json .}}")
	if err != nil {
		return DeploymentStatus{}, err
	}
	services := []map[string]any{}
	trimmed := strings.TrimSpace(string(output))
	for _, line := range strings.Split(trimmed, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var service map[string]any
		if err := json.Unmarshal([]byte(line), &service); err != nil {
			return DeploymentStatus{}, fmt.Errorf("decode Docker status: %w", err)
		}
		services = append(services, service)
	}
	return DeploymentStatus{Target: d.Target(), Project: project, Services: services}, nil
}

func (d Deployment) Logs(ctx context.Context, appID, _ string, lines int) (string, error) {
	if lines < 1 || lines > 10000 {
		return "", fmt.Errorf("log line limit must be 1-10000")
	}
	docker := d.Client()
	ids, err := d.applicationContainers(ctx, docker, appID, true, "")
	if err != nil {
		return "", err
	}
	var output strings.Builder
	for _, id := range ids {
		logs, err := docker.Run(ctx, "logs", "--tail", strconv.Itoa(lines), id)
		if err != nil {
			return "", err
		}
		output.Write(logs)
	}
	return output.String(), nil
}

func (d Deployment) InvokeAgent(ctx context.Context, run domain.Run, root string, input AgentInvocation) (AgentResponse, error) {
	if run.Profile != domain.ProfileAgentic {
		return AgentResponse{}, fmt.Errorf("run is not an agentic application")
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return AgentResponse{}, err
	}
	docker := d.Client()
	ids, err := d.applicationContainers(ctx, docker, ApplicationID(run), false, "backend")
	if err != nil {
		return AgentResponse{}, err
	}
	if len(ids) != 1 {
		return AgentResponse{}, fmt.Errorf("expected one managed backend container, found %d", len(ids))
	}
	output, err := docker.RunInput(ctx, string(payload), "exec", "-i", ids[0], "wget", "-qO-", "--header=Content-Type: application/json", "--post-file=-", "http://127.0.0.1:8000/api/agents/run")
	if err != nil {
		return AgentResponse{}, fmt.Errorf("invoke agent: %w: %s", err, tail(string(output), 1000))
	}
	var response AgentResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return AgentResponse{}, fmt.Errorf("decode agent response: %w", err)
	}
	if response.State == "" {
		if response.Status != "" {
			response.State = response.Status
		} else {
			response.State = "completed"
		}
	}
	return response, nil
}

func (d Deployment) buildImages(ctx context.Context, docker DockerClient, run domain.Run, appRoot string) (map[string]string, error) {
	images := map[string]string{}
	project := ProjectName(ApplicationID(run))
	for _, component := range []string{"frontend", "ingress"} {
		image := project + "-" + component + ":" + shortImageID(run.ID)
		dockerfile := filepath.Join(appRoot, filepath.FromSlash(deploymentDirectory), component+".Dockerfile")
		if _, err := docker.Run(ctx, "build", "--label", "norbot.managed=true", "--label", "norbot.app_id="+ApplicationID(run), "--label", "norbot.run_id="+run.ID, "--file", dockerfile, "--tag", image, appRoot); err != nil {
			return nil, fmt.Errorf("build %s image: %w", component, err)
		}
		images[component] = image
	}
	if run.Profile != domain.ProfileFrontend {
		component := "backend"
		image := project + "-" + component + ":" + shortImageID(run.ID)
		dockerfile := filepath.Join(appRoot, filepath.FromSlash(deploymentDirectory), component+".Dockerfile")
		if _, err := docker.Run(ctx, "build", "--label", "norbot.managed=true", "--label", "norbot.app_id="+ApplicationID(run), "--label", "norbot.run_id="+run.ID, "--file", dockerfile, "--tag", image, appRoot); err != nil {
			return nil, fmt.Errorf("build %s image: %w", component, err)
		}
		images[component] = image
	}
	return images, nil
}

func (d Deployment) runContainer(ctx context.Context, docker DockerClient, run domain.Run, network, component, image string, publicPort int) (string, error) {
	name := ProjectName(ApplicationID(run)) + "-" + component + "-" + shortImageID(run.ID)
	args := []string{"run", "-d", "--name", name, "--network", network, "--network-alias", component, "--label", "norbot.managed=true", "--label", "norbot.app_id=" + ApplicationID(run), "--label", "norbot.run_id=" + run.ID, "--label", "norbot.role=application", "--label", "norbot.component=" + component, "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=128", "--memory=512m", "--cpus=0.5", "--tmpfs=/tmp:rw,noexec,nosuid,size=64m", "--tmpfs=/var/run:rw,noexec,nosuid,size=16m"}
	if component == "frontend" && publicPort > 0 {
		args = append(args, "--publish", "127.0.0.1:"+strconv.Itoa(publicPort)+":8080")
	} else if component == "backend" {
		args = append(args, "--env", "PORT=8000")
	}
	args = append(args, image)
	if _, err := docker.Run(ctx, args...); err != nil {
		return "", fmt.Errorf("start %s container: %w", component, err)
	}
	return name, nil
}

func (d Deployment) runIngress(ctx context.Context, docker DockerClient, run domain.Run, network, image string, publicPort int) (string, error) {
	name := ProjectName(ApplicationID(run)) + "-ingress-" + shortImageID(run.ID)
	args := []string{"run", "-d", "--name", name, "--network", network, "--network-alias", "ingress", "--label", "norbot.managed=true", "--label", "norbot.app_id=" + ApplicationID(run), "--label", "norbot.run_id=" + run.ID, "--label", "norbot.role=ingress", "--label", "norbot.component=ingress", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=128", "--memory=128m", "--cpus=0.25", "--tmpfs=/tmp:rw,noexec,nosuid,size=64m", "--publish", "127.0.0.1:" + strconv.Itoa(publicPort) + ":8080", image, "reverse-proxy", "--from", ":8080", "--to", "frontend:8080"}
	if _, err := docker.Run(ctx, args...); err != nil {
		return "", fmt.Errorf("start deployment ingress: %w", err)
	}
	return name, nil
}

func (d Deployment) waitHTTP(ctx context.Context, docker DockerClient, network, endpoint string) error {
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var last error
	for {
		if _, err := docker.Run(ctx, "run", "--rm", "--network", network, "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=64", "--memory=64m", "--cpus=0.25", "--tmpfs=/tmp:rw,noexec,nosuid,size=16m", "curlimages/curl:8.12.1", "--connect-timeout", "2", "--max-time", "5", "-fsS", endpoint); err == nil {
			return nil
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for %s: %w", endpoint, last)
		case <-ticker.C:
		}
	}
}

func (d Deployment) applicationContainers(ctx context.Context, docker DockerClient, appID string, all bool, component string) ([]string, error) {
	args := []string{"ps"}
	if all {
		args = append(args, "-a")
	}
	args = append(args, "-q", "--filter", "label=norbot.managed=true", "--filter", "label=norbot.app_id="+appID)
	if component != "" {
		args = append(args, "--filter", "label=norbot.component="+component)
	}
	output, err := docker.Run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(output)), nil
}

func (d Deployment) deleteApplication(ctx context.Context, docker DockerClient, appID string, legacy bool) error {
	ids, err := d.applicationContainers(ctx, docker, appID, true, "")
	if err != nil {
		return err
	}
	if len(ids) > 0 {
		if _, err := docker.Run(ctx, append([]string{"rm", "-f"}, ids...)...); err != nil {
			return err
		}
	}
	project := ProjectName(appID)
	_, _ = docker.Run(ctx, "network", "rm", project+"-network")
	_, _ = docker.Run(ctx, "network", "rm", project+"-ingress")
	if !legacy {
		return nil
	}
	legacyIDs, err := docker.Run(ctx, "ps", "-aq", "--filter", "label=com.docker.compose.project="+project)
	if err == nil && len(strings.Fields(string(legacyIDs))) > 0 {
		if _, err := docker.Run(ctx, append([]string{"rm", "-f"}, strings.Fields(string(legacyIDs))...)...); err != nil {
			return err
		}
	}
	for _, resource := range []string{"network", "volume"} {
		output, err := docker.Run(ctx, resource, "ls", "-q", "--filter", "label=com.docker.compose.project="+project)
		if err != nil || len(strings.Fields(string(output))) == 0 {
			continue
		}
		_, _ = docker.Run(ctx, append([]string{resource, "rm"}, strings.Fields(string(output))...)...)
	}
	return nil
}

func shortImageID(value string) string {
	value = strings.ToLower(value)
	var out strings.Builder
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			out.WriteRune(character)
		}
	}
	if out.Len() == 0 {
		return "run"
	}
	if out.Len() > 12 {
		return out.String()[:12]
	}
	return out.String()
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
