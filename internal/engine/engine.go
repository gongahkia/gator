package engine

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/extension"
	"github.com/gongahkia/norbot/internal/provider"
	"github.com/gongahkia/norbot/internal/runtime"
	"github.com/gongahkia/norbot/internal/store"
	"github.com/gongahkia/norbot/internal/telemetry"
)

type CreateRunInput struct {
	Prompt           string                  `json:"prompt"`
	Profile          domain.Profile          `json:"profile"`
	Providers        map[domain.Stage]string `json:"providers"`
	DeploymentTarget domain.DeploymentTarget `json:"deployment_target"`
	PublicIngress    bool                    `json:"public_ingress"`
	MaxFixes         int                     `json:"max_fixes"`
	SkillDigests     []string                `json:"skill_digests"`
}

type ApprovalInput struct {
	Action     domain.ApprovalAction `json:"action"`
	Feedback   string                `json:"feedback"`
	RevisionID int64                 `json:"revision_id"`
}

type DeploymentInfo struct {
	Deployment domain.Deployment        `json:"deployment"`
	Runtime    runtime.DeploymentStatus `json:"runtime"`
}

type RuntimeOptions struct {
	DefaultTarget        domain.DeploymentTarget `json:"default_target"`
	KubernetesConfigured bool                    `json:"kubernetes_configured"`
	IngressConfigured    bool                    `json:"ingress_configured"`
}

type Service struct {
	store            *store.Store
	config           config.Config
	dockerWorkspace  runtime.Workspace
	dockerDeployment runtime.Deployment
	extensions       *extension.Registry
	log              *slog.Logger
	metrics          *telemetry.Metrics
}

func New(st *store.Store, cfg config.Config, logger *slog.Logger) *Service {
	return NewWithExtensions(st, cfg, logger, extension.NewRegistry())
}

func NewWithExtensions(st *store.Store, cfg config.Config, logger *slog.Logger, extensions *extension.Registry) *Service {
	workspace := runtime.Workspace{DockerBin: cfg.DockerBin, ArtifactsDir: cfg.ArtifactsDir, Runner: runtime.OSRunner{}}
	if extensions == nil {
		extensions = extension.NewRegistry()
	}
	return &Service{
		store: st, config: cfg, dockerWorkspace: workspace,
		dockerDeployment: runtime.Deployment{DockerBin: cfg.DockerBin, Runner: runtime.OSRunner{}},
		extensions:       extensions, log: logger, metrics: telemetry.NewMetrics(),
	}
}

func (s *Service) Metrics() *telemetry.Metrics { return s.metrics }

func (s *Service) RuntimeOptions() RuntimeOptions {
	k := s.config.Manifest.Runtime.Kubernetes
	configured := k.Kubeconfig != "" && k.Namespace != "" && k.ServiceAccount != "" && k.RegistryRepository != "" && k.RegistryPullSecret != ""
	return RuntimeOptions{DefaultTarget: s.config.Manifest.DefaultTarget(), KubernetesConfigured: configured, IngressConfigured: configured && k.IngressClass != "" && k.IngressBaseDomain != ""}
}

func (s *Service) backendFor(ctx context.Context, target domain.DeploymentTarget) (runtime.WorkspaceBackend, runtime.DeploymentBackend, error) {
	if target == domain.DeploymentDocker {
		return s.dockerWorkspace, s.dockerDeployment, nil
	}
	if target != domain.DeploymentKubernetes {
		return nil, nil, fmt.Errorf("unsupported deployment target %q", target)
	}
	kube, err := runtime.NewKubernetesRuntime(s.config.Manifest.Runtime.Kubernetes, s.config.ArtifactsDir)
	if err != nil {
		return nil, nil, err
	}
	if err := kube.Validate(ctx); err != nil {
		return nil, nil, err
	}
	return kube, kube, nil
}

func (s *Service) backendForRun(ctx context.Context, run domain.Run) (runtime.WorkspaceBackend, runtime.DeploymentBackend, error) {
	return s.backendFor(ctx, run.DeploymentTarget)
}

