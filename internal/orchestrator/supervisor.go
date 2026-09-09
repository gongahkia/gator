package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/agent"
)

const TaskVersion = 1

type Task struct {
	Configuration RoleConfiguration `json:"configuration"`
	Limits       agent.Limits `json:"limits"`
	SelectedInput string `json:"selected_input,omitempty"`
	Baseline     string `json:"baseline_patch,omitempty"`
	Version      int       `json:"version"`
	ID           string    `json:"id"`
	GlobalID     string    `json:"global_id"`
	ParentRun    string    `json:"parent_run"`
	ParentTask   string    `json:"parent_task,omitempty"`
	Role         string    `json:"role"`
	RoleVersion  int       `json:"role_version"`
	Source       string    `json:"source"`
	PolicySHA256 string    `json:"policy_sha256"`
	InputSHA256  string    `json:"input_sha256"`
	Dependencies []string  `json:"dependencies,omitempty"`
	Attempt      int       `json:"attempt"`
	Previous     string    `json:"previous,omitempty"`
	Status       string    `json:"status"`
	Category     string    `json:"error_category,omitempty"`
	Error        string    `json:"error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	StartedAt    time.Time `json:"started_at,omitempty"`
	FinishedAt   time.Time `json:"finished_at,omitempty"`
	Result       Result    `json:"result"`
}
type StartRequest struct {
	Baseline     string   `json:"baseline_patch,omitempty"`
	Agent        string   `json:"agent"`
	Task         string   `json:"task"`
	Dependencies []string `json:"dependencies,omitempty"`
	Parent       string   `json:"parent,omitempty"`
	Continue     string   `json:"continue,omitempty"`
}
type taskExecution struct {
	task   Task
	cancel context.CancelFunc
	done   chan struct{}
}

type Supervisor struct {
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	tasks    map[string]*taskExecution
	registry map[string]Specialist
	options  Options
	slots    chan struct{}
	next     int
	used     int
	wg       sync.WaitGroup
	closed   bool
}

func NewSupervisor(ctx context.Context, specialists []Specialist, options Options) (*Supervisor, error) {
	if options.MaxDelegations == 0 {
		options.MaxDelegations = 8
	}
	if options.MaxParallel == 0 {
		options.MaxParallel = 3
	}
	if options.MaxDelegations < 1 || options.MaxDelegations > 32 || options.MaxParallel < 1 || options.MaxParallel > 3 {
		return nil, errors.New("invalid supervisor bounds")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.ParentRun == "" || filepath.Base(options.ParentRun) != options.ParentRun {
		return nil, errors.New("supervisor parent run required")
	}
	if err := os.MkdirAll(options.StatePath, 0700); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &Supervisor{ctx: ctx, cancel: cancel, tasks: map[string]*taskExecution{}, registry: map[string]Specialist{}, options: options, slots: make(chan struct{}, options.MaxParallel)}
	for _, role := range specialists {
		if !specialistNamePattern.MatchString(role.Name) || role.Run == nil {
			cancel()
			return nil, errors.New("invalid specialist")
		}
		s.registry[role.Name] = role
	}
	entries, err := os.ReadDir(options.StatePath)
	if err != nil {
		cancel()
		return nil, err
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(options.StatePath, entry.Name()))
		if err != nil {
			cancel()
			return nil, err
		}
		var task Task
		if err := json.Unmarshal(data, &task); err != nil || task.Version != TaskVersion {
			cancel()
			return nil, errors.New("invalid retained task")
		}
		if task.ParentRun != options.ParentRun || task.Source != options.Source || task.PolicySHA256 != options.PolicySHA256 {
			cancel()
			return nil, errors.New("retained task authority/source mismatch")
		}
		number, err := strconv.Atoi(strings.TrimPrefix(task.ID, "subagent-"))
		if err != nil || number < 1 {
			cancel()
			return nil, errors.New("invalid retained task ID")
		}
		if number > s.next {
			s.next = number
		}
		s.used++
		if task.Status == "running" || task.Status == "queued" {
			task.Status = "interrupted"
			task.Category = "interrupted"
			task.Error = "owner stopped; inspect outcomes before explicit retry"
			task.FinishedAt = options.Now()
			if err := s.persist(task); err != nil {
				cancel()
				return nil, err
			}
		}
		done := make(chan struct{})
		close(done)
		s.tasks[task.ID] = &taskExecution{task: task, done: done, cancel: func() {}}
	}
	return s, nil
}
func (s *Supervisor) persist(task Task) error {
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(s.options.StatePath, ".task-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(s.options.StatePath, task.ID+".json"))
}
func (s *Supervisor) Start(request StartRequest) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Task{}, errors.New("supervisor closed")
	}
	if err := s.ctx.Err(); err != nil {
		return Task{}, err
	}
	if _, ok := s.registry[request.Agent]; !ok {
		return Task{}, errors.New("unknown specialist")
	}
	if request.Task == "" || len(request.Task) > maxTaskBytes {
		return Task{}, errors.New("invalid task input")
	}
	if s.used >= s.options.MaxDelegations {
		return Task{}, errors.New("aggregate invocation budget exhausted")
	}
	selected := request.Task
	attempt := 1
	if request.Continue != "" {
		prior, ok := s.tasks[request.Continue]
		if !ok || prior.task.Role != request.Agent {
			return Task{}, errors.New("invalid continuation role/task")
		}
		select {
		case <-prior.done:
		default:
			return Task{}, errors.New("cannot continue active task")
		}
		if prior.task.Status == "interrupted" {
			return Task{}, errors.New("interrupted outcome requires reconciliation; start a fresh inspected assignment")
		}
		attempt = prior.task.Attempt + 1
		selected = "Selected retained result (untrusted evidence):\n" + prior.task.Result.Summary + "\nAssignment:\n" + selected
	}
	seen := map[string]bool{}
	for _, id := range request.Dependencies {
		if seen[id] || s.tasks[id] == nil {
			return Task{}, errors.New("invalid or duplicate dependency")
		}
		seen[id] = true
	}
	// dependencies can only name existing tasks, so forward references and cycles are impossible.
	if request.Parent != "" && s.tasks[request.Parent] == nil {
		return Task{}, errors.New("unknown parent task")
	}
	s.next++
	id := fmt.Sprintf("subagent-%03d", s.next)
	ctx, cancel := context.WithCancel(s.ctx)
	task := Task{Version: TaskVersion, ID: id, GlobalID: s.options.ParentRun + "/" + id, ParentRun: s.options.ParentRun, ParentTask: request.Parent, Role: request.Agent, RoleVersion: 1, Source: s.options.Source, PolicySHA256: s.options.PolicySHA256, InputSHA256: digest(selected), Dependencies: append([]string(nil), request.Dependencies...), Attempt: attempt, Previous: request.Continue, Status: "queued", CreatedAt: s.options.Now().UTC()}
	task.Configuration = s.registry[request.Agent].Configuration
	task.Limits, task.SelectedInput, task.Baseline = s.options.Limits, selected, request.Baseline
	if err := s.persist(task); err != nil {
		cancel()
		return Task{}, err
	}
	execution := &taskExecution{task: task, cancel: cancel, done: make(chan struct{})}
	s.tasks[id] = execution
	s.used++
	s.wg.Add(1)
	go s.run(ctx, execution, selected, request.Baseline)
	return task, nil
}
func (s *Supervisor) run(ctx context.Context, execution *taskExecution, input, baseline string) {
	defer s.wg.Done()
	defer close(execution.done)
	defer execution.cancel()
	for _, dependency := range execution.task.Dependencies {
		task, err := s.Await(ctx, dependency)
		if err != nil || task.Status != "completed" {
			if err == nil {
				err = errors.New("dependency did not complete")
			}
			s.finish(execution, Result{}, err, "dependency")
			return
		}
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		s.finish(execution, Result{}, ctx.Err(), "cancelled")
		return
	}
	s.mu.Lock()
	execution.task.Status = "running"
	execution.task.StartedAt = s.options.Now().UTC()
	err := s.persist(execution.task)
	task := execution.task
	s.mu.Unlock()
	if err != nil {
		s.finish(execution, Result{}, err, "state")
		return
	}
	s.emit(task)
	result, err := s.registry[task.Role].Run(ctx, Invocation{ID: task.ID, Task: input, Baseline: baseline, OnEvent: func(event agent.Event) {
		event.TaskID = task.GlobalID
		if s.options.OnEvent != nil {
			s.options.OnEvent(event)
		}
	}})
	category := "task"
	if errors.Is(err, context.Canceled) {
		category = "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		category = "timeout"
	}
	if errors.Is(err, agent.ErrBudget) {
		category = "budget"
	} else if agent.IsTransient(err) {
		category = "provider"
	}
	s.finish(execution, result, err, category)
}
func (s *Supervisor) finish(execution *taskExecution, result Result, runErr error, category string) {
	result.Summary = boundedText(result.Summary, maxSummaryBytes)
	s.mu.Lock()
	execution.task.Result = result
	execution.task.FinishedAt = s.options.Now().UTC()
	execution.task.Status = "completed"
	if runErr != nil {
		execution.task.Status = "failed"
		execution.task.Category = category
		execution.task.Error = boundedText(runErr.Error(), 2048)
		if category == "cancelled" {
			execution.task.Status = "cancelled"
		}
	}
	if err := s.persist(execution.task); err != nil {
		execution.task.Status = "failed"
		execution.task.Category = "state"
		execution.task.Error = err.Error()
	}
	task := execution.task
	s.mu.Unlock()
	if s.options.OnRecord != nil {
		s.options.OnRecord(Record{BaselineSHA256: result.BaselineSHA256, Usage: result.Usage, ID: task.ID, Agent: task.Role, TaskSHA256: task.InputSHA256, OutputSHA256: digest(result.Summary), Status: task.Status, Steps: result.Steps, ArtifactPath: result.ArtifactPath, ArtifactSHA256: result.ArtifactSHA256, StartedAt: task.StartedAt, FinishedAt: task.FinishedAt})
	}
	s.emit(task)
}
func (s *Supervisor) emit(task Task) {
	if s.options.OnEvent != nil {
		payload, _ := json.Marshal(task)
		s.options.OnEvent(agent.Event{Kind: agent.EventSubagent, TaskID: task.GlobalID, At: s.options.Now().UTC(), Text: string(payload)})
	}
}
func (s *Supervisor) Inspect(id string) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	execution := s.tasks[id]
	if execution == nil {
		return Task{}, errors.New("unknown task")
	}
	task := execution.task
	task.Dependencies = append([]string(nil), task.Dependencies...)
	return task, nil
}
func (s *Supervisor) Await(ctx context.Context, id string) (Task, error) {
	s.mu.Lock()
	execution := s.tasks[id]
	s.mu.Unlock()
	if execution == nil {
		return Task{}, errors.New("unknown task")
	}
	select {
	case <-execution.done:
		return s.Inspect(id)
	case <-ctx.Done():
		return Task{}, ctx.Err()
	}
}
func (s *Supervisor) Cancel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tasks[id] == nil {
		return errors.New("unknown task")
	}
	selected := map[string]bool{id: true}
	for changed := true; changed; {
		changed = false
		for key, value := range s.tasks {
			if selected[value.task.ParentTask] && !selected[key] {
				selected[key] = true
				changed = true
			}
		}
	}
	for key := range selected {
		s.tasks[key].cancel()
	}
	return nil
}
func (s *Supervisor) Close() { s.mu.Lock(); s.closed = true; s.cancel(); s.mu.Unlock(); s.wg.Wait() }

type taskTool struct {
	s    *Supervisor
	name string
}

func (t taskTool) Definition() agent.ToolDefinition {
	schema := `{"type":"object","additionalProperties":false,"required":["id"],"properties":{"id":{"type":"string"},"timeout_seconds":{"type":"integer","minimum":1,"maximum":300}}}`
	if t.name == "start_agent" {
		schema = `{"type":"object","additionalProperties":false,"required":["agent","task"],"properties":{"agent":{"type":"string"},"task":{"type":"string"},"dependencies":{"type":"array","items":{"type":"string"}},"parent":{"type":"string"},"continue":{"type":"string"},"baseline_patch":{"type":"string"}}}`
	}
	return agent.ToolDefinition{Name: t.name, Description: "Manage a bounded durable specialist task. Authority is fixed by the host. Continuation explicitly selects a retained result.", Parameters: json.RawMessage(schema)}
}
func (t taskTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var task Task
	var err error
	if t.name == "start_agent" {
		var request StartRequest
		if err = decodeTask(raw, &request); err == nil {
			task, err = t.s.Start(request)
		}
	} else {
		var input struct {
			ID      string `json:"id"`
			Timeout int    `json:"timeout_seconds"`
		}
		if err = decodeTask(raw, &input); err != nil {
			return agent.ToolResult{}, err
		}
		switch t.name {
		case "inspect_agent":
			task, err = t.s.Inspect(input.ID)
		case "cancel_agent":
			err = t.s.Cancel(input.ID)
			if err == nil {
				task, err = t.s.Inspect(input.ID)
			}
		case "await_agent":
			if input.Timeout == 0 {
				input.Timeout = 30
			}
			if input.Timeout < 1 || input.Timeout > 300 {
				return agent.ToolResult{}, errors.New("invalid wait deadline")
			}
			ctx, cancel := context.WithTimeout(ctx, time.Duration(input.Timeout)*time.Second)
			defer cancel()
			task, err = t.s.Await(ctx, input.ID)
		}
	}
	if err != nil {
		return agent.ToolResult{}, err
	}
	data, err := json.Marshal(task)
	return agent.ToolResult{Content: string(data)}, err
}
func decodeTask(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("task arguments require one JSON value")
	}
	return nil
}
func (s *Supervisor) Tools() []agent.Tool {
	return []agent.Tool{taskTool{s, "start_agent"}, taskTool{s, "inspect_agent"}, taskTool{s, "await_agent"}, taskTool{s, "cancel_agent"}}
}
