package workrun

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/snapshot"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/worksession"
	"github.com/gongahkia/gator/internal/workspace"
)

// Execute creates a private non-Git workspace, runs the bounded agent loop,
// and seals a filesystem-backed manifest on both success and ordinary agent
// failure. The retained output can therefore be reviewed after an interrupted
// or unsuccessful attempt.
func (e Executor) Execute(ctx context.Context, request Request) (Outcome, error) {
	now := e.Now
	if now == nil {
		now = time.Now
	}
	request, err := e.normalizeAndValidate(request)
	if err != nil {
		return Outcome{}, err
	}
	if request.RunID == "" {
		request.RunID, err = newID(now())
		if err != nil {
			return Outcome{}, err
		}
	}
	startedAt := now()
	sessions, err := worksession.Open(request.StateDir)
	if err != nil {
		return Outcome{}, err
	}
	var conversation worksession.Conversation
	var sourceSnapshot snapshot.Manifest
	if request.ConversationID != "" {
		conversation, err = sessions.Load(request.ConversationID)
		if err != nil {
			return Outcome{}, fmt.Errorf("load Work conversation: %w", err)
		}
		if request.ParentRevisionID == "" {
			request.ParentRevisionID = conversation.HeadRevision
		}
		if request.RefreshSource {
			options := request.SnapshotOptions
			options.Now = now
			sourceSnapshot, err = snapshot.Create(conversation.SourcePath, request.StateDir, options)
		} else {
			snapshotID := request.SnapshotID
			if snapshotID == "" {
				snapshotID = conversation.SnapshotID
			}
			sourceSnapshot, err = snapshot.Open(request.StateDir, snapshotID)
		}
	} else if request.SnapshotID != "" {
		sourceSnapshot, err = snapshot.Open(request.StateDir, request.SnapshotID)
	} else {
		options := request.SnapshotOptions
		options.Now = now
		sourceSnapshot, err = snapshot.Create(request.SourcePath, request.StateDir, options)
	}
	if err != nil {
		return Outcome{}, err
	}
	if request.OnSnapshot != nil {
		request.OnSnapshot(sourceSnapshot)
	}
	if conversation.ID == "" {
		conversation, err = sessions.Create(conversationTitle(request.Objective), sourceSnapshot.SourcePath, sourceSnapshot.ID, startedAt)
		if err != nil {
			return Outcome{}, err
		}
	}
	work, err := workspace.CreateWork(sourceSnapshot.Materialized, request.StateDir, request.RunID, startedAt)
	if err != nil {
		return Outcome{}, err
	}
	outcome := Outcome{Work: work, ConversationID: conversation.ID, RevisionID: request.RunID, SnapshotID: sourceSnapshot.ID, SourceSnapshot: sourceSnapshot}
	var previousRoot workspace.Root
	if request.ParentRevisionID != "" {
		parent, loadErr := sessions.LoadRevision(conversation.ID, request.ParentRevisionID)
		if loadErr != nil {
			return outcome, fmt.Errorf("load parent Work revision: %w", loadErr)
		}
		if copyErr := seedPreviousArtifacts(filepath.Join(parent.BundlePath, "output"), work.Output, request.Contract.MaxArtifactBytes); copyErr != nil {
			return outcome, fmt.Errorf("seed parent Work artifacts: %w", copyErr)
		}
		previousRoot, err = workspace.Open(filepath.Join(parent.BundlePath, "output"))
		if err != nil {
			return outcome, fmt.Errorf("open parent Work artifacts: %w", err)
		}
	}

	surface, err := tools.WorkFiles(work.Source, work.Output, request.Contract, request.Mode != action.Inspect, previousRoot)
	if err != nil {
		return outcome, err
	}
	connectedSources := make([]connector.Provenance, 0, len(request.ConnectorIDs))
	actions := make([]action.Record, 0, 4)
	connectorSurface, err := tools.ConnectorTools(e.Connectors, request.ConnectorIDs, tools.ConnectorPolicy{
		Mode: request.Mode, ExternalActions: request.Contract.ExternalActions, Approve: request.ApproveAction,
		Permissions: request.ConnectorPermissions,
		OnContent: func(result connector.Result) error {
			source, err := persistConnectedSnapshot(work.Path, result)
			if err != nil {
				return err
			}
			connectedSources = append(connectedSources, source)
			return nil
		},
		OnAction: func(record action.Record) { actions = append(actions, record) },
	})
	if err != nil {
		return outcome, err
	}
	surface = append(surface, connectorSurface...)
	events := make([]agent.Event, 0, 32)
	emit := func(event agent.Event) {
		events = append(events, event)
		if request.OnEvent != nil {
			request.OnEvent(event)
		}
	}
	runner := agent.Runner{Model: e.Model, Tools: surface, Now: now}
	var check func([]agent.Message) error
	if request.Mode != action.Inspect {
		check = outcomeCompletionCheck(work.Output, request.Contract)
	}
	result, runErr := runner.Run(ctx, agent.RunOptions{
		Task:            request.Objective,
		System:          systemPrompt(request),
		MaxSteps:        request.MaxSteps,
		OnEvent:         emit,
		Steering:        request.Steering,
		CompletionCheck: check,
	})
	outcome.Result = result
	outcome.Events = append([]agent.Event(nil), events...)

	failure := ""
	if runErr != nil {
		failure = runErr.Error()
	}
	manifest, sealErr := artifact.Seal(work.Output, request.Contract, artifact.SealOptions{
		RunID: request.RunID, Objective: request.Objective, Source: work.Source, SourceName: sourceSnapshot.SourceName, SourceIdentity: sourceSnapshot.SourcePath, SnapshotSHA256: sourceSnapshot.SHA256,
		Failure: failure, Actions: actions, ConnectedSources: connectedSources, StartedAt: startedAt, FinishedAt: now(),
	})
	if sealErr != nil {
		if runErr != nil {
			return outcome, fmt.Errorf("work execution failed: %v; seal artifacts: %w", runErr, sealErr)
		}
		return outcome, fmt.Errorf("seal work artifacts: %w", sealErr)
	}
	outcome.Manifest = manifest
	if err := artifact.WriteManifest(work.ManifestPath, manifest); err != nil {
		if runErr != nil {
			return outcome, fmt.Errorf("work execution failed: %v; write manifest: %w", runErr, err)
		}
		return outcome, fmt.Errorf("write work manifest: %w", err)
	}
	if _, err := sessions.AddRevision(conversation.ID, worksession.Revision{
		ID: request.RunID, ParentRevisionID: request.ParentRevisionID, SnapshotID: sourceSnapshot.ID,
		Objective: request.Objective, BundlePath: work.Path, Status: string(manifest.Status), FinalText: result.FinalText, CreatedAt: now(),
	}); err != nil {
		if runErr != nil {
			return outcome, fmt.Errorf("work execution failed: %v; retain revision: %w", runErr, err)
		}
		return outcome, fmt.Errorf("retain Work revision: %w", err)
	}
	if runErr != nil {
		return outcome, runErr
	}
	if manifest.Status != artifact.Completed {
		return outcome, errors.New("work outcome contract did not pass")
	}
	return outcome, nil
}