func (s *Service) CreateRun(ctx context.Context, input CreateRunInput) (domain.Run, error) {
	input.Prompt = strings.TrimSpace(input.Prompt)
	if input.Prompt == "" {
		return domain.Run{}, fmt.Errorf("prompt is required")
	}
	if !input.Profile.Valid() {
		return domain.Run{}, fmt.Errorf("unsupported profile %q", input.Profile)
	}
	target := input.DeploymentTarget
	if target == "" {
		target = s.config.Manifest.DefaultTarget()
	}
	if !target.Valid() {
		return domain.Run{}, fmt.Errorf("unsupported deployment_target %q", target)
	}
	if input.PublicIngress && target != domain.DeploymentKubernetes {
		return domain.Run{}, fmt.Errorf("public_ingress requires deployment_target kubernetes")
	}
	if input.PublicIngress && (s.config.Manifest.Runtime.Kubernetes.IngressClass == "" || s.config.Manifest.Runtime.Kubernetes.IngressBaseDomain == "") {
		return domain.Run{}, fmt.Errorf("public_ingress requires configured kubernetes ingress")
	}
	workspace, _, err := s.backendFor(ctx, target)
	if err != nil {
		return domain.Run{}, err
	}
	providers, err := s.normalizeProviders(input.Providers)
	if err != nil {
		return domain.Run{}, err
	}
	for _, digest := range input.SkillDigests {
		if _, err := s.store.ActiveSkill(ctx, digest); err != nil {
			return domain.Run{}, fmt.Errorf("selected skill %q is not active", digest)
		}
	}
	id, err := newID()
	if err != nil {
		return domain.Run{}, err
	}
	now := time.Now().UTC()
	maxFixes := input.MaxFixes
	if maxFixes == 0 {
		maxFixes = s.config.Manifest.Workflow.MaxFixes
	}
	if maxFixes == 0 {
		maxFixes = 2
	}
	if maxFixes < 0 || maxFixes > 10 {
		return domain.Run{}, fmt.Errorf("max_fixes must be between 0 and 10")
	}
	run := domain.Run{ID: id, Prompt: input.Prompt, Profile: input.Profile, DeploymentTarget: target, PublicIngress: input.PublicIngress, MaxFixes: maxFixes, Stage: domain.StagePlanner, Status: domain.StatusQueued, Providers: providers, Graph: domain.DefaultGraph(), CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateRun(ctx, run); err != nil {
		return domain.Run{}, err
	}
	if len(input.SkillDigests) > 0 {
		if err := s.store.SelectRunSkills(ctx, id, input.SkillDigests); err != nil {
			return domain.Run{}, err
		}
	}
	if err := workspace.Ensure(ctx, id); err != nil {
		return domain.Run{}, err
	}
	if err := s.store.Enqueue(ctx, id, domain.StagePlanner, 1); err != nil {
		return domain.Run{}, err
	}
	return s.store.GetRun(ctx, id)
}

func (s *Service) normalizeProviders(requested map[domain.Stage]string) (map[domain.Stage]string, error) {
	result := map[domain.Stage]string{domain.StageDeployer: "local-deployer"}
	for _, stage := range []domain.Stage{domain.StagePlanner, domain.StageBuilder, domain.StageVerifier} {
		id := strings.TrimSpace(requested[stage])
		if id == "" {
			for _, candidate := range s.config.Manifest.Providers {
				if _, ok := s.config.Manifest.Provider(candidate.ID, stage); ok {
					id = candidate.ID
					break
				}
			}
		}
		if _, ok := s.config.Manifest.Provider(id, stage); !ok {
			return nil, fmt.Errorf("no configured provider %q for %s", id, stage)
		}
		result[stage] = id
	}
	return result, nil
}

func (s *Service) UpdateGraph(ctx context.Context, runID string, graph domain.Graph) (domain.Run, error) {
	if err := graph.Validate(); err != nil {
		return domain.Run{}, err
	}
	return s.store.UpdateGraph(ctx, runID, graph)
}

func (s *Service) Approve(ctx context.Context, runID string, input ApprovalInput) (domain.Run, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return domain.Run{}, err
	}
	if input.Action == domain.ApprovalApprove && run.Status == domain.StatusAwaiting && run.Stage == domain.StageBuilder {
		revision, err := s.store.LatestRevision(ctx, runID)
		if err != nil {
			return domain.Run{}, err
		}
		if input.RevisionID != 0 && input.RevisionID != revision.ID {
			return domain.Run{}, fmt.Errorf("revision is no longer current")
		}
		if revision.State != "proposed" {
			return domain.Run{}, fmt.Errorf("code revision is no longer proposed")
		}
		workspace, _, err := s.backendForRun(ctx, run)
		if err != nil {
			return domain.Run{}, err
		}
		if err := applyRevision(workspace, run, revision); err != nil {
			return domain.Run{}, err
		}
		if err := workspace.MirrorGeneratedApp(ctx, runID); err != nil {
			return domain.Run{}, err
		}
		if err := s.store.ApproveRevision(ctx, runID, revision.ID); err != nil {
			return domain.Run{}, err
		}
	}
	return s.store.Approve(ctx, runID, input.Action, strings.TrimSpace(input.Feedback))
}

func (s *Service) Cancel(ctx context.Context, runID string) (domain.Run, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return domain.Run{}, err
	}
	if !run.Status.Terminal() {
		run, err = s.store.Approve(ctx, runID, domain.ApprovalAbandon, "")
		if err != nil {
			return domain.Run{}, err
		}
	}
	workspace, deployment, backendErr := s.backendForRun(ctx, run)
	if backendErr != nil {
		return domain.Run{}, backendErr
	}
	_ = deployment.Delete(ctx, runID, workspace.RunPath(runID))
	if err := workspace.Cleanup(ctx, runID); err != nil {
		s.log.Warn("workspace cleanup failed after cancel", "run_id", runID, "error", err)
	}
	_ = s.store.RecordEvent(ctx, runID, "run_cancelled", "Run cancelled; pinned workspace and deployment cleanup requested", map[string]any{"deployment_target": run.DeploymentTarget})
	return s.store.GetRun(ctx, runID)
}

func (s *Service) Cleanup(ctx context.Context, runID string) error {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if !run.Status.Terminal() {
		return fmt.Errorf("cancel an active run before cleanup")
	}
	workspace, deployment, backendErr := s.backendForRun(ctx, run)
	if backendErr != nil {
		return backendErr
	}
	_ = deployment.Delete(ctx, runID, workspace.RunPath(runID))
	if err := workspace.Cleanup(ctx, runID); err != nil {
		return err
	}
	return s.store.RecordEvent(ctx, runID, "run_cleanup_completed", "Pinned workspace and deployment cleaned; host artifacts retained", map[string]any{"deployment_target": run.DeploymentTarget})
}

func (s *Service) DeploymentStatus(ctx context.Context, runID string) (DeploymentInfo, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return DeploymentInfo{}, err
	}
	deployment, err := s.store.GetDeployment(ctx, runID)
	if err != nil {
		return DeploymentInfo{}, err
	}
	workspace, backend, err := s.backendForRun(ctx, run)
	if err != nil {
		return DeploymentInfo{}, err
	}
	status, err := backend.Status(ctx, runID, workspace.RunPath(runID))
	if err != nil {
		return DeploymentInfo{}, err
	}
	return DeploymentInfo{Deployment: deployment, Runtime: status}, nil
}

