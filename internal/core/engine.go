package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

type Engine struct {
	workspace Workspace
	store     *Store
	providers *Registry
	sequence  atomic.Uint64
}

func Open(ctx context.Context, cwd string) (*Engine, error) {
	workspace, err := DiscoverWorkspace(ctx, cwd)
	if err != nil {
		return nil, err
	}
	store := NewStore(workspace.Root)
	if err := store.Ensure(); err != nil {
		return nil, err
	}
	return &Engine{workspace: workspace, store: store, providers: DefaultRegistry()}, nil
}

func NewForTest(workspace Workspace, store *Store, providers *Registry) *Engine {
	return &Engine{workspace: workspace, store: store, providers: providers}
}

func (e *Engine) Workspace() Workspace      { return e.workspace }
func (e *Engine) Store() *Store              { return e.store }
func (e *Engine) Providers(ctx context.Context) []ProviderStatus { return e.providers.Statuses(ctx) }
func (e *Engine) Policy() (Policy, PolicyRef, error) { return LoadPolicy(e.workspace.Root) }

type RunRequest struct {
	Objective    string
	Provider     string
	Role         string
	Files        []FileReference
	IncludeDiff  *bool
	Worktree     bool
	Execute      bool
	ParentRunID  string
	Dependencies []string
}

func (e *Engine) Run(ctx context.Context, request RunRequest, input io.Reader, output, errors io.Writer) (Run, error) {
	policy, reference, err := e.Policy()
	if err != nil {
		return Run{}, err
	}
	return e.runWithPolicy(ctx, request, policy, reference, input, output, errors)
}

func (e *Engine) runWithPolicy(ctx context.Context, request RunRequest, policy Policy, reference PolicyRef, input io.Reader, output, errors io.Writer) (Run, error) {
	if strings.TrimSpace(request.Objective) == "" {
		return Run{}, fmt.Errorf("objective must be non-empty")
	}
	if err := policy.Validate(); err != nil {
		return Run{}, err
	}
	providerID := request.Provider
	if providerID == "" {
		providerID = policy.Routing.DefaultProvider
	}
	provider, exists := e.providers.Provider(providerID)
	if !exists {
		return Run{}, fmt.Errorf("provider %s is unknown", providerID)
	}
	status := provider.Status(ctx)
	if !status.Available && request.Execute {
		return Run{}, fmt.Errorf("provider %s is unavailable: %s", providerID, status.Reason)
	}
	role := request.Role
	if role == "" {
		role = "writer"
	}
	if role != "writer" && role != "researcher" && role != "reviewer" && role != "integrator" {
		return Run{}, fmt.Errorf("run role is unsupported")
	}
	workspace := e.workspace
	id := e.id("run")
	if request.Worktree {
		workspace, err = CreateWorktree(ctx, e.workspace, id)
		if err != nil {
			return Run{}, err
		}
	}
	includeDiff := policy.Context.IncludeDiff
	if request.IncludeDiff != nil {
		includeDiff = *request.IncludeDiff
	}
	prepared, err := BuildContext(ctx, workspace, request.Objective, request.Files, includeDiff, policy)
	if err != nil {
		return Run{}, err
	}
	now := time.Now().UTC()
	run := Run{
		SchemaVersion: SchemaVersion, ID: id, TaskID: e.id("task"), Objective: redactObjective(request.Objective),
		Provider: providerID, Role: role, Transport: transport(status.Capabilities), State: "planned", Workspace: workspace,
		ParentRunID: request.ParentRunID, Dependencies: append([]string(nil), request.Dependencies...), Context: prepared.Manifest,
		Policy: reference, Usage: Usage{State: "unknown"}, Activity: Activity{State: "inactive"},
		Verification: Verification{State: "unknown", BaseSHA: workspace.Head}, CreatedAt: now, UpdatedAt: now,
	}
	if err := e.store.PutRun(run); err != nil {
		return Run{}, err
	}
	if err := e.event(run, "run.created", "planned", "", nil, ""); err != nil {
		return Run{}, err
	}
	if err := e.event(run, "context.prepared", "planned", prepared.Manifest.Description, nil, ""); err != nil {
		return Run{}, err
	}
	if !request.Execute {
		run.State, run.FinishedAt, run.UpdatedAt = "completed", time.Now().UTC(), time.Now().UTC()
		if err := e.store.PutRun(run); err != nil {
			return Run{}, err
		}
		if err := e.event(run, "run.dry_run", run.State, "provider was not executed", nil, ""); err != nil {
			return Run{}, err
		}
		return run, e.recordEpisode(ctx, run)
	}

	run.State, run.StartedAt, run.UpdatedAt = "running", time.Now().UTC(), time.Now().UTC()
	run.Activity = Activity{State: "active", LastEventAt: run.StartedAt}
	if err := e.store.PutRun(run); err != nil {
		return Run{}, err
	}
	if err := e.event(run, "run.started", run.State, "provider process started", nil, ""); err != nil {
		return Run{}, err
	}
	command, err := provider.Start(ctx, workspace, prepared.Prompt, input, output, errors)
	if err == nil {
		err = command.Run()
	}
	finished := time.Now().UTC()
	run.FinishedAt, run.UpdatedAt, run.Activity = finished, finished, Activity{State: "inactive", LastEventAt: finished}
	if err != nil {
		run.State, run.ProviderError = "failed", redactObjective(err.Error())
		if putErr := e.store.PutRun(run); putErr != nil {
			return Run{}, putErr
		}
		code := exitCode(err)
		if eventErr := e.event(run, "provider.exited", run.State, run.ProviderError, &code, ""); eventErr != nil {
			return Run{}, eventErr
		}
		_ = e.recordEpisode(ctx, run)
		return run, fmt.Errorf("provider run failed: %w", err)
	}
	run.State = "completed"
	if err := e.store.PutRun(run); err != nil {
		return Run{}, err
	}
	zero := 0
	if err := e.event(run, "provider.exited", run.State, "provider process completed", &zero, ""); err != nil {
		return Run{}, err
	}
	if len(policy.Verification.Commands) > 0 {
		if _, err := e.VerifyAll(ctx, run.ID, output, errors); err != nil {
			return e.store.GetRun(run.ID)
		}
		run, _ = e.store.GetRun(run.ID)
	}
	return run, e.recordEpisode(ctx, run)
}

