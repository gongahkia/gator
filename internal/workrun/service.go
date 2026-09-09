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
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/orchestrator"
	"github.com/gongahkia/gator/internal/tools"
)

// Service is the typed application boundary shared by foreground and unattended Work.
type Service struct {
	Executor Executor
	Defaults *Request
}

func (s Service) Execute(ctx context.Context, request Request) (Outcome, error) {
	if s.Defaults != nil {
		request.Provider = s.Defaults.Provider
		request.ConnectorPermissions = s.Defaults.ConnectorPermissions
		request.SnapshotOptions = s.Defaults.SnapshotOptions
		request.OTLPEndpoint = s.Defaults.OTLPEndpoint
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
  Contract artifact.Contract
  Mode action.Mode
  Code CodePolicy
  Connectors []string
  Permissions any
  WebOrigins []string
  Limits agent.Limits
  MaxSteps int
  Provider string
  Roles map[string]orchestrator.RoleConfiguration
 }{request.Contract, request.Mode, request.Code, request.ConnectorIDs, request.ConnectorPermissions, request.WebOrigins, request.Limits, request.MaxSteps, request.Provider, request.RoleConfiguration}
}

type Interaction struct {
	ID      int    `json:"id"`
	Kind    string `json:"kind"`
	Preview any    `json:"preview"`
}
type Completion struct {
	Outcome Outcome
	Err     error
}
type Operation struct {
	tasks        *orchestrator.Supervisor
	Events       <-chan agent.Event
	Interactions <-chan Interaction
	Done         <-chan Completion
	cancel       context.CancelFunc
	steering     chan string
	mu           sync.Mutex
	next         int
	pending      map[int]chan bool
}

func (o *Operation) Cancel() { o.cancel() }
func (o *Operation) Steer(text string) error {
	if text == "" || len(text) > 64*1024 {
		return errors.New("invalid steering text")
	}
	select {
	case o.steering <- text:
		return nil
	default:
		return errors.New("steering queue is full")
	}
}
func (o *Operation) Respond(id int, approved bool) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	response, ok := o.pending[id]
	if !ok {
		return errors.New("interaction is no longer pending")
	}
	delete(o.pending, id)
	response <- approved
	return nil
}
func (s Service) Start(ctx context.Context, request Request) *Operation {
	ctx, cancel := context.WithCancel(ctx)
	if request.StateDir == "" {
		request.StateDir = s.Executor.StateDir
	}
	if request.RunID == "" {
		request.RunID, _ = newID(time.Now())
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
		return os.WriteFile(filepath.Join(interactionDir, fmt.Sprintf("%d.json", id)), data, 0600)
	}
	events := make(chan agent.Event, 128)
	interactions := make(chan Interaction, 8)
	done := make(chan Completion, 1)
	o := &Operation{Events: events, Interactions: interactions, Done: done, cancel: cancel, steering: make(chan string, 16), pending: map[int]chan bool{}}
	ask := func(ctx context.Context, kind string, preview any) (bool, error) {
		o.mu.Lock()
		o.next++
		id := o.next
		response := make(chan bool, 1)
		o.pending[id] = response
		o.mu.Unlock()
		defer func() { o.mu.Lock(); delete(o.pending, id); o.mu.Unlock() }()
		if err := persistInteraction(struct {
			Version     int
			Status      string
			Interaction Interaction
		}{1, "pending", Interaction{id, kind, preview}}, id); err != nil {
			return false, err
		}
		select {
		case interactions <- Interaction{id, kind, preview}:
		case <-ctx.Done():
			return false, ctx.Err()
		}
		select {
		case approved := <-response:
			if err := persistInteraction(struct {
				Version     int
				Status      string
				Interaction Interaction
				Approved    bool
			}{1, "resolved", Interaction{id, kind, preview}, approved}, id); err != nil {
				return false, err
			}
			return approved, nil
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
	request.OnSupervisor = func(supervisor *orchestrator.Supervisor) { o.mu.Lock(); o.tasks = supervisor; o.mu.Unlock() }
	request.Steering = o.steering
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
		outcome, err := s.Execute(ctx, request)
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