func (s *Service) DeploymentLogs(ctx context.Context, runID string, lines int) (string, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return "", err
	}
	if _, err := s.store.GetDeployment(ctx, runID); err != nil {
		return "", err
	}
	workspace, backend, err := s.backendForRun(ctx, run)
	if err != nil {
		return "", err
	}
	return backend.Logs(ctx, runID, workspace.RunPath(runID), lines)
}

func (s *Service) InvokeAgent(ctx context.Context, runID string, input runtime.AgentInvocation) (runtime.AgentResponse, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return runtime.AgentResponse{}, err
	}
	if run.Profile != domain.ProfileAgentic {
		return runtime.AgentResponse{}, fmt.Errorf("channel accounts require an agentic deployment")
	}
	if input.Role == "" {
		input.Role = "operator"
	}
	if input.Provider.Kind == "" {
		providerID := run.Providers[domain.StageBuilder]
		providerConfig, ok := s.config.Manifest.Provider(providerID, domain.StageBuilder)
		if !ok || (providerConfig.Kind != "openai_responses" && providerConfig.Kind != "openai_compatible" && providerConfig.Kind != "anthropic_messages" && providerConfig.Kind != "gemini_generate_content") {
			return runtime.AgentResponse{}, fmt.Errorf("agentic application requires an HTTP model provider")
		}
		input.Provider = runtime.AgentProvider{Kind: providerConfig.Kind, BaseURL: providerConfig.BaseURL, Model: providerConfig.Model, CredentialEnv: providerConfig.CredentialEnv}
	}
	if _, err := s.store.GetDeployment(ctx, runID); err != nil {
		return runtime.AgentResponse{}, err
	}
	workspace, backend, err := s.backendForRun(ctx, run)
	if err != nil {
		return runtime.AgentResponse{}, err
	}
	invoker, ok := backend.(runtime.AgentBackend)
	if !ok {
		return runtime.AgentResponse{}, fmt.Errorf("runtime does not support private agent invocation")
	}
	return invoker.InvokeAgent(ctx, run, workspace.RunPath(runID), input)
}

func (s *Service) RunSandbox(ctx context.Context, runID string, request runtime.SandboxRequest) (runtime.SandboxResult, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return runtime.SandboxResult{}, err
	}
	workspace, _, err := s.backendForRun(ctx, run)
	if err != nil {
		return runtime.SandboxResult{}, err
	}
	executor, ok := workspace.(runtime.SandboxBackend)
	if !ok {
		return runtime.SandboxResult{}, fmt.Errorf("runtime sandbox is unavailable for %s", run.DeploymentTarget)
	}
	return executor.RunSandbox(ctx, runID, request, s.config.Manifest.Runtime.Sandbox)
}

func (s *Service) StartDeployment(ctx context.Context, runID string) (DeploymentInfo, error) {
	deployment, err := s.store.GetDeployment(ctx, runID)
	if err != nil {
		return DeploymentInfo{}, err
	}
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return DeploymentInfo{}, err
	}
	workspace, backend, err := s.backendForRun(ctx, run)
	if err != nil {
		return DeploymentInfo{}, err
	}
	if err := backend.Start(ctx, runID, workspace.RunPath(runID)); err != nil {
		return DeploymentInfo{}, err
	}
	if err := s.store.UpsertDeployment(ctx, runID, deployment.ProjectName, deployment.PublicURL, "running", ""); err != nil {
		return DeploymentInfo{}, err
	}
	if err := s.store.RecordEvent(ctx, runID, "deployment_started", "Deployment started by operator", nil); err != nil {
		return DeploymentInfo{}, err
	}
	s.metrics.ObserveDeployment("started")
	return s.DeploymentStatus(ctx, runID)
}

func (s *Service) StopDeployment(ctx context.Context, runID string) (domain.Deployment, error) {
	deployment, err := s.store.GetDeployment(ctx, runID)
	if err != nil {
		return domain.Deployment{}, err
	}
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return domain.Deployment{}, err
	}
	workspace, backend, err := s.backendForRun(ctx, run)
	if err != nil {
		return domain.Deployment{}, err
	}
	if err := backend.Stop(ctx, runID, workspace.RunPath(runID)); err != nil {
		return domain.Deployment{}, err
	}
	if err := s.store.UpsertDeployment(ctx, runID, deployment.ProjectName, deployment.PublicURL, "stopped", ""); err != nil {
		return domain.Deployment{}, err
	}
	if err := s.store.RecordEvent(ctx, runID, "deployment_stopped", "Deployment stopped by operator", nil); err != nil {
		return domain.Deployment{}, err
	}
	s.metrics.ObserveDeployment("stopped")
	return s.store.GetDeployment(ctx, runID)
}

func (s *Service) DeleteDeployment(ctx context.Context, runID string) (domain.Deployment, error) {
	deployment, err := s.store.GetDeployment(ctx, runID)
	if err != nil {
		return domain.Deployment{}, err
	}
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return domain.Deployment{}, err
	}
	workspace, backend, err := s.backendForRun(ctx, run)
	if err != nil {
		return domain.Deployment{}, err
	}
	if err := backend.Delete(ctx, runID, workspace.RunPath(runID)); err != nil {
		return domain.Deployment{}, err
	}
	if err := s.store.UpsertDeployment(ctx, runID, deployment.ProjectName, deployment.PublicURL, "deleted", ""); err != nil {
		return domain.Deployment{}, err
	}
	if err := s.store.RecordEvent(ctx, runID, "deployment_deleted", "Deployment deleted by operator", nil); err != nil {
		return domain.Deployment{}, err
	}
	s.metrics.ObserveDeployment("deleted")
	if err := workspace.Cleanup(ctx, runID); err != nil {
		return domain.Deployment{}, err
	}
	return s.store.GetDeployment(ctx, runID)
}

