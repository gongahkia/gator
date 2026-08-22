// Package rpc implements Gator's JSONL process-integration boundary.
package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/model"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/tools"
	protocol "github.com/gongahkia/gator/rpc"
)

const maxRequestBytes = 1024 * 1024

// Config supplies environment-specific behavior for a protocol server.
type Config struct {
	Input           io.Reader
	Output          io.Writer
	RepositoryPath  string
	StateDir        string
	DefaultProvider string
	DefaultModel    string
	ResolveProvider func(provider, model string) (string, string, error)
	NewExecutor     func(provider, model, baseURL string) (gatorrun.Executor, error)
}

// Server processes requests until its input closes. One native run may be
// active for each request ID, which lets a client submit steer or cancel while
// still receiving ordered events for that run.
type Server struct {
	config Config
	write  sync.Mutex
	mu     sync.Mutex
	active map[string]*activeRun
	wait   sync.WaitGroup
}

type activeRun struct {
	steering chan<- string
	approve  chan tools.CommandDecision
	cancel   context.CancelFunc
}

// New constructs a protocol server.
func New(config Config) (*Server, error) {
	if config.Input == nil || config.Output == nil {
		return nil, errors.New("RPC input and output are required")
	}
	if strings.TrimSpace(config.RepositoryPath) == "" {
		return nil, errors.New("RPC repository path is required")
	}
	if strings.TrimSpace(config.DefaultProvider) == "" {
		return nil, errors.New("RPC default provider is required")
	}
	if config.NewExecutor == nil {
		return nil, errors.New("RPC executor factory is required")
	}
	return &Server{config: config, active: make(map[string]*activeRun)}, nil
}