func persistConnectedSnapshot(workPath string, result connector.Result) (connector.Provenance, error) {
	directory := filepath.Join(workPath, "connected")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return connector.Provenance{}, err
	}
	name := result.Provenance.ConnectorID + "-" + strings.ReplaceAll(result.Provenance.Operation, "_", "-") + "-" + result.Provenance.SHA256[:16] + ".json"
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, result.Data, 0o600); err != nil {
		return connector.Provenance{}, err
	}
	result.Provenance.SnapshotPath = filepath.ToSlash(filepath.Join("connected", name))
	return result.Provenance, nil
}

func conversationTitle(objective string) string {
	title := strings.TrimSpace(objective)
	if line, _, found := strings.Cut(title, "\n"); found {
		title = line
	}
	if len(title) > 80 {
		title = strings.TrimSpace(title[:80]) + "…"
	}
	return title
}

func seedPreviousArtifacts(source string, destination workspace.Root, maxFileBytes int64) error {
	info, err := os.Stat(source)
	if err != nil || !info.IsDir() {
		return errors.New("parent Work output is missing")
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("parent output contains symlink %q", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("parent output contains non-regular file %q", path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxFileBytes {
			return fmt.Errorf("parent artifact %q exceeds the current contract limit", path)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		return destination.WriteRegularFileAtomic(relative, contents, maxFileBytes)
	})
}

func (e Executor) normalizeAndValidate(request Request) (Request, error) {
	if e.Model == nil {
		return Request{}, errors.New("work agent model is required")
	}
	request.Objective = strings.TrimSpace(request.Objective)
	if request.Objective == "" || len(request.Objective) > 64*1024 || strings.ContainsRune(request.Objective, 0) {
		return Request{}, errors.New("work objective is missing or invalid")
	}
	if strings.TrimSpace(request.SourcePath) == "" {
		return Request{}, errors.New("work source directory is required")
	}
	if request.Mode == "" {
		request.Mode = action.Draft
	}
	if err := request.Mode.Validate(); err != nil {
		return Request{}, err
	}
	request.Contract = request.Contract.Normalize()
	if err := request.Contract.Validate(); err != nil {
		return Request{}, fmt.Errorf("validate outcome contract: %w", err)
	}
	if request.Mode == action.Inspect {
		if len(request.Contract.Artifacts) != 0 || request.Contract.ExternalActions != action.Forbid {
			return Request{}, errors.New("inspect mode requires an inspection-only contract")
		}
	} else if len(request.Contract.Artifacts) == 0 {
		return Request{}, errors.New("draft and act modes require at least one artifact")
	}
	if request.Mode == action.Draft && request.Contract.ExternalActions == action.Approve {
		return Request{}, errors.New("draft mode cannot approve external actions")
	}
	stateDir := strings.TrimSpace(request.StateDir)
	if stateDir == "" {
		stateDir = strings.TrimSpace(e.StateDir)
	}
	if stateDir == "" {
		return Request{}, errors.New("work state directory is required")
	}
	request.StateDir = stateDir
	if request.MaxSteps < 0 {
		return Request{}, errors.New("work max steps must not be negative")
	}
	if _, err := tools.ConnectorTools(e.Connectors, request.ConnectorIDs, tools.ConnectorPolicy{
		Mode: request.Mode, ExternalActions: request.Contract.ExternalActions, Approve: request.ApproveAction,
		Permissions: request.ConnectorPermissions,
	}); err != nil {
		return Request{}, fmt.Errorf("select work connectors: %w", err)
	}
	return request, nil
}

func outcomeCompletionCheck(root workspace.Root, contract artifact.Contract) func([]agent.Message) error {
	return func(_ []agent.Message) error {
		inspection, err := artifact.Inspect(root, contract)
		if err != nil {
			return fmt.Errorf("inspect staged artifacts: %w", err)
		}
		if inspection.Passed {
			return nil
		}
		failures := make([]string, 0, 3)
		for _, result := range inspection.Validations {
			if result.Passed {
				continue
			}
			message := result.Path + " failed " + string(result.Kind)
			if result.Diagnostic != "" {
				message += ": " + result.Diagnostic
			}
			failures = append(failures, message)
			if len(failures) == 3 {
				break
			}
		}
		if len(failures) == 0 {
			return errors.New("the outcome contract has not passed")
		}
		return errors.New(strings.Join(failures, "; "))
	}
}

func newID(now time.Time) (string, error) {
	entropy := make([]byte, 6)
	if _, err := rand.Read(entropy); err != nil {
		return "", fmt.Errorf("generate work id: %w", err)
	}
	return "work-" + now.UTC().Format("20060102T150405") + "-" + hex.EncodeToString(entropy), nil
}