func (e *Engine) Handoff(ctx context.Context, sourceID, provider string, execute bool, input io.Reader, output, errors io.Writer) (Run, error) {
	source, err := e.store.GetRun(sourceID)
	if err != nil {
		return Run{}, err
	}
	objective := "Continue the reviewed handoff from " + source.Provider + ": " + source.Objective
	return e.Run(ctx, RunRequest{
		Objective: objective, Provider: provider, Role: "writer", ParentRunID: source.ID,
		Dependencies: []string{source.ID}, Execute: execute,
	}, input, output, errors)
}

func (e *Engine) event(run Run, kind, state, message string, exit *int, evidenceID string) error {
	return e.store.AppendEvent(Event{
		SchemaVersion: SchemaVersion, ID: e.id("event"), RunID: run.ID, At: time.Now().UTC(), Type: kind,
		Provider: run.Provider, State: state, Message: redactObjective(message), ExitCode: exit, EvidenceID: evidenceID,
	})
}

func (e *Engine) recordEpisode(ctx context.Context, run Run) error {
	_, diff, err := WorkspaceDiff(ctx, run.Workspace)
	if err != nil {
		return err
	}
	ending, err := DiscoverWorkspace(ctx, run.Workspace.Root)
	if err != nil {
		return err
	}
	duration := int64(0)
	if !run.StartedAt.IsZero() && !run.FinishedAt.IsZero() {
		duration = run.FinishedAt.Sub(run.StartedAt).Milliseconds()
	}
	fingerprint := sha256.Sum256([]byte(run.Objective))
	episode := Episode{
		SchemaVersion: SchemaVersion, ID: e.id("episode"), RunID: run.ID,
		TaskFingerprint: hex.EncodeToString(fingerprint[:12]), Provider: run.Provider, PolicyVersion: run.Policy.Version,
		WorkflowTopology: "direct_writer", StartingSHA: run.Workspace.Head, EndingSHA: ending.Head,
		EndingDiffSHA256: diff, DurationMillis: duration, RunState: run.State,
		VerificationState: run.Verification.State, Usage: run.Usage, ObservedAt: time.Now().UTC(),
	}
	return e.store.PutEpisode(episode)
}

func (e *Engine) id(prefix string) string {
	sequence := e.sequence.Add(1)
	source := fmt.Sprintf("%s:%d:%d:%d", prefix, os.Getpid(), time.Now().UnixNano(), sequence)
	digest := sha256.Sum256([]byte(source))
	return prefix + "-" + hex.EncodeToString(digest[:12])
}

func transport(capabilities ProviderCapabilities) string {
	if capabilities.Structured {
		return "structured"
	}
	return "terminal"
}

func exitCode(err error) int {
	type exitCoder interface{ ExitCode() int }
	if value, ok := err.(exitCoder); ok {
		return value.ExitCode()
	}
	return 1
}

func redactObjective(value string) string {
	redacted, _ := redactText(value)
	return redacted
}

func (e *Engine) StatePath(parts ...string) string {
	return filepath.Join(append([]string{e.store.Dir()}, parts...)...)
}