// Serve blocks until input closes and all accepted runs reach a terminal
// response. Per-request failures are represented on the wire and do not end
// the server, so one malformed IDE message cannot break a session.
func (s *Server) Serve(ctx context.Context) error {
	scanner := bufio.NewScanner(s.config.Input)
	scanner.Buffer(make([]byte, 4096), maxRequestBytes)
	for scanner.Scan() {
		var request protocol.Request
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			s.send(protocol.Message{Version: protocol.Version, Type: "error", Error: &protocol.Error{Code: "invalid_request", Message: "request must be valid JSON"}, At: time.Now().UTC()})
			continue
		}
		if err := s.handle(ctx, request); err != nil {
			s.sendError(request.ID, "invalid_request", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read RPC request: %w", err)
	}
	s.wait.Wait()
	return nil
}

func (s *Server) handle(ctx context.Context, request protocol.Request) error {
	if request.Version != protocol.Version {
		return fmt.Errorf("unsupported protocol version %d", request.Version)
	}
	if !validID(request.ID) {
		return errors.New("request id must contain 1-128 letters, digits, '.', '_', or '-'")
	}
	switch request.Method {
	case protocol.MethodCapabilities:
		s.sendResult(request.ID, map[string]any{"protocol_version": protocol.Version, "methods": []string{protocol.MethodCapabilities, protocol.MethodRun, protocol.MethodResume, protocol.MethodSteer, protocol.MethodCancel, protocol.MethodApprove, protocol.MethodStatus, protocol.MethodThreads}})
		return nil
	case protocol.MethodStatus:
		s.mu.Lock()
		active := make([]string, 0, len(s.active))
		for id := range s.active {
			active = append(active, id)
		}
		s.mu.Unlock()
		s.sendResult(request.ID, map[string]any{"active_run_ids": active})
		return nil
	case protocol.MethodThreads:
		return s.threads(request)
	case protocol.MethodSteer:
		return s.steer(request)
	case protocol.MethodCancel:
		return s.cancel(request)
	case protocol.MethodApprove:
		return s.approve(request)
	case protocol.MethodRun:
		return s.startRun(ctx, request)
	case protocol.MethodResume:
		return s.startResume(ctx, request)
	default:
		return fmt.Errorf("unknown method %q", request.Method)
	}
}

func (s *Server) threads(request protocol.Request) error {
	var (
		threads []journal.RecentThread
		err     error
	)
	if request.Params.All {
		threads, err = journal.ListAllRecentThreads(s.config.StateDir, 1_000)
	} else {
		threads, err = journal.ListRecentThreads(s.config.StateDir, s.config.RepositoryPath, 1_000)
	}
	if err != nil {
		return err
	}
	result := make([]map[string]any, 0, len(threads))
	for _, thread := range threads {
		result = append(result, map[string]any{
			"id": thread.ID, "repository": thread.Repository, "provider": thread.Provider, "model": thread.Model,
			"task": thread.Task, "turn_count": thread.TurnCount, "updated_at": thread.UpdatedAt,
			"available": thread.Available,
		})
	}
	s.sendResult(request.ID, map[string]any{"threads": result})
	return nil
}

func (s *Server) steer(request protocol.Request) error {
	if !validID(request.Params.RunID) {
		return errors.New("steer requires a valid run_id")
	}
	message := strings.TrimSpace(request.Params.Message)
	if message == "" {
		return errors.New("steer requires a message")
	}
	s.mu.Lock()
	run, found := s.active[request.Params.RunID]
	s.mu.Unlock()
	if !found {
		return fmt.Errorf("run %q is not active", request.Params.RunID)
	}
	select {
	case run.steering <- message:
		s.sendResult(request.ID, map[string]any{"run_id": request.Params.RunID, "accepted": true})
		return nil
	default:
		return fmt.Errorf("run %q steering queue is full", request.Params.RunID)
	}
}

func (s *Server) cancel(request protocol.Request) error {
	if !validID(request.Params.RunID) {
		return errors.New("cancel requires a valid run_id")
	}
	s.mu.Lock()
	run, found := s.active[request.Params.RunID]
	s.mu.Unlock()
	if !found {
		return fmt.Errorf("run %q is not active", request.Params.RunID)
	}
	run.cancel()
	s.sendResult(request.ID, map[string]any{"run_id": request.Params.RunID, "cancel_requested": true})
	return nil
}

func (s *Server) startRun(parent context.Context, request protocol.Request) error {
	mode, err := parseMode(request.Params.Mode)
	if err != nil {
		return err
	}
	if strings.TrimSpace(request.Params.Task) == "" {
		return errors.New("run requires a task")
	}
	if mode == gatorrun.ExecuteMode && len(request.Params.Verify) == 0 {
		return errors.New("execute runs require at least one verify argv; use mode plan for read-only exploration")
	}
	provider, modelName, err := s.resolveModel(request.Params.Provider, request.Params.Model)
	if err != nil {
		return err
	}
	executor, err := s.config.NewExecutor(provider, modelName, request.Params.BaseURL)
	if err != nil {
		return err
	}
	return s.start(parent, request.ID, func(ctx context.Context, steering <-chan string, emit agent.EventSink) (gatorrun.Outcome, error) {
		return executor.Execute(ctx, gatorrun.Request{
			RepositoryPath: s.config.RepositoryPath, Task: request.Params.Task, Provider: provider, Model: modelName,
			BaseURL: request.Params.BaseURL, MaxSteps: request.Params.MaxSteps, Verification: request.Params.Verify,
			Scopes:           request.Params.Scopes,
			Profile:          request.Params.Profile,
			BaseRef:          request.Params.BaseRef,
			CopyIgnoredFiles: request.Params.CopyIgnoredFiles,
			Scouts:           request.Params.Scouts,
			StateDir:         s.config.StateDir, Mode: mode, ForceCompaction: request.Params.Compact, Steering: steering, OnEvent: emit,
			Approve: s.approveFor(request.ID),
		})
	})
}

func (s *Server) startResume(parent context.Context, request protocol.Request) error {
	if strings.TrimSpace(request.Params.StatePath) == "" {
		return errors.New("resume requires a state_path")
	}
	if strings.TrimSpace(request.Params.Task) == "" {
		return errors.New("resume requires a task")
	}
	previous, err := journal.LoadSession(request.Params.StatePath)
	if err != nil {
		return err
	}
	provider, modelName, err := s.resolveModel(previous.Provider, previous.Model)
	if err != nil {
		return fmt.Errorf("load retained provider: %w", err)
	}
	executor, err := s.config.NewExecutor(provider, modelName, previous.BaseURL)
	if err != nil {
		return err
	}
	return s.start(parent, request.ID, func(ctx context.Context, steering <-chan string, emit agent.EventSink) (gatorrun.Outcome, error) {
		return executor.Resume(ctx, previous, request.Params.StatePath, request.Params.Task, gatorrun.Request{
			MaxSteps: request.Params.MaxSteps, StateDir: s.config.StateDir, ForceCompaction: request.Params.Compact, Steering: steering, OnEvent: emit,
			Scopes:  request.Params.Scopes,
			Approve: s.approveFor(request.ID),
		})
	})
}

func (s *Server) start(parent context.Context, id string, execute func(context.Context, <-chan string, agent.EventSink) (gatorrun.Outcome, error)) error {
	s.mu.Lock()
	if _, exists := s.active[id]; exists {
		s.mu.Unlock()
		return fmt.Errorf("request id %q is already active", id)
	}
	ctx, cancel := context.WithCancel(parent)
	steering := make(chan string, 16)
	s.active[id] = &activeRun{steering: steering, approve: make(chan tools.CommandDecision, 1), cancel: cancel}
	s.mu.Unlock()
	s.sendResult(id, map[string]any{"accepted": true, "run_id": id})
	s.wait.Add(1)
	go func() {
		defer s.wait.Done()
		defer cancel()
		defer func() {
			s.mu.Lock()
			delete(s.active, id)
			s.mu.Unlock()
		}()
		outcome, err := execute(ctx, steering, func(event agent.Event) { s.sendEvent(id, event) })
		if err != nil {
			s.sendError(id, "run_failed", err)
			return
		}
		s.sendResult(id, map[string]any{
			"thread_id": outcome.ThreadID, "state_path": outcome.StatePath, "worktree_path": outcome.Worktree.Path,
			"final_text": outcome.Result.FinalText, "steps": outcome.Result.Steps,
		})
	}()
	return nil
}

func (s *Server) resolveModel(provider, modelName string) (string, string, error) {
	if strings.TrimSpace(provider) == "" {
		provider = s.config.DefaultProvider
	}
	if strings.TrimSpace(modelName) == "" {
		modelName = s.config.DefaultModel
	}
	if s.config.ResolveProvider != nil {
		return s.config.ResolveProvider(provider, modelName)
	}
	parsed, err := model.ParseProvider(provider)
	if err != nil {
		return "", "", err
	}
	return string(parsed), model.EffectiveModel(parsed, modelName), nil
}

func (s *Server) sendEvent(id string, event agent.Event) {
	message := protocol.Event{Kind: string(event.Kind), Step: event.Step, Text: event.Text, ToolError: event.ToolError, Argv: event.Argv}
	if event.ToolCall != nil {
		message.Tool = event.ToolCall.Name
	}
	s.send(protocol.Message{Version: protocol.Version, Type: "event", ID: id, Event: &message, At: event.At.UTC()})
}

func (s *Server) approveFor(id string) func(context.Context, []string) (tools.CommandDecision, error) {
	return func(ctx context.Context, _ []string) (tools.CommandDecision, error) {
		s.mu.Lock()
		run, ok := s.active[id]
		s.mu.Unlock()
		if !ok {
			return tools.CommandDeny, fmt.Errorf("run %q is not active", id)
		}
		select {
		case decision := <-run.approve:
			return decision, nil
		case <-ctx.Done():
			return tools.CommandDeny, ctx.Err()
		}
	}
}

func (s *Server) approve(request protocol.Request) error {
	if !validID(request.Params.RunID) {
		return errors.New("approve requires a valid run_id")
	}
	decision, err := parseCommandDecision(request.Params.Decision)
	if err != nil {
		return err
	}
	s.mu.Lock()
	run, found := s.active[request.Params.RunID]
	s.mu.Unlock()
	if !found {
		return fmt.Errorf("run %q is not active", request.Params.RunID)
	}
	select {
	case run.approve <- decision:
		s.sendResult(request.ID, map[string]any{"run_id": request.Params.RunID, "decision": decision.String(), "accepted": true})
		return nil
	default:
		return fmt.Errorf("run %q has no pending command approval", request.Params.RunID)
	}
}

func parseCommandDecision(value string) (tools.CommandDecision, error) {
	switch strings.TrimSpace(value) {
	case "allow_once":
		return tools.CommandAllowOnce, nil
	case "allow_always":
		return tools.CommandAllowAlways, nil
	case "deny":
		return tools.CommandDeny, nil
	default:
		return tools.CommandDeny, fmt.Errorf("unknown approval decision %q; choose allow_once, allow_always, or deny", value)
	}
}

func (s *Server) sendResult(id string, result any) {
	s.send(protocol.Message{Version: protocol.Version, Type: "response", ID: id, Result: result, At: time.Now().UTC()})
}

func (s *Server) sendError(id, code string, err error) {
	s.send(protocol.Message{Version: protocol.Version, Type: "error", ID: id, Error: &protocol.Error{Code: code, Message: err.Error()}, At: time.Now().UTC()})
}

func (s *Server) send(message protocol.Message) {
	s.write.Lock()
	defer s.write.Unlock()
	_ = json.NewEncoder(s.config.Output).Encode(message)
}

func parseMode(value string) (gatorrun.Mode, error) {
	switch strings.TrimSpace(value) {
	case "", "execute":
		return gatorrun.ExecuteMode, nil
	case "plan":
		return gatorrun.PlanMode, nil
	default:
		return 0, fmt.Errorf("unknown run mode %q; choose execute or plan", value)
	}
}

func validID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}
