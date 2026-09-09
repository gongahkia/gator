package workrun

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/attachment"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/orchestrator"
	"github.com/gongahkia/gator/internal/projectcapture"
	"github.com/gongahkia/gator/internal/snapshot"
	"github.com/gongahkia/gator/internal/telemetry"
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

	if request.Limits.ModelRequests == 0 {
		request.Limits.ModelRequests = 256
	}
	if request.Limits.WallSeconds == 0 {
		request.Limits.WallSeconds = 1800
	}
	if request.Limits.ModelRequests < 1 || request.Limits.ModelRequests > 4096 || request.Limits.WallSeconds < 1 || request.Limits.WallSeconds > 86400 || request.Limits.Tokens < 0 {
		return Outcome{}, errors.New("invalid aggregate budget")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(request.Limits.WallSeconds)*time.Second)
	defer cancel()
	if request.Budget == nil {
		request.Budget = &agent.Budget{Limits: request.Limits}
	}
	e.Model = agent.WithBudget(e.Model, request.Budget)
	roles := make(map[string]agent.Model, len(e.RoleModels))
	for role, model := range e.RoleModels {
		roles[role] = agent.WithBudget(model, request.Budget)
	}
	e.RoleModels = roles
	startedAt := now()
	trace := telemetry.New(request.RunID, startedAt)
	defer trace.Finish(filepath.Join(request.StateDir, "gator", "traces", request.RunID), request.OTLPEndpoint)
	sessions, err := worksession.Open(request.StateDir)
	if err != nil {
		return Outcome{}, err
	}
	var parent worksession.Revision
	var initial []agent.Message
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

		if request.ParentRevisionID != "" {
			parent, err = sessions.LoadRevision(conversation.ID, request.ParentRevisionID)
			if err != nil {
				return Outcome{}, fmt.Errorf("load parent Work revision: %w", err)
			}
			initial, err = replayMessages(sessions, parent, request.Provider)
			if err != nil {
				return Outcome{}, err
			}
		}
		if request.RefreshSource {
			options := request.SnapshotOptions
			options.Now = now
			source := conversation.SourcePath
			if parent.SnapshotID != "" {
				previous, openErr := snapshot.Open(request.StateDir, parent.SnapshotID)
				if openErr != nil {
					return Outcome{}, openErr
				}
				source = previous.SourcePath
			}
			if request.SourcePath != "" && request.SourcePath != "." {
				source = request.SourcePath
			}
			sourceSnapshot, err = snapshot.Create(source, request.StateDir, options)
		} else {
			snapshotID := request.SnapshotID
			if snapshotID == "" {
				snapshotID = parent.SnapshotID
				if snapshotID == "" {
					snapshotID = conversation.SnapshotID
				}
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

	if request.Project == nil {
		if parent.ID != "" && !request.RefreshSource {
			request.Project = parent.Project
		} else if request.SnapshotID == "" {
			bundle, captureErr := projectcapture.Capture(sourceSnapshot.SourcePath, request.Code.Scopes, request.Code.Profile, request.Code.Capabilities)
			if captureErr != nil {
				return Outcome{}, fmt.Errorf("capture Code configuration: %w", captureErr)
			}
			request.Project = &bundle
		}
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

	var stateMu sync.Mutex
	var eventMu sync.Mutex
	renderers := make(map[string]artifact.RendererEvidence)
	surface, err := tools.WorkFilesWithRendererEvidence(work.Source, work.Output, request.Contract, request.Mode != action.Inspect, previousRoot, func(evidence artifact.RendererEvidence) {
		stateMu.Lock()
		defer stateMu.Unlock()
		renderers[evidence.Path] = evidence
	})
	if err != nil {
		return outcome, err
	}
	catalog := newCatalog(work, sourceSnapshot)
	request.catalog = catalog
	surface = append(surface, tools.ExtractDocument{Root: work.Source}, tools.InspectTable{Root: work.Source})
	surface = append(surface, catalog.tools()...)
	webTools, err := catalog.web(request.WebOrigins, e.HTTP)
	if err != nil {
		return outcome, err
	}
	surface = append(surface, webTools...)
	if request.Mode != action.Inspect {
		surface = append(surface, tools.ReconcileTables{Source: work.Source, Output: work.Output, Contract: request.Contract})
	}
	connectedSources := make([]connector.Provenance, 0, len(request.ConnectorIDs))
	if parent.ID != "" {
		bundle, err := artifact.OpenBundle(parent.BundlePath)
		if err != nil {
			return outcome, err
		}
		if err := artifact.VerifyBundle(bundle); err != nil {
			return outcome, err
		}
		for _, entry := range bundle.Manifest.Evidence {
			if entry.SnapshotPath == "" {
				continue
			}
			data, err := bundle.Root.ReadRegularFile(entry.SnapshotPath, 512*1024)
			if err != nil {
				return outcome, err
			}
			if _, err := catalog.retain(entry.Locator, data, entry.RetrievedAt); err != nil {
				return outcome, err
			}
		}
		for _, entry := range bundle.Manifest.ConnectedSources {
			data, err := bundle.Root.ReadRegularFile(entry.SnapshotPath, 256*1024)
			if err != nil {
				return outcome, err
			}
			retained, err := persistConnectedSnapshot(work.Path, connector.Result{Data: data, Provenance: entry})
			if err != nil {
				return outcome, err
			}
			connectedSources = append(connectedSources, retained)
		}
	}
	actions := make([]action.Record, 0, 4)
	connectorPolicy := tools.ConnectorPolicy{
		Mode: request.Mode, ExternalActions: request.Contract.ExternalActions, Approve: request.ApproveAction, ApproveRead: request.ApproveConnectorRead,
		Permissions: request.ConnectorPermissions,
		OnContent: func(result connector.Result) error {
			source, err := persistConnectedSnapshot(work.Path, result)
			if err != nil {
				return err
			}
			stateMu.Lock()
			defer stateMu.Unlock()
			if _, err := catalog.retain("connector:"+source.ConnectorID+"/"+source.Operation, result.Data, now()); err != nil {
				return err
			}
			connectedSources = append(connectedSources, source)
			return nil
		},
		OnAction: func(record action.Record) { stateMu.Lock(); defer stateMu.Unlock(); actions = append(actions, record) },
	}
	connectorSurface, err := tools.ConnectorTools(e.Connectors, request.ConnectorIDs, connectorPolicy)
	if err != nil {
		return outcome, err
	}
	surface = append(surface, connectorSurface...)
	request.researchTools = append(catalog.tools(), webTools...)
	readPolicy := connectorPolicy
	readPolicy.Mode = action.Inspect
	readPolicy.ExternalActions = action.Forbid
	readPolicy.Approve = nil
	connectedReads, err := tools.ConnectorTools(e.Connectors, request.ConnectorIDs, readPolicy)
	if err != nil {
		return outcome, err
	}
	request.researchTools = append(request.researchTools, connectedReads...)
	events := make([]agent.Event, 0, 32)
	emit := func(event agent.Event) {
		eventMu.Lock()
		defer eventMu.Unlock()
		trace.Record(event)
		events = append(events, event)
		if request.OnEvent != nil {
			request.OnEvent(event)
		}
	}
	subagents := make([]artifact.SubagentEvidence, 0, 8)
	integration := &codeIntegration{request: request, work: work, patches: map[string]string{}, accepted: map[string]string{}}
	for path, hash := range parent.AcceptedCode {
		integration.accepted[path] = hash
	}
	request.integration = integration
	if request.Mode != action.Inspect && e.Code != nil {
		surface = append(surface, integration)
	}
	delegationSurface, closeSpecialists, err := e.subagentTools(ctx, request, work, previousRoot, emit, func(record orchestrator.Record) {
		stateMu.Lock()
		defer stateMu.Unlock()
		subagents = append(subagents, subagentEvidence(record))
	})
	if err != nil {
		return outcome, fmt.Errorf("configure Work specialists: %w", err)
	}
	defer closeSpecialists()
	surface = append(surface, delegationSurface...)
	runner := agent.Runner{Model: e.Model, Tools: surface, Now: now}
	var check func([]agent.Message) error
	if request.Mode != action.Inspect {
		check = outcomeCompletionCheck(work.Output, request.Contract, func() bool {
			stateMu.Lock()
			defer stateMu.Unlock()
			if !request.RequireCode {
				return true
			}
			for _, record := range subagents {
				if record.Agent == "code" && record.Status == "completed" && record.ArtifactPath != "" {
					return true
				}
			}
			return false
		})
	}

	initial, summary, compacted, compactErr := gatorrun.CompactContext(ctx, e.Model, initial)
	if compactErr != nil {
		return outcome, compactErr
	}
	if compacted {
		emit(agent.Event{Kind: agent.EventContextCompacted, At: now(), Text: "Compacted retained Work context."})
	}
	initial = append(initial, agent.Message{Role: agent.RoleUser, Content: request.Objective, Images: request.Images, Attachments: request.Attachments})
	if visual, ok := e.Model.(agent.VisualInputModel); ok && !visual.SupportsVisualInput() {
		for _, message := range initial {
			if len(message.Images) > 0 {
				return outcome, errors.New("selected model does not support retained or selected images")
			}
			for _, a := range message.Attachments {
				if a.MediaType == "application/pdf" {
					return outcome, errors.New("selected model does not support PDF attachments")
				}
			}
		}
	}
	result, runErr := runner.Run(ctx, agent.RunOptions{
		InitialMessages: initial,
		Task:            request.Objective,
		Images:          request.Images,
		Attachments:     request.Attachments,
		System:          systemPrompt(request),
		MaxSteps:        request.MaxSteps,
		OnEvent:         emit,
		Steering:        request.Steering,
		CompletionCheck: check,
	})
	closeSpecialists()
	outcome.Result = result
	outcome.Events = append([]agent.Event(nil), events...)

	failure := ""
	if runErr != nil {
		failure = runErr.Error()
	}
	sortSubagentEvidence(subagents)
	trace.Record(agent.Event{Kind: "artifact_seal", At: now()})
	manifest, sealErr := artifact.Seal(work.Output, request.Contract, artifact.SealOptions{
		RunID: request.RunID, Objective: request.Objective, Source: work.Source, SourceName: sourceSnapshot.SourceName, SourceIdentity: sourceSnapshot.SourcePath, SnapshotSHA256: sourceSnapshot.SHA256,
		Failure: failure, Actions: actions, ConnectedSources: connectedSources, Renderers: rendererEvidence(renderers), Subagents: subagents, StartedAt: startedAt, FinishedAt: now(),
	})
	if sealErr != nil {
		if runErr != nil {
			return outcome, fmt.Errorf("work execution failed: %v; seal artifacts: %w", runErr, sealErr)
		}
		return outcome, fmt.Errorf("seal work artifacts: %w", sealErr)
	}
	manifest.PolicySHA256, _ = PolicyDigest(request)
	manifest.Usage = request.Budget.Usage()
	manifest.Candidates = integration.records
	manifest.Evidence = catalog.list()
	outcome.Manifest = manifest
	if err := artifact.WriteManifest(work.ManifestPath, manifest); err != nil {
		if runErr != nil {
			return outcome, fmt.Errorf("work execution failed: %v; write manifest: %w", runErr, err)
		}
		return outcome, fmt.Errorf("write work manifest: %w", err)
	}
	configuration, _ := json.Marshal(effectiveConfiguration(request))
	if _, err := sessions.AddRevision(conversation.ID, worksession.Revision{
		AcceptedCode: integration.accepted,
		Project:      request.Project,
		Replay:       &worksession.ReplayState{Version: 1, Provider: request.Provider, Messages: result.Messages, Configuration: configuration, CompactionVersion: 1, Summary: summary},
		ID:           request.RunID, ParentRevisionID: request.ParentRevisionID, SnapshotID: sourceSnapshot.ID,
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
		if request.RequireCode {
			return Request{}, errors.New("a required Code implementation is unavailable in inspect mode")
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
	if request.MaxSteps == 0 {
		request.MaxSteps = 24
	}
	if request.Code.MaxSteps == 0 || request.Code.MaxSteps > 32 {
		request.Code.MaxSteps = 32
	}
	if request.Limits.ModelRequests == 0 {
		request.Limits.ModelRequests = 256
	}
	if request.Limits.WallSeconds == 0 {
		request.Limits.WallSeconds = 1800
	}
	if request.MaxSteps < 0 {
		return Request{}, errors.New("work max steps must not be negative")
	}
	request.RoleConfiguration = make(map[string]orchestrator.RoleConfiguration)
	for _, name := range []string{"source_researcher", "artifact_reviewer", "claim_verifier", "connected_researcher", "spreadsheet_analyst", "code"} {
		maximum := maxWorkSpecialistSteps
		if name == "code" {
			maximum = request.Code.MaxSteps
		}
		configuration := e.RoleConfiguration[name]
		configuration.Version = 1
		if configuration.Provider == "" {
			configuration.Provider = request.Provider
		}
		configuration.MaxSteps = e.roleSteps(name, maximum)
		request.RoleConfiguration[name] = configuration
	}
	request.Code.MaxSteps = request.RoleConfiguration["code"].MaxSteps
	if err := attachment.ValidateLoaded(request.Images, request.Attachments); err != nil {
		return Request{}, err
	}
	request.Code.Sandbox = request.Code.Sandbox.Normalize()
	if err := request.Code.Sandbox.Validate(); err != nil {
		return Request{}, fmt.Errorf("validate Code specialist sandbox: %w", err)
	}
	if request.Code.MaxSteps < 0 {
		return Request{}, errors.New("Code specialist max steps must not be negative")
	}
	seenCapabilities := make(map[string]struct{}, len(request.Code.Capabilities))
	for _, capability := range request.Code.Capabilities {
		if _, duplicate := seenCapabilities[capability]; duplicate {
			return Request{}, fmt.Errorf("Code specialist capability %q is repeated", capability)
		}
		seenCapabilities[capability] = struct{}{}
		switch capability {
		case CodeCapabilityLSP, CodeCapabilityMCP, CodeCapabilityExtension, CodeCapabilityHTTP, CodeCapabilityBrowser, CodeCapabilityTerminal, CodeCapabilityHooks:
		default:
			return Request{}, fmt.Errorf("unknown Code specialist capability %q", capability)
		}
	}
	if request.Code.BrowserSession != "" && !request.Code.HasCapability(CodeCapabilityBrowser) {
		return Request{}, errors.New("a Code browser session requires an explicit browser capability grant")
	}
	for _, prefix := range request.Code.AllowedCommandPrefixes {
		if err := tools.ValidateCommandPrefix(prefix); err != nil {
			return Request{}, fmt.Errorf("validate Code command prefix: %w", err)
		}
	}
	if _, err := tools.ConnectorTools(e.Connectors, request.ConnectorIDs, tools.ConnectorPolicy{
		Mode: request.Mode, ExternalActions: request.Contract.ExternalActions, Approve: request.ApproveAction, ApproveRead: request.ApproveConnectorRead,
		Permissions: request.ConnectorPermissions,
	}); err != nil {
		return Request{}, fmt.Errorf("select work connectors: %w", err)
	}
	return request, nil
}

func outcomeCompletionCheck(root workspace.Root, contract artifact.Contract, codeReady func() bool) func([]agent.Message) error {
	return func(_ []agent.Message) error {
		if codeReady != nil && !codeReady() {
			return errors.New("delegate the coding implementation to the Code specialist and retain its patch evidence")
		}
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

func rendererEvidence(values map[string]artifact.RendererEvidence) []artifact.RendererEvidence {
	paths := make([]string, 0, len(values))
	for path := range values {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make([]artifact.RendererEvidence, 0, len(paths))
	for _, path := range paths {
		result = append(result, values[path])
	}
	return result
}

func newID(now time.Time) (string, error) {
	entropy := make([]byte, 6)
	if _, err := rand.Read(entropy); err != nil {
		return "", fmt.Errorf("generate work id: %w", err)
	}
	return "work-" + now.UTC().Format("20060102T150405") + "-" + hex.EncodeToString(entropy), nil
}

func replayMessages(store worksession.Store, revision worksession.Revision, provider string) ([]agent.Message, error) {
	if revision.Replay != nil {
		if revision.Replay.Version != 1 {
			return nil, errors.New("unsupported Work replay version")
		}
		payload, err := json.Marshal(revision.Replay.Messages)
		if err != nil {
			return nil, err
		}
		var messages []agent.Message
		if err := json.Unmarshal(payload, &messages); err != nil {
			return nil, err
		}
		if provider != revision.Replay.Provider {
			for i := range messages {
				messages[i].ProviderData = nil
				for j := range messages[i].ToolCalls {
					messages[i].ToolCalls[j].ProviderID = ""
				}
			}
		}
		return messages, nil
	}
	// legacy revisions retain only objective and final text; never invent tool history.
	var messages []agent.Message
	if revision.ParentRevisionID != "" {
		parent, err := store.LoadRevision(revision.ConversationID, revision.ParentRevisionID)
		if err != nil {
			return nil, err
		}
		messages, err = replayMessages(store, parent, provider)
		if err != nil {
			return nil, err
		}
	}
	messages = append(messages, agent.Message{Role: agent.RoleUser, Content: revision.Objective})
	if revision.FinalText != "" {
		messages = append(messages, agent.Message{Role: agent.RoleAgent, Content: revision.FinalText})
	}
	return messages, nil
}
