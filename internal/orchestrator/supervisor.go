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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/cliagent"
	"github.com/gongahkia/gator/internal/privatefs"
)

const TaskVersion = 1

type Task struct {
	Native           *cliagent.NativeState `json:"native,omitempty"`
	AnswersQueued    int                   `json:"answers_queued,omitempty"`
	SessionID        string                `json:"session_id,omitempty"`
	SteeringRequests []string              `json:"steering_requests,omitempty"`
	SourceCapture    string                `json:"source_capture,omitempty"`
	Configuration    RoleConfiguration     `json:"configuration"`
	Limits           agent.Limits          `json:"limits"`
	SelectedInput    string                `json:"selected_input,omitempty"`
	Baseline         string                `json:"baseline_patch,omitempty"`
	Version          int                   `json:"version"`
	ID               string                `json:"id"`
	GlobalID         string                `json:"global_id"`
	ParentRun        string                `json:"parent_run"`
	ParentTask       string                `json:"parent_task,omitempty"`
	Role             string                `json:"role"`
	RoleVersion      int                   `json:"role_version"`
	Source           string                `json:"source"`
	PolicySHA256     string                `json:"policy_sha256"`
	InputSHA256      string                `json:"input_sha256"`
	Dependencies     []string              `json:"dependencies,omitempty"`
	Attempt          int                   `json:"attempt"`
	Previous         string                `json:"previous,omitempty"`
	Status           string                `json:"status"`
	Category         string                `json:"error_category,omitempty"`
	Error            string                `json:"error,omitempty"`
	CreatedAt        time.Time             `json:"created_at"`
	StartedAt        time.Time             `json:"started_at,omitempty"`
	FinishedAt       time.Time             `json:"finished_at,omitempty"`
	Result           Result                `json:"result"`
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
	task     Task
	cancel   context.CancelFunc
	done     chan struct{}
	steering chan string
	controls chan cliagent.SessionControl
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
		role.Name = strings.TrimSpace(role.Name)
		role.Description = strings.TrimSpace(role.Description)
		if err := role.validate(); err != nil {
			cancel()
			return nil, err
		}
		if _, duplicate := s.registry[role.Name]; duplicate {
			cancel()
			return nil, fmt.Errorf("specialist %q is repeated", role.Name)
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
		if err != nil || number < 1 || number > 32 || task.ID != fmt.Sprintf("subagent-%03d", number) || task.GlobalID != options.ParentRun+"/"+task.ID {
			cancel()
			return nil, errors.New("invalid retained task ID")
		}
		if number > s.next {
			s.next = number
		}
		s.used++
		if task.Status == "running" || task.Status == "queued" || task.Status == "awaiting_input" || task.Status == "answering" {
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
	if err := os.Rename(file.Name(), filepath.Join(s.options.StatePath, task.ID+".json")); err != nil {
		return err
	}
	directory, err := os.Open(s.options.StatePath)
	if err != nil {
		return err
	}
	defer directory.Close()
	return privatefs.SyncDir(directory)
}
func (s *Supervisor) Start(request StartRequest) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startLocked(request)
}

func (s *Supervisor) startLocked(request StartRequest) (Task, error) {
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
	sessionID := ""
	attempt := 1
	if request.Continue != "" {
		prior, ok := s.tasks[request.Continue]
		if !ok {
			if retained, exists := s.options.Retained[request.Continue]; exists {
				done := make(chan struct{})
				close(done)
				prior = &taskExecution{task: retained, done: done}
				ok = true
			}
		}
		if !ok || prior.task.Role != request.Agent {
			return Task{}, errors.New("invalid continuation role/task")
		}
		select {
		case <-prior.done:
		default:
			return Task{}, errors.New("cannot continue active task")
		}
		if prior.task.Status == "interrupted" || prior.task.Status == "running" || prior.task.Status == "queued" {
			return Task{}, errors.New("interrupted outcome requires reconciliation; start a fresh inspected assignment")
		}
		attempt = prior.task.Attempt + 1
		if s.registry[request.Agent].Sessions {
			if request.Baseline != "" || prior.task.Result.SessionID == "" {
				return Task{}, errors.New("native continuation requires a retained session and its existing baseline")
			}
			sessionID = prior.task.Result.SessionID
		} else {
			selected = "Selected retained result (untrusted evidence):\n" + prior.task.Result.Summary + "\nAssignment:\n" + selected
		}
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
	task.SourceCapture = s.options.SourceCapture
	task.SessionID = sessionID
	task.Configuration = s.registry[request.Agent].Configuration
	task.Limits, task.SelectedInput, task.Baseline = s.options.Limits, selected, request.Baseline
	if err := s.persist(task); err != nil {
		cancel()
		return Task{}, err
	}
	execution := &taskExecution{task: task, cancel: cancel, done: make(chan struct{})}
	if s.registry[request.Agent].Sessions {
		execution.controls = make(chan cliagent.SessionControl, 16)
	}
	if role := s.registry[request.Agent]; role.Backend == BackendLLM || role.Steering {
		execution.steering = make(chan string, 16)
	}
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
	result, err := s.registry[task.Role].Run(ctx, Invocation{SessionID: task.SessionID, Controls: execution.controls, OnState: func(id string, state cliagent.NativeState) error {
		s.mu.Lock()
		updated := execution.task
		updated.SessionID, updated.Native = id, &state
		if state.Status == "awaiting_input" {
			updated.Status = "awaiting_input"
		} else {
			updated.Status = "running"
		}
		err := s.persist(updated)
		if err == nil {
			execution.task = updated
		}
		s.mu.Unlock()
		if err == nil {
			s.emit(updated)
		}
		return err
	}, ID: task.ID, Task: input, Baseline: baseline, Steering: execution.steering, OnEvent: func(event agent.Event) {
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
	execution := s.tasks[id]
	if execution == nil {
		return errors.New("unknown task")
	}
	if execution.task.Status != "queued" && execution.task.Status != "running" && execution.task.Status != "awaiting_input" && execution.task.Status != "answering" {
		return errors.New("task is no longer active")
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

// Steer queues an instruction for one LLM specialist. It takes effect at the
// agent runner's next model boundary if the specialist is still running.
func (s *Supervisor) Steer(id, instruction string) error {
	if strings.TrimSpace(instruction) == "" || len(instruction) > 8*1024 || !utf8.ValidString(instruction) || strings.ContainsRune(instruction, 0) {
		return errors.New("invalid specialist steering text")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.ctx.Err() != nil {
		return errors.New("supervisor stopped")
	}
	execution := s.tasks[id]
	if execution == nil {
		return errors.New("unknown task")
	}
	if execution.task.Status != "queued" && execution.task.Status != "running" {
		return errors.New("task is no longer active")
	}
	if execution.steering == nil {
		return errors.New("specialist does not support live steering")
	}
	if len(execution.task.SteeringRequests) >= 16 || len(execution.steering) == cap(execution.steering) {
		return errors.New("specialist steering limit reached")
	}
	updated := execution.task
	updated.SteeringRequests = append(append([]string(nil), updated.SteeringRequests...), instruction)
	if err := s.persist(updated); err != nil {
		return err
	}
	execution.task = updated
	select {
	case execution.steering <- instruction:
		return nil
	default:
		return errors.New("specialist steering queue is full")
	}
}
func (s *Supervisor) Close() { s.mu.Lock(); s.closed = true; s.cancel(); s.mu.Unlock(); s.wg.Wait() }

// Answer queues an answer to the exact current question. Acceptance means
// queued, not delivered; the native session journal records actual delivery.
func (s *Supervisor) Answer(id, questionID string, answers map[string][]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.ctx.Err() != nil {
		return errors.New("supervisor stopped")
	}
	e := s.tasks[id]
	if e == nil || e.controls == nil || e.task.Status != "awaiting_input" || e.task.Native == nil || questionID == "" || e.task.Native.QuestionID != questionID {
		return errors.New("no matching pending worker question")
	}
	questions := e.task.Native.Questions
	if len(answers) != len(questions) {
		return errors.New("answer every pending question exactly once")
	}
	copyAnswers := map[string][]string{}
	for _, q := range questions {
		values := answers[q.ID]
		if len(values) != 1 || strings.TrimSpace(values[0]) == "" || len(values[0]) > 8192 || !utf8.ValidString(values[0]) || strings.ContainsRune(values[0], 0) {
			return errors.New("each question requires one valid answer of at most 8192 bytes")
		}
		copyAnswers[q.ID] = append([]string(nil), values...)
	}
	if e.task.AnswersQueued >= 16 || len(e.controls) == cap(e.controls) {
		return errors.New("worker answer limit reached")
	}
	updated := e.task
	updated.AnswersQueued++
	updated.Status = "answering"
	if err := s.persist(updated); err != nil {
		return err
	}
	e.task = updated
	e.controls <- cliagent.SessionControl{ID: fmt.Sprintf("answer-%s-%d", digest(e.task.GlobalID), updated.AnswersQueued), Kind: "answer", QuestionID: questionID, Answers: copyAnswers}
	return nil
}

type taskTool struct {
	s    *Supervisor
	name string
}

func (t taskTool) Definition() agent.ToolDefinition {
	schema := `{"type":"object","additionalProperties":false,"required":["id"],"properties":{"id":{"type":"string"},"timeout_seconds":{"type":"integer","minimum":1,"maximum":300}}}`
	if t.name == "start_agent" {
		schema = `{"type":"object","additionalProperties":false,"required":["agent","task"],"properties":{"agent":{"type":"string"},"task":{"type":"string"},"dependencies":{"type":"array","items":{"type":"string"}},"parent":{"type":"string"},"continue":{"type":"string"},"baseline_patch":{"type":"string"}}}`
	}
	if t.name == "list_agents" {
		schema = `{"type":"object","additionalProperties":false,"properties":{}}`
	}
	if t.name == "steer_agent" {
		schema = `{"type":"object","additionalProperties":false,"required":["id","message"],"properties":{"id":{"type":"string"},"message":{"type":"string","maxLength":8192}}}`
	}
	if t.name == "answer_agent" {
		schema = `{"type":"object","additionalProperties":false,"required":["id","question_id","answers"],"properties":{"id":{"type":"string"},"question_id":{"type":"string"},"answers":{"type":"object","additionalProperties":{"type":"array","items":{"type":"string"},"minItems":1,"maxItems":1}}}}`
	}
	return agent.ToolDefinition{Name: t.name, Description: "Manage a bounded durable specialist task. Authority is fixed by the host. Continuation explicitly selects a retained result.", Parameters: json.RawMessage(schema)}
}
func (t taskTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	if t.name == "list_agents" {
		if err := decodeTask(raw, &struct{}{}); err != nil {
			return agent.ToolResult{}, err
		}
		t.s.mu.Lock()
		defer t.s.mu.Unlock()
		type role struct {
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Backend     Backend `json:"backend"`
			Steering    bool    `json:"steering"`
		}
		roles := []role{}
		tasks := []Task{}
		for _, r := range t.s.registry {
			roles = append(roles, role{r.Name, r.Description, r.Backend, r.Backend == BackendLLM || r.Steering})
		}
		for _, item := range t.s.tasks {
			tasks = append(tasks, item.task)
		}
		sort.Slice(roles, func(i, j int) bool { return roles[i].Name < roles[j].Name })
		sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
		data, err := json.Marshal(map[string]any{"agents": roles, "tasks": tasks, "remaining": t.s.options.MaxDelegations - t.s.used, "max_parallel": t.s.options.MaxParallel})
		return agent.ToolResult{Content: string(data)}, err
	}
	var task Task
	var err error
	if t.name == "start_agent" {
		var request StartRequest
		if err = decodeTask(raw, &request); err == nil {
			task, err = t.s.Start(request)
		}
	} else {
		var input struct {
			ID         string              `json:"id"`
			Timeout    int                 `json:"timeout_seconds"`
			Message    string              `json:"message"`
			QuestionID string              `json:"question_id"`
			Answers    map[string][]string `json:"answers"`
		}
		if err = decodeTask(raw, &input); err != nil {
			return agent.ToolResult{}, err
		}
		switch t.name {
		case "answer_agent":
			err = t.s.Answer(input.ID, input.QuestionID, input.Answers)
			if err == nil {
				task, err = t.s.Inspect(input.ID)
			}
		case "steer_agent":
			err = t.s.Steer(input.ID, input.Message)
			if err == nil {
				task, err = t.s.Inspect(input.ID)
			}
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
			parent := ctx
			ctx, cancel := context.WithTimeout(ctx, time.Duration(input.Timeout)*time.Second)
			defer cancel()
			task, err = t.s.Await(ctx, input.ID)
			if errors.Is(err, context.DeadlineExceeded) && parent.Err() == nil {
				task, err = t.s.Inspect(input.ID)
			}
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
	return []agent.Tool{taskTool{s, "list_agents"}, taskTool{s, "start_agent"}, taskTool{s, "inspect_agent"}, taskTool{s, "await_agent"}, taskTool{s, "steer_agent"}, taskTool{s, "answer_agent"}, taskTool{s, "cancel_agent"}}
}

// StartBatch validates the complete fresh-context batch before any child starts.
func (s *Supervisor) StartBatch(requests []StartRequest) ([]Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(requests) < 1 || len(requests) > s.options.MaxParallel || s.used+len(requests) > s.options.MaxDelegations {
		return nil, errors.New("invalid batch or aggregate invocation budget exhausted")
	}
	for _, request := range requests {
		if _, ok := s.registry[request.Agent]; !ok {
			return nil, errors.New("unknown specialist")
		}
		if strings.TrimSpace(request.Task) == "" || len(request.Task) > maxTaskBytes || strings.ContainsRune(request.Task, 0) {
			return nil, errors.New("invalid task input")
		}
		if len(request.Dependencies) != 0 || request.Parent != "" || request.Continue != "" {
			return nil, errors.New("batch accepts fresh independent tasks only")
		}
	}
	var tasks []Task
	for _, request := range requests {
		task, err := s.startLocked(request)
		if err != nil {
			for _, prior := range tasks {
				s.tasks[prior.ID].cancel()
			}
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// ReadTasks inspects checkpoints without acquiring ownership or repeating effects.
func ReadTasks(directory string) ([]Task, error) {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var tasks []Task
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		var task Task
		if err := json.Unmarshal(data, &task); err != nil || task.Version != TaskVersion || task.ID+".json" != entry.Name() {
			return nil, errors.New("invalid retained task checkpoint")
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}