func (s *Service) Capacity(ctx context.Context) runtime.Capacity {
	capacity := runtime.DetectCapacity(ctx, s.config.DockerBin, s.config.Workers, s.config.MaxWorkers, runtime.OSRunner{})
	if s.config.Manifest.DefaultTarget() == domain.DeploymentKubernetes {
		if kube, err := runtime.NewKubernetesRuntime(s.config.Manifest.Runtime.Kubernetes, s.config.ArtifactsDir); err != nil {
			capacity.Target = domain.DeploymentKubernetes
			capacity.Recommendation = "Kubernetes configuration is unavailable: " + err.Error()
		} else if detected, err := kube.Capacity(ctx, s.config.Workers, s.config.MaxWorkers); err != nil {
			capacity.Target = domain.DeploymentKubernetes
			capacity.Recommendation = "Kubernetes capacity is unavailable: " + err.Error()
		} else {
			capacity = detected
		}
	}
	observations, err := s.store.LatestProviderObservations(ctx)
	if err != nil {
		s.log.Warn("load provider capacity observations", "error", err)
		return capacity
	}
	latest := make(map[string]store.ProviderObservation, len(observations))
	for _, observation := range observations {
		latest[observation.ProviderID] = observation
	}
	factors := make([]runtime.QuotaFactor, 0, len(s.config.Manifest.Providers))
	for _, configured := range s.config.Manifest.Providers {
		factor := runtime.QuotaFactor{ProviderID: configured.ID, ConfiguredLimit: configured.Budget.MaxConcurrent, ConfiguredRequestsPerMinute: configured.Budget.RequestsPerMinute}
		if observation, ok := latest[configured.ID]; ok {
			factor.ObservedRemaining = observation.RemainingRequests
		}
		factors = append(factors, factor)
	}
	capacity.ApplyProviderQuotas(factors)
	return capacity
}

func (s *Service) RecommendCapacity(ctx context.Context) (store.CapacityRecommendation, error) {
	capacity := s.Capacity(ctx)
	factors := map[string]any{"target": capacity.Target, "cpus": capacity.CPUs, "memory_bytes": capacity.MemoryBytes, "docker_available": capacity.DockerAvailable, "kubernetes_available": capacity.KubernetesAvailable, "quota_workers": capacity.QuotaWorkers, "providers": capacity.QuotaFactors}
	return s.store.CreateCapacityRecommendation(ctx, capacity.RecommendedWorkers, factors)
}

func (s *Service) AcceptCapacity(ctx context.Context, id int64, workers int) (store.CapacityRecommendation, error) {
	if workers > s.config.MaxWorkers {
		return store.CapacityRecommendation{}, fmt.Errorf("accepted workers exceed NORBOT_MAX_WORKERS")
	}
	return s.store.AcceptCapacityRecommendation(ctx, id, workers)
}

func (s *Service) ProviderOptions() []config.Provider {
	options := make([]config.Provider, len(s.config.Manifest.Providers))
	copy(options, s.config.Manifest.Providers)
	return options
}

func (s *Service) StartWorkers(ctx context.Context) {
	go s.recoverLeases(ctx)
	var workers sync.WaitGroup
	for index := 0; index < s.config.Workers; index++ {
		workers.Add(1)
		go func(workerID int) {
			defer workers.Done()
			s.worker(ctx, workerID)
		}(index + 1)
	}
	go func() { <-ctx.Done(); workers.Wait() }()
}

func (s *Service) worker(ctx context.Context, workerID int) {
	identity := fmt.Sprintf("%s-%d", workerIdentity(), workerID)
	for {
		if ctx.Err() != nil {
			return
		}
		job, claimed, err := s.store.ClaimJob(ctx, identity, 2*time.Minute)
		if err != nil {
			s.log.Error("claim job", "worker", workerID, "error", err)
			sleep(ctx, time.Second)
			continue
		}
		if !claimed {
			sleep(ctx, 300*time.Millisecond)
			continue
		}
		if err := s.executeLeased(ctx, job); err != nil {
			s.log.Error("stage failed", "run_id", job.RunID, "stage", job.Stage, "attempt", job.Attempt, "error", err)
			_ = s.store.FailJob(context.Background(), job, err.Error())
		}
	}
}

func (s *Service) recoverLeases(ctx context.Context) {
	for {
		recovered, err := s.store.RecoverExpiredJobs(ctx)
		if err != nil {
			s.log.Error("recover expired jobs", "error", err)
		} else if recovered > 0 {
			s.log.Warn("interrupted expired jobs", "count", recovered)
		}
		sleep(ctx, 15*time.Second)
		if ctx.Err() != nil {
			return
		}
	}
}

func (s *Service) executeLeased(ctx context.Context, job domain.Job) error {
	stageCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	heartbeatErr := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-stageCtx.Done():
				return
			case <-ticker.C:
				if err := s.store.HeartbeatJob(stageCtx, job, 2*time.Minute); err != nil {
					select {
					case heartbeatErr <- err:
					default:
					}
					cancel()
					return
				}
			}
		}
	}()
	err := s.execute(stageCtx, job)
	close(done)
	select {
	case leaseErr := <-heartbeatErr:
		if err == nil {
			return fmt.Errorf("lease heartbeat: %w", leaseErr)
		}
	default:
	}
	return err
}

