package workrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/orchestrator"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workspace"
)

// Service is the typed application boundary shared by foreground and unattended Work.
type Service struct {
	Executor Executor
	Defaults *Request
}

var workRoleNames = []string{"source_researcher", "artifact_reviewer", "claim_verifier", "connected_researcher", "spreadsheet_analyst", "goal_reviewer", "code"}

func ValidateDisabledRoles(roles []string) error {
	seen := map[string]bool{}
	for _, role := range roles {
		if !slices.Contains(workRoleNames, role) || seen[role] {
			return fmt.Errorf("invalid or repeated disabled role %q", role)
		}
		seen[role] = true
	}
	return nil
}

func (s Service) Execute(ctx context.Context, request Request) (Outcome, error) {
	if s.Defaults != nil {
		request.Provider = s.Defaults.Provider
		request.ConnectorPermissions = s.Defaults.ConnectorPermissions
		request.SnapshotOptions = s.Defaults.SnapshotOptions
		request.OTLPEndpoint = s.Defaults.OTLPEndpoint
		request.LearningContext = s.Defaults.LearningContext
		request.MemoryContext = s.Defaults.MemoryContext
		request.NoteContext = s.Defaults.NoteContext
		request.ContextRefs = append([]string(nil), s.Defaults.ContextRefs...)
		request.RecallStatus = s.Defaults.RecallStatus
		request.GoalID = s.Defaults.GoalID
		request.GoalPlanContext = s.Defaults.GoalPlanContext
	}
	resolved, err := s.Executor.normalizeAndValidate(request)
	if err != nil {
		return Outcome{}, err
	}
	return s.Executor.Execute(ctx, resolved)
}

