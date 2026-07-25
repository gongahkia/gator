package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/provider"
	"github.com/gongahkia/norbot/internal/runtime"
	"github.com/gongahkia/norbot/internal/store"
)

type CreateRunInput struct {
	Prompt    string                  `json:"prompt"`
	Profile   domain.Profile          `json:"profile"`
	Providers map[domain.Stage]string `json:"providers"`
}

type ApprovalInput struct {
	Action   domain.ApprovalAction `json:"action"`
	Feedback string                `json:"feedback"`
}

type Service struct {
	store      *store.Store
	config     config.Config
	workspace  runtime.Workspace
	deployment runtime.Deployment
	invoker    provider.Invoker
	log        *slog.Logger
}

func New(st *store.Store, cfg config.Config, logger *slog.Logger) *Service {
	workspace := runtime.Workspace{DockerBin: cfg.DockerBin, ArtifactsDir: cfg.ArtifactsDir, Runner: runtime.OSRunner{}}
	return &Service{
		store: st, config: cfg, workspace: workspace,
		deployment: runtime.Deployment{DockerBin: cfg.DockerBin, Runner: runtime.OSRunner{}},
		invoker:    provider.Invoker{Workspace: workspace}, log: logger,
	}
}

func (s *Service) CreateRun(ctx context.Context, input CreateRunInput) (domain.Run, error) {
	input.Prompt = strings.TrimSpace(input.Prompt)
	if input.Prompt == "" {
		return domain.Run{}, fmt.Errorf("prompt is required")
	}
	if !input.Profile.Valid() {
		return domain.Run{}, fmt.Errorf("unsupported profile %q", input.Profile)
	}
	providers, err := s.normalizeProviders(input.Providers)
	if err != nil {
		return domain.Run{}, err
	}
	id, err := newID()
	if err != nil {
		return domain.Run{}, err
	}
	now := time.Now().UTC()
	run := domain.Run{ID: id, Prompt: input.Prompt, Profile: input.Profile, Stage: domain.StagePlanner, Status: domain.StatusQueued, Providers: providers, Graph: domain.DefaultGraph(), CreatedAt: now, UpdatedAt: now}
	if err := s.store.CreateRun(ctx, run); err != nil {
		return domain.Run{}, err
	}
	if err := s.workspace.Ensure(ctx, id); err != nil {
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
	_ = s.deployment.Stop(ctx, runID, s.workspace.RunPath(runID))
	if err := s.workspace.Cleanup(ctx, runID); err != nil {
		s.log.Warn("workspace cleanup failed after cancel", "run_id", runID, "error", err)
	}
	_ = s.store.RecordEvent(ctx, runID, "run_cancelled", "Run cancelled; Docker workspace and deployment cleanup requested", nil)
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
	_ = s.deployment.Stop(ctx, runID, s.workspace.RunPath(runID))
	if err := s.workspace.Cleanup(ctx, runID); err != nil {
		return err
	}
	return s.store.RecordEvent(ctx, runID, "run_cleanup_completed", "Docker workspace and deployment cleaned; host artifacts retained", nil)
}

func (s *Service) Capacity(ctx context.Context) runtime.Capacity {
	capacity := runtime.DetectCapacity(ctx, s.config.DockerBin, s.config.Workers, s.config.MaxWorkers, runtime.OSRunner{})
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
		factor := runtime.QuotaFactor{ProviderID: configured.ID, ConfiguredLimit: configured.Budget.MaxConcurrent}
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
	factors := map[string]any{"cpus": capacity.CPUs, "memory_bytes": capacity.MemoryBytes, "docker_available": capacity.DockerAvailable, "quota_workers": capacity.QuotaWorkers, "providers": capacity.QuotaFactors}
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

func (s *Service) execute(ctx context.Context, job domain.Job) error {
	run, err := s.store.GetRun(ctx, job.RunID)
	if err != nil {
		return err
	}
	if run.Status != domain.StatusQueued || run.Stage != job.Stage {
		return fmt.Errorf("stale job")
	}
	providerID := run.Providers[job.Stage]
	if err := s.store.MarkStageRunning(ctx, job, providerID); err != nil {
		return err
	}
	if err := s.workspace.Ensure(ctx, run.ID); err != nil {
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
	if job.Stage == domain.StageBuilder {
		generatedFiles, err = generateApp(s.workspace, run)
		if err != nil {
			return err
		}
		if err := s.workspace.MirrorGeneratedApp(ctx, run.ID); err != nil {
			return err
		}
	}
	prompt := stagePrompt(run, job.Stage, providerConfig.Kind == "cli")
	result, err := s.invoker.Invoke(ctx, providerConfig, provider.Request{RunID: run.ID, Stage: job.Stage, Prompt: prompt})
	if err != nil {
		return err
	}
	if result.RateLimit.RemainingRequests != nil || result.RateLimit.ResetAt != nil {
		if err := s.store.RecordProviderObservation(ctx, store.ProviderObservation{ProviderID: providerConfig.ID, RemainingRequests: result.RateLimit.RemainingRequests, ResetAt: result.RateLimit.ResetAt, Metadata: result.Metadata}); err != nil {
			s.log.Warn("record provider quota observation", "provider", providerConfig.ID, "error", err)
		}
	}
	artifact := map[string]any{"stage": job.Stage, "attempt": job.Attempt, "provider": result.Provider, "model": result.Model, "response": result.Text, "metadata": result.Metadata, "created_at": time.Now().UTC()}
	encoded, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.ToSlash(filepath.Join("stage-output", string(job.Stage)+fmt.Sprintf("-%d.json", job.Attempt)))
	if _, err := s.workspace.WriteArtifact(run.ID, path, encoded); err != nil {
		return err
	}
	if err := s.workspace.MirrorToVolume(ctx, run.ID, path); err != nil {
		return err
	}
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
			if err := s.workspace.SyncGeneratedApp(ctx, run.ID); err != nil {
				return err
			}
		} else {
			files, err := applyBuilderResponse(s.workspace, run, result.Text)
			if err != nil {
				return err
			}
			generatedFiles = append(generatedFiles, files...)
			if err := s.workspace.MirrorGeneratedApp(ctx, run.ID); err != nil {
				return err
			}
		}
		artifact["generated_files"] = generatedFiles
	}
	if job.Stage == domain.StageVerifier {
		report, err := verifyApp(s.workspace, run)
		if err != nil {
			return err
		}
		artifact["verification"] = report
	}
	return s.store.MarkStageAwaitingApproval(ctx, job, artifact)
}

func (s *Service) deploy(ctx context.Context, run domain.Run, job domain.Job) error {
	root := s.workspace.RunPath(run.ID)
	if err := s.store.UpsertDeployment(ctx, run.ID, runtime.ProjectName(run.ID), "", "building", ""); err != nil {
		return err
	}
	url, err := s.deployment.Deploy(ctx, run, root)
	if err != nil {
		_ = s.store.UpsertDeployment(ctx, run.ID, runtime.ProjectName(run.ID), "", "failed", err.Error())
		return err
	}
	if err := s.store.UpsertDeployment(ctx, run.ID, runtime.ProjectName(run.ID), url, "running", ""); err != nil {
		return err
	}
	return s.store.CompleteRun(ctx, job, map[string]any{"project": runtime.ProjectName(run.ID), "public_url": url})
}

func verifyApp(workspace runtime.Workspace, run domain.Run) (map[string]any, error) {
	root := filepath.Join(workspace.RunPath(run.ID), "generated-app")
	required := []string{"docker-compose.yml", "frontend/package.json", "frontend/src/main.jsx"}
	if run.Profile != domain.ProfileFrontend {
		required = append(required, "backend/main.go", "backend/go.mod")
	}
	for _, relative := range required {
		if _, err := os.Stat(filepath.Join(root, relative)); err != nil {
			return nil, fmt.Errorf("generated app missing %s", relative)
		}
	}
	return map[string]any{"status": "pass", "checks": required, "summary": "Generated profile contract is present. Operator approval is required before local deployment."}, nil
}

func stagePrompt(run domain.Run, stage domain.Stage, isCLI bool) string {
	base := fmt.Sprintf("You are Norbot's %s stage. Work only on the operator-approved scope.\nRun: %s\nProfile: %s\nRequest: %s\nGraph: %+v\nPlanner feedback: %s\nDo not reveal credentials or execute unapproved external actions.\n", stage, run.ID, run.Profile, run.Prompt, run.Graph, run.Feedback)
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

func applyBuilderResponse(workspace runtime.Workspace, run domain.Run, text string) ([]string, error) {
	payload, err := responseObject(text)
	if err != nil {
		return nil, fmt.Errorf("builder must return one JSON object: %w", err)
	}
	rawFiles, ok := payload["files"].(map[string]any)
	if !ok || len(rawFiles) == 0 || len(rawFiles) > 64 {
		return nil, fmt.Errorf("builder output requires 1-64 files")
	}
	paths := make([]string, 0, len(rawFiles))
	for path, rawContent := range rawFiles {
		content, ok := rawContent.(string)
		if !ok || len(content) > 512<<10 {
			return nil, fmt.Errorf("invalid builder content for %q", path)
		}
		if !strings.HasPrefix(path, "generated-app/") || strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
			return nil, fmt.Errorf("builder path %q is outside generated-app", path)
		}
		if _, err := workspace.WriteArtifact(run.ID, path, []byte(content)); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
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