func workerIdentity() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "norbot"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

func (s *Service) execute(ctx context.Context, job domain.Job) (err error) {
	ctx, span := otel.Tracer("norbot.engine").Start(ctx, "stage.execute")
	span.SetAttributes(attribute.String("norbot.run_id", job.RunID), attribute.String("norbot.stage", string(job.Stage)), attribute.Int("norbot.attempt", job.Attempt), attribute.String("norbot.worker_id", job.WorkerID))
	finishMetric := s.metrics.StartStage(string(job.Stage))
	defer func() {
		finishMetric(err)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()
	run, err := s.store.GetRun(ctx, job.RunID)
	if err != nil {
		return err
	}
	if run.Status != domain.StatusQueued || run.Stage != job.Stage {
		return fmt.Errorf("stale job")
	}
	workspace, deployment, err := s.backendForRun(ctx, run)
	if err != nil {
		return err
	}
	providerID := run.Providers[job.Stage]
	if err := s.store.MarkStageRunning(ctx, job, providerID); err != nil {
		return err
	}
	if err := workspace.Ensure(ctx, run.ID); err != nil {
		return err
	}
	if job.Stage == domain.StageDeployer {
		return s.deploy(ctx, run, job)
	}
	providerConfig, ok := s.config.Manifest.Provider(providerID, job.Stage)
	if !ok {
		return fmt.Errorf("provider %q is no longer enabled for %s", providerID, job.Stage)
	}
	generatedFiles := []string(nil)
	if job.Stage == domain.StageBuilder && job.Attempt == 1 {
		generatedFiles, err = generateApp(workspace, run, s.config.Manifest.ToolPolicy)
		if err != nil {
			return err
		}
		if err := workspace.MirrorGeneratedApp(ctx, run.ID); err != nil {
			return err
		}
	}
	skills, err := s.store.RunSkills(ctx, run.ID)
	if err != nil {
		return err
	}
	if err := materializeRunSkills(workspace, run.ID, skills); err != nil {
		return err
	}
	prompt := stagePrompt(run, job.Stage, providerConfig.Kind == "cli", skills)
	providerStarted := time.Now()
	providerCtx, providerSpan := otel.Tracer("norbot.provider").Start(ctx, "provider.invoke")
	providerSpan.SetAttributes(attribute.String("norbot.provider_id", providerConfig.ID), attribute.String("norbot.provider_kind", providerConfig.Kind), attribute.String("norbot.stage", string(job.Stage)))
	invoker := provider.Invoker{Workspace: workspace, Extensions: s.extensions}
	result, err := invoker.Invoke(providerCtx, providerConfig, provider.Request{RunID: run.ID, Stage: job.Stage, Prompt: prompt})
	s.metrics.ObserveProvider(providerConfig.ID, time.Since(providerStarted), err)
	if err != nil {
		providerSpan.RecordError(err)
		providerSpan.SetStatus(codes.Error, err.Error())
	}
	providerSpan.End()
	if err != nil {
		return err
	}
	var revisionID *int64
	if result.RateLimit.RemainingRequests != nil || result.RateLimit.ResetAt != nil {
		if err := s.store.RecordProviderObservation(ctx, store.ProviderObservation{ProviderID: providerConfig.ID, RemainingRequests: result.RateLimit.RemainingRequests, ResetAt: result.RateLimit.ResetAt, Metadata: result.Metadata}); err != nil {
			s.log.Warn("record provider quota observation", "provider", providerConfig.ID, "error", err)
		}
	}
	artifact := map[string]any{"stage": job.Stage, "attempt": job.Attempt, "provider": result.Provider, "model": result.Model, "response": result.Text, "metadata": result.Metadata, "created_at": time.Now().UTC()}
	path := filepath.ToSlash(filepath.Join("stage-output", string(job.Stage)+fmt.Sprintf("-%d.json", job.Attempt)))
	if job.Stage == domain.StagePlanner {
		if graph, ok := graphFromResponse(result.Text); ok {
			if err := s.store.SetPlannerGraph(ctx, run.ID, graph); err != nil {
				return err
			}
			run.Graph = graph
		}
	}
	if job.Stage == domain.StageBuilder {
		if providerConfig.Kind == "cli" {
			baseline, err := snapshotGeneratedApp(workspace.RunPath(run.ID))
			if err != nil {
				return err
			}
			if err := workspace.SyncGeneratedApp(ctx, run.ID); err != nil {
				return err
			}
			current, err := snapshotGeneratedApp(workspace.RunPath(run.ID))
			if err != nil {
				return err
			}
			files, err := changedFiles(baseline, current)
			if err != nil {
				return err
			}
			if err := restoreGeneratedApp(workspace.RunPath(run.ID), baseline); err != nil {
				return err
			}
			if err := workspace.MirrorGeneratedApp(ctx, run.ID); err != nil {
				return err
			}
			revision, err := s.store.CreateRevision(ctx, domain.Revision{RunID: run.ID, Kind: revisionKind(job.Attempt), Attempt: job.Attempt, BaselineDigest: digestFiles(baseline), PatchDigest: digestStringMap(files), Files: files, Report: map[string]any{"provider": result.Provider, "model": result.Model}})
			if err != nil {
				return err
			}
			revisionID = &revision.ID
			generatedFiles = mapKeys(files)
		} else {
			files, err := builderFiles(result.Text)
			if err != nil {
				return err
			}
			baseline, err := snapshotGeneratedApp(workspace.RunPath(run.ID))
			if err != nil {
				return err
			}
			revision, err := s.store.CreateRevision(ctx, domain.Revision{RunID: run.ID, Kind: revisionKind(job.Attempt), Attempt: job.Attempt, BaselineDigest: digestFiles(baseline), PatchDigest: digestStringMap(files), Files: files, Report: map[string]any{"provider": result.Provider, "model": result.Model}})
			if err != nil {
				return err
			}
			revisionID = &revision.ID
			generatedFiles = append(generatedFiles, mapKeys(files)...)
		}
		artifact["generated_files"] = generatedFiles
		artifact["revision_id"] = *revisionID
		artifact["review_kind"] = revisionKind(job.Attempt)
	}
	if err := s.recordUsage(ctx, run.ID, job.Stage, revisionID, providerConfig.ID, result, prompt); err != nil {
		s.log.Warn("record provider usage", "run_id", run.ID, "error", err)
	}
	if job.Stage == domain.StageVerifier {
		revision, err := s.store.LatestRevision(ctx, run.ID)
		if err != nil {
			return err
		}
		report, err := verifyApp(ctx, workspace, run)
		if err != nil {
			if report == nil {
				report = map[string]any{}
			}
			report["status"] = "fail"
			report["error"] = err.Error()
		}
		artifact["verification"] = report
		artifact["revision_id"] = revision.ID
		if err := s.store.RecordRevisionReport(ctx, run.ID, revision.ID, report, "tested"); err != nil {
			return err
		}
	}
	encoded, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	if _, err := workspace.WriteArtifact(run.ID, path, encoded); err != nil {
		return err
	}
	if err := workspace.MirrorToVolume(ctx, run.ID, path); err != nil {
		return err
	}
	_ = deployment
	return s.store.MarkStageAwaitingApproval(ctx, job, artifact)
}

func (s *Service) deploy(ctx context.Context, run domain.Run, job domain.Job) error {
	ctx, span := otel.Tracer("norbot.deployment").Start(ctx, "deployment.create")
	defer span.End()
	span.SetAttributes(attribute.String("norbot.run_id", run.ID), attribute.String("norbot.project", runtime.ProjectName(run.ID)))
	workspace, deployment, err := s.backendForRun(ctx, run)
	if err != nil {
		return err
	}
	root := workspace.RunPath(run.ID)
	if err := s.store.UpsertDeployment(ctx, run.ID, runtime.ProjectName(run.ID), "", "building", ""); err != nil {
		return err
	}
	url, err := deployment.Deploy(ctx, run, root)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		s.metrics.ObserveDeployment("failed")
		_ = s.store.UpsertDeployment(ctx, run.ID, runtime.ProjectName(run.ID), "", "failed", err.Error())
		return err
	}
	if err := s.store.UpsertDeployment(ctx, run.ID, runtime.ProjectName(run.ID), url, "running", ""); err != nil {
		return err
	}
	s.metrics.ObserveDeployment("created")
	return s.store.CompleteRun(ctx, job, map[string]any{"project": runtime.ProjectName(run.ID), "public_url": url})
}