// PolicyDigest includes the complete effective contract and domain-specific grants.
func PolicyDigest(request Request) (string, error) {
	data, err := json.Marshal(effectiveConfiguration(request))
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func effectiveConfiguration(request Request) any {
	return struct {
		Contract            artifact.Contract
		Mode                action.Mode
		Code                CodePolicy
		Connectors          []string
		OAuthRefreshPins    map[string]string
		OAuthAccountPins    map[string]string
		Permissions         any
		WebOrigins          []string
		IgnoredInstructions []string
		Limits              agent.Limits
		MaxSteps            int
		Provider            string
		GoalContext         string
		GoalPlanContext     string
		GoalID              string
		MemoryContext       string
		NoteContext         string
		Roles               map[string]orchestrator.RoleConfiguration
		ProjectSHA256       string
		DisableDelegation   bool
		DisabledRoles       []string
		EnableCode          bool
		RequireCode         bool
		BrowserSession      string
		DisableBrowser      bool
		DesktopSession      string
	}{request.Contract, request.Mode, request.Code, request.ConnectorIDs, request.OAuthRefreshPins, request.OAuthAccountPins, request.ConnectorPermissions, request.WebOrigins, request.IgnoredInstructionPaths, request.Limits, request.MaxSteps, request.Provider, request.GoalContext, request.GoalPlanContext, request.GoalID, request.MemoryContext, request.NoteContext, request.RoleConfiguration, func() string {
		if request.Project != nil {
			return request.Project.SHA256
		}
		return ""
	}(), request.DisableDelegation, request.DisabledRoles, request.EnableCode, request.RequireCode, request.BrowserSession, request.DisableBrowser, request.DesktopSession}
}

type Interaction struct {
	ID      int    `json:"id"`
	Kind    string `json:"kind"`
	Preview any    `json:"preview"`
}

// InteractionRecord is the retained state of a foreground decision. A pending
// record alone grants no approval authority and may be stale if the original
// process exited unexpectedly. Remote answers additionally need a live run,
// a private control token, and the original preview digest.
type InteractionRecord struct {
	Version     int         `json:"version"`
	Status      string      `json:"status"`
	Interaction Interaction `json:"interaction"`
	Approved    *bool       `json:"approved,omitempty"`
	RunID       string      `json:"run_id,omitempty"`
	LiveStatus  string      `json:"live_status,omitempty"`
}
type Completion struct {
	Outcome Outcome
	Err     error
}
type Operation struct {
	RunID        string
	tasks        *orchestrator.Supervisor
	Events       <-chan agent.Event
	Interactions <-chan Interaction
	Done         <-chan Completion
	cancel       context.CancelFunc
	steering     chan string
	mu           sync.Mutex
	next         int
	pending      map[int]pendingInteraction
	stopped      <-chan struct{}
}

type pendingInteraction struct {
	response chan bool
	digest   string
}

func (o *Operation) Cancel() { o.cancel() }
func (o *Operation) Steer(text string) error {
	if text == "" || len(text) > 64*1024 {
		return errors.New("invalid steering text")
	}
	select {
	case <-o.stopped:
		return errors.New("operation has stopped")
	default:
	}
	select {
	case o.steering <- text:
		return nil
	default:
		return errors.New("steering queue is full")
	}
}
func (o *Operation) Respond(id int, approved bool) error {
	return o.respond(id, "", approved)
}

func (o *Operation) respondBound(id int, digest string, approved bool) error {
	if digest == "" {
		return errors.New("interaction digest is required")
	}
	return o.respond(id, digest, approved)
}

func (o *Operation) respond(id int, digest string, approved bool) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	pending, ok := o.pending[id]
	if !ok {
		return errors.New("interaction is no longer pending")
	}
	if digest != "" && pending.digest != digest {
		return errors.New("pending interaction preview changed")
	}
	delete(o.pending, id)
	pending.response <- approved
	return nil
}
func (s Service) Start(ctx context.Context, request Request) *Operation {
	ctx, cancel := context.WithCancel(ctx)
	request.EnableControl = true
	if request.StateDir == "" {
		request.StateDir = s.Executor.StateDir
	}
	var initializationErr error
	if request.RunID == "" {
		request.RunID, initializationErr = newID(time.Now())
	}
	if initializationErr == nil && !validInteractionRunID(request.RunID) {
		initializationErr = errors.New("invalid Work run ID")
	}
	interactionDir := filepath.Join(request.StateDir, "gator", "interactions", request.RunID)
	persistInteraction := func(value any, id int) error {
		if err := os.MkdirAll(interactionDir, 0700); err != nil {
			return err
		}
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		root, err := workspace.Open(interactionDir)
		if err != nil {
			return err
		}
		return root.WriteRegularFileAtomic(fmt.Sprintf("%d.json", id), data, 1024*1024)
	}
	events := make(chan agent.Event, 128)
	interactions := make(chan Interaction, 8)
	done := make(chan Completion, 1)
	o := &Operation{RunID: request.RunID, Events: events, Interactions: interactions, Done: done, cancel: cancel, steering: make(chan string, 16), pending: map[int]pendingInteraction{}, stopped: ctx.Done()}
	var lockOnce sync.Once
	var liveLock *os.File
	var lockErr error
	ensureLiveLock := func() error {
		lockOnce.Do(func() { liveLock, lockErr = acquireInteractionRunLock(request.StateDir, request.RunID) })
		return lockErr
	}
	ask := func(ctx context.Context, kind string, preview any) (bool, error) {
		if err := ensureLiveLock(); err != nil {
			return false, err
		}
		o.mu.Lock()
		o.next++
		id := o.next
		response := make(chan bool, 1)
		interaction := Interaction{id, kind, preview}
		digest, err := InteractionDigest(interaction)
		if err != nil {
			o.mu.Unlock()
			return false, err
		}
		o.pending[id] = pendingInteraction{response: response, digest: digest}
		o.mu.Unlock()
		defer func() { o.mu.Lock(); delete(o.pending, id); o.mu.Unlock() }()
		if err := persistInteraction(InteractionRecord{Version: 1, Status: "pending", Interaction: interaction}, id); err != nil {
			return false, err
		}
		select {
		case interactions <- interaction:
		case <-ctx.Done():
			return false, errors.Join(ctx.Err(), persistInteraction(InteractionRecord{Version: 1, Status: "cancelled", Interaction: interaction}, id))
		}
		select {
		case approved := <-response:
			if err := persistInteraction(InteractionRecord{Version: 1, Status: "resolved", Interaction: interaction, Approved: &approved}, id); err != nil {
				return false, err
			}
			return approved, nil
		case <-ctx.Done():
			return false, errors.Join(ctx.Err(), persistInteraction(InteractionRecord{Version: 1, Status: "cancelled", Interaction: interaction}, id))
		}
	}
	request.OnSupervisor = func(supervisor *orchestrator.Supervisor) { o.mu.Lock(); o.tasks = supervisor; o.mu.Unlock() }
	request.Steering = o.steering
	request.controlSteering = o.steering
	request.controlRespond = o.respondBound
	request.controlCancelTask = o.CancelTask
	request.controlSteerTask = o.SteerTask
	sink := request.OnEvent
	request.OnEvent = func(event agent.Event) {
		if sink != nil {
			sink(event)
		}
		select {
		case events <- event:
		case <-ctx.Done():
		}
	}
	request.ApproveCodeCommand = func(ctx context.Context, argv []string) (tools.CommandDecision, error) {
		approved, err := ask(ctx, "command", argv)
		if approved {
			return tools.CommandAllowOnce, err
		}
		return tools.CommandDeny, err
	}
	request.ApproveBrowser = func(ctx context.Context, argv []string) (tools.CommandDecision, error) {
		approved, err := ask(ctx, "browser", argv)
		if approved {
			return tools.CommandAllowOnce, err
		}
		return tools.CommandDeny, err
	}
	request.ApproveDesktop = func(ctx context.Context, argv []string) (tools.CommandDecision, error) {
		approved, err := ask(ctx, "desktop", argv)
		if approved {
			return tools.CommandAllowOnce, err
		}
		return tools.CommandDeny, err
	}
	request.ApproveConnectorRead = func(ctx context.Context, id, operation string, args json.RawMessage) (bool, error) {
		return ask(ctx, "connected_read", struct {
			Connector, Operation string
			Arguments            json.RawMessage
		}{id, operation, args})
	}
	// external actions keep their exact request and digest at the domain boundary.
	request.ApproveAction = func(ctx context.Context, proposal action.Proposal) (action.Decision, error) {
		approved, err := ask(ctx, "action", proposal)
		if err != nil {
			return action.Deny, err
		}
		if !approved {
			return action.Deny, errors.New("action denied")
		}
		return action.Allow, nil
	}
	go func() {
		defer cancel()
		var outcome Outcome
		err := initializationErr
		if err == nil {
			outcome, err = s.Execute(ctx, request)
		}
		if liveLock != nil {
			err = errors.Join(err, liveLock.Close())
		}
		close(events)
		close(interactions)
		done <- Completion{outcome, err}
		close(done)
	}()
	return o
}

func (o *Operation) InspectTask(id string) (orchestrator.Task, error) {
	o.mu.Lock()
	tasks := o.tasks
	o.mu.Unlock()
	if tasks == nil {
		return orchestrator.Task{}, errors.New("specialists are not started")
	}
	return tasks.Inspect(id)
}
func (o *Operation) CancelTask(id string) error {
	o.mu.Lock()
	tasks := o.tasks
	o.mu.Unlock()
	if tasks == nil {
		return errors.New("specialists are not started")
	}
	return tasks.Cancel(id)
}

func (o *Operation) SteerTask(id, instruction string) error {
	o.mu.Lock()
	tasks := o.tasks
	o.mu.Unlock()
	if tasks == nil {
		return errors.New("specialists are not started")
	}
	return tasks.Steer(id, instruction)
}