func verifyApp(ctx context.Context, workspace runtime.WorkspaceBackend, run domain.Run) (map[string]any, error) {
	if verifier, ok := workspace.(interface {
		Verify(context.Context, domain.Run) (map[string]any, error)
	}); ok && run.DeploymentTarget == domain.DeploymentKubernetes {
		return verifier.Verify(ctx, run)
	}
	docker, ok := workspace.(runtime.Workspace)
	if !ok {
		return nil, fmt.Errorf("workspace has no verifier")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	root := filepath.Join(docker.RunPath(run.ID), "generated-app")
	required := []string{"docker-compose.yml", "frontend/package.json", "frontend/package-lock.json", "frontend/src/main.jsx"}
	if run.Profile != domain.ProfileFrontend {
		required = append(required, "backend/main.go", "backend/go.mod", "backend/go.sum")
	}
	if run.Profile == domain.ProfileAgentic {
		required = append(required, "backend/harness.go", "backend/tool_policy.json")
	}
	for _, relative := range required {
		if _, err := os.Stat(filepath.Join(root, relative)); err != nil {
			return nil, fmt.Errorf("generated app missing %s", relative)
		}
	}
	runner := docker.Runner
	if runner == nil {
		runner = runtime.OSRunner{}
	}
	dockerBin := docker.DockerBin
	if dockerBin == "" {
		dockerBin = "docker"
	}
	project := runtime.ProjectName("verify-" + run.ID)
	port, err := reserveVerificationPort()
	if err != nil {
		return nil, err
	}
	envFile := filepath.Join(root, ".norbot.verify.env")
	if err := os.WriteFile(envFile, []byte("NORBOT_PUBLIC_PORT="+fmt.Sprint(port)+"\n"), 0o600); err != nil {
		return nil, err
	}
	defer os.Remove(envFile)
	compose := []string{"compose", "-p", project, "--project-directory", root, "--env-file", envFile}
	checks := []string{"profile contract", "locked frontend dependencies", "compose config"}
	runCommand := func(name string, args ...string) error {
		if _, err := runner.Run(ctx, name, args...); err != nil {
			return err
		}
		return nil
	}
	if err := runCommand(dockerBin, append(compose, "config", "--quiet")...); err != nil {
		return verificationFailure(ctx, runner, dockerBin, compose, checks, err)
	}
	frontend := []string{"run", "--rm", "-v", docker.Volume(run.ID) + ":/workspace", "-w", "/workspace/generated-app/frontend", "node:22-alpine", "sh", "-ceu", "npm ci && npm test && npm run build && npm audit --omit=dev --audit-level=high"}
	if err := runCommand(dockerBin, frontend...); err != nil {
		return verificationFailure(ctx, runner, dockerBin, compose, checks, fmt.Errorf("frontend test/build/audit: %w", err))
	}
	checks = append(checks, "npm ci/test/build/audit")
	if run.Profile != domain.ProfileFrontend {
		backend := []string{"run", "--rm", "-v", docker.Volume(run.ID) + ":/workspace", "-w", "/workspace/generated-app/backend", "golang:1.26-alpine", "sh", "-ceu", "go test ./... && go build ./... && go install golang.org/x/vuln/cmd/govulncheck@v1.6.0 && govulncheck ./..."}
		if err := runCommand(dockerBin, backend...); err != nil {
			return verificationFailure(ctx, runner, dockerBin, compose, checks, fmt.Errorf("Go test/build/govulncheck: %w", err))
		}
		checks = append(checks, "go test/build/govulncheck")
	}
	deployed := false
	defer func() {
		if deployed {
			_, _ = runner.Run(context.Background(), dockerBin, append(compose, "down", "--remove-orphans", "--volumes")...)
		}
	}()
	if err := runCommand(dockerBin, append(compose, "up", "--build", "-d", "--wait", "--wait-timeout", "90")...); err != nil {
		return verificationFailure(ctx, runner, dockerBin, compose, checks, fmt.Errorf("compose smoke startup: %w", err))
	}
	deployed = true
	network := project + "_default"
	if err := runCommand(dockerBin, "run", "--rm", "--network", network, "curlimages/curl:8.12.1", "-fsS", "http://frontend/"); err != nil {
		return verificationFailure(ctx, runner, dockerBin, compose, checks, fmt.Errorf("frontend smoke: %w", err))
	}
	if run.Profile != domain.ProfileFrontend {
		if err := runCommand(dockerBin, "run", "--rm", "--network", network, "curlimages/curl:8.12.1", "-fsS", "http://backend:8000/api/health"); err != nil {
			return verificationFailure(ctx, runner, dockerBin, compose, checks, fmt.Errorf("backend smoke: %w", err))
		}
	}
	checks = append(checks, "compose build/health/smoke")
	return map[string]any{"status": "pass", "checks": checks, "summary": "Locked dependency, build, test, vulnerability scan, Compose health, and network smoke checks passed. Operator approval is required before deployment."}, nil
}

func verificationFailure(ctx context.Context, runner runtime.CommandRunner, dockerBin string, compose, checks []string, cause error) (map[string]any, error) {
	logs, _ := runner.Run(ctx, dockerBin, append(compose, "logs", "--no-color", "--tail", "200")...)
	return map[string]any{"status": "fail", "checks": checks, "logs": string(logs)}, fmt.Errorf("verification failed: %w", cause)
}

func reserveVerificationPort() (int, error) { return runtime.ReservePort() }

func stagePrompt(run domain.Run, stage domain.Stage, isCLI bool, skills []domain.SkillPackage) string {
	base := fmt.Sprintf("You are Norbot's %s stage. Work only on the operator-approved scope.\nRun: %s\nProfile: %s\nRequest: %s\nGraph: %+v\nPlanner feedback: %s\nDo not reveal credentials or execute unapproved external actions.\n", stage, run.ID, run.Profile, run.Prompt, run.Graph, run.Feedback)
	if len(skills) > 0 {
		entries := make([]string, 0, len(skills))
		for _, skill := range skills {
			entries = append(entries, skill.Name+"@"+skill.Version+" ("+skill.Digest+")")
		}
		base += "Approved declarative skills: " + strings.Join(entries, ", ") + ". Their materialized manifests are under /workspace/selected-skills.\n"
	}
	if isCLI && stage == domain.StageBuilder {
		return base + "The approved baseline is in /workspace/generated-app. Modify only that directory, then return a concise summary."
	}
	if stage == domain.StagePlanner {
		return base + "Return strict JSON without markdown: {\"graph\":{\"nodes\":[{\"id\":\"input-request\",\"label\":\"...\",\"kind\":\"input\"}],\"edges\":[]},\"notes\":\"...\"}. Graph requires input and output nodes."
	}
	if stage == domain.StageBuilder {
		return base + "Return strict JSON without markdown: {\"files\":{\"generated-app/path/to/file\":\"complete source\"}}. Include only approved files, use safe relative paths, and preserve required profile files."
	}
	return base + "Return concise verification notes."
}

func graphFromResponse(text string) (domain.Graph, bool) {
	payload, err := responseObject(text)
	if err != nil {
		return domain.Graph{}, false
	}
	raw, ok := payload["graph"]
	if !ok {
		return domain.Graph{}, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return domain.Graph{}, false
	}
	var graph domain.Graph
	if err := json.Unmarshal(encoded, &graph); err != nil || graph.Validate() != nil {
		return domain.Graph{}, false
	}
	return graph, true
}

func builderFiles(text string) (map[string]string, error) {
	payload, err := responseObject(text)
	if err != nil {
		return nil, fmt.Errorf("builder must return one JSON object: %w", err)
	}
	rawFiles, ok := payload["files"].(map[string]any)
	if !ok || len(rawFiles) == 0 || len(rawFiles) > 64 {
		return nil, fmt.Errorf("builder output requires 1-64 files")
	}
	files := make(map[string]string, len(rawFiles))
	for path, rawContent := range rawFiles {
		content, ok := rawContent.(string)
		if !ok || len(content) > 512<<10 {
			return nil, fmt.Errorf("invalid builder content for %q", path)
		}
		if !strings.HasPrefix(path, "generated-app/") || strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
			return nil, fmt.Errorf("builder path %q is outside generated-app", path)
		}
		files[path] = content
	}
	return files, nil
}

func applyBuilderResponse(workspace runtime.ArtifactWorkspace, run domain.Run, text string) ([]string, error) {
	files, err := builderFiles(text)
	if err != nil {
		return nil, err
	}
	for path, content := range files {
		if _, err := workspace.WriteArtifact(run.ID, path, []byte(content)); err != nil {
			return nil, err
		}
	}
	return mapKeys(files), nil
}

func applyRevision(workspace runtime.ArtifactWorkspace, run domain.Run, revision domain.Revision) error {
	if revision.State != "proposed" || len(revision.Files) == 0 {
		return fmt.Errorf("revision is not an applicable proposal")
	}
	if digestStringMap(revision.Files) != revision.PatchDigest {
		return fmt.Errorf("revision patch digest mismatch")
	}
	if current, err := digestGeneratedApp(workspace.RunPath(run.ID)); err != nil {
		return err
	} else if current != revision.BaselineDigest {
		return fmt.Errorf("workspace baseline changed; revision cannot be applied")
	}
	for path, content := range revision.Files {
		if _, err := workspace.WriteArtifact(run.ID, path, []byte(content)); err != nil {
			return err
		}
	}
	return nil
}

func revisionKind(attempt int) domain.ReviewKind {
	if attempt > 1 {
		return domain.ReviewFix
	}
	return domain.ReviewCode
}

func snapshotGeneratedApp(runPath string) (map[string]string, error) {
	root := filepath.Join(runPath, "generated-app")
	files := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular generated file %s", path)
		}
		relative, err := filepath.Rel(runPath, path)
		if err != nil {
			return err
		}
		value, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = string(value)
		return nil
	})
	return files, err
}

func changedFiles(baseline, current map[string]string) (map[string]string, error) {
	result := map[string]string{}
	for path, content := range current {
		if baseline[path] != content {
			result[path] = content
		}
	}
	for path := range baseline {
		if _, ok := current[path]; !ok {
			return nil, fmt.Errorf("builder cannot delete required or existing file %q", path)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("builder produced no file changes")
	}
	return result, nil
}

func restoreGeneratedApp(runPath string, files map[string]string) error {
	root := filepath.Join(runPath, "generated-app")
	if err := os.RemoveAll(root); err != nil {
		return err
	}
	for path, content := range files {
		if !strings.HasPrefix(path, "generated-app/") {
			return fmt.Errorf("invalid snapshot path")
		}
		full := filepath.Join(runPath, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(content), 0o640); err != nil {
			return err
		}
	}
	return nil
}

func digestGeneratedApp(runPath string) (string, error) {
	files, err := snapshotGeneratedApp(runPath)
	if err != nil {
		return "", err
	}
	return digestFiles(files), nil
}

func digestFiles(files map[string]string) string { return digestStringMap(files) }

func digestStringMap(values map[string]string) string {
	keys := mapKeys(values)
	sort.Strings(keys)
	hash := sha256.New()
	for _, key := range keys {
		_, _ = hash.Write([]byte(key))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(values[key]))
		_, _ = hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func mapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func materializeRunSkills(workspace runtime.WorkspaceBackend, runID string, skills []domain.SkillPackage) error {
	manifest := make([]map[string]any, 0, len(skills))
	for _, skill := range skills {
		files := []string{}
		err := filepath.Walk(skill.Path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(skill.Path, path)
			if err != nil {
				return err
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			target := filepath.ToSlash(filepath.Join("selected-skills", strings.TrimPrefix(skill.Digest, "sha256:"), relative))
			if _, err := workspace.WriteArtifact(runID, target, contents); err != nil {
				return err
			}
			files = append(files, target)
			return nil
		})
		if err != nil {
			return err
		}
		manifest = append(manifest, map[string]any{"digest": skill.Digest, "id": skill.ID, "version": skill.Version, "files": files, "manifest": skill.Manifest})
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if _, err := workspace.WriteArtifact(runID, "selected-skills/manifest.json", encoded); err != nil {
		return err
	}
	return workspace.MirrorToVolume(context.Background(), runID, "selected-skills")
}

func (s *Service) recordUsage(ctx context.Context, runID string, stage domain.Stage, revisionID *int64, providerID string, result provider.Result, prompt string) error {
	input, output, cached := result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.CachedTokens
	source, estimator := "reported", ""
	if input == nil || output == nil {
		source, estimator = "estimated", "chars_div_4_v1"
		inputValue, outputValue := estimateTokens(prompt), estimateTokens(result.Text)
		if input == nil {
			input = &inputValue
		}
		if output == nil {
			output = &outputValue
		}
	}
	if cached == nil {
		zero := 0
		cached = &zero
	}
	_, err := s.store.RecordUsage(ctx, domain.UsageRecord{RunID: runID, Stage: stage, RevisionID: revisionID, ProviderID: providerID, Model: result.Model, InputTokens: *input, OutputTokens: *output, CachedTokens: *cached, Source: source, Estimator: estimator, Metadata: result.Metadata})
	return err
}

func estimateTokens(value string) int {
	if len(value) == 0 {
		return 0
	}
	return (len(value) + 3) / 4
}

func responseObject(text string) (map[string]any, error) {
	start := strings.Index(text, "{")
	if start < 0 {
		return nil, fmt.Errorf("JSON object not found")
	}
	decoder := json.NewDecoder(strings.NewReader(text[start:]))
	decoder.UseNumber()
	payload := map[string]any{}
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func newID() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
func sleep(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
