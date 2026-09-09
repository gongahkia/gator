// Package orchestrator exposes fresh-context specialist agents as bounded
// tools to a user-facing manager agent. It implements the manager-as-tools
// orchestration pattern without coupling Gator's provider-neutral runtime to a
// second model SDK.
package orchestrator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/agent"
)

const (
	defaultDelegationBudget = 8
	defaultParallelLimit    = 3
	maxBatchSize            = 3
	maxTaskBytes            = 8 * 1024
	maxSummaryBytes         = 16 * 1024
)

var specialistNamePattern = regexp.MustCompile(`\A[a-z][a-z0-9_]{0,63}\z`)

// Invocation is the isolated assignment passed to one specialist.
type Invocation struct {
	Baseline string
	OnEvent  agent.EventSink
	ID       string
	Task     string
}

// Result is the bounded information returned from a specialist to its manager.
// Artifact fields are optional trusted evidence supplied by the host.
type Result struct {
	BaselineSHA256 string
	Usage          agent.Usage
	Summary        string
	Steps          int
	ArtifactPath   string
	ArtifactSHA256 string
}

// Specialist describes one capability visible to the manager. Run must start
// from fresh context and return only the information needed by the manager.
type Specialist struct {
	Configuration RoleConfiguration
	Name          string
	Description   string
	Run           func(context.Context, Invocation) (Result, error)
}

// Record is trusted orchestration evidence for one specialist invocation.
type Record struct {
	BaselineSHA256 string
	Usage          agent.Usage
	ID             string
	Agent          string
	TaskSHA256     string
	OutputSHA256   string
	Status         string
	Steps          int
	ArtifactPath   string
	ArtifactSHA256 string
	StartedAt      time.Time
	FinishedAt     time.Time
}

// Options configures one manager's bounded delegation surface.
type Options struct {
	SourceCapture  string
	Retained       map[string]Task
	Limits         agent.Limits
	StatePath      string
	ParentRun      string
	Source         string
	PolicySHA256   string
	Supervisor     *Supervisor
	MaxDelegations int
	MaxParallel    int
	Now            func() time.Time
	OnEvent        agent.EventSink
	OnRecord       func(Record)
}

type RoleConfiguration struct {
	Version  int    `json:"version"`
	Provider string `json:"provider"`
	MaxSteps int    `json:"max_steps"`
}

// Tools returns the model-callable delegation surface for a small specialist
// registry. A single tool keeps the manager's prompt compact and supports one
// to three independent assignments in parallel.
func Tools(specialists []Specialist, options Options) ([]agent.Tool, error) {
	registry := make(map[string]Specialist, len(specialists))
	ordered := append([]Specialist(nil), specialists...)
	for index, specialist := range ordered {
		specialist.Name = strings.TrimSpace(specialist.Name)
		specialist.Description = strings.TrimSpace(specialist.Description)
		if !specialistNamePattern.MatchString(specialist.Name) || specialist.Description == "" || len(specialist.Description) > 1024 || specialist.Run == nil {
			return nil, fmt.Errorf("specialist %d is invalid", index+1)
		}
		if _, duplicate := registry[specialist.Name]; duplicate {
			return nil, fmt.Errorf("specialist %q is repeated", specialist.Name)
		}
		registry[specialist.Name] = specialist
	}
	if len(registry) == 0 {
		return nil, nil
	}
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].Name < ordered[right].Name })
	now := options.Now
	if now == nil {
		now = time.Now
	}
	budget := options.MaxDelegations
	if budget == 0 {
		budget = defaultDelegationBudget
	}
	parallel := options.MaxParallel
	if parallel == 0 {
		parallel = defaultParallelLimit
	}
	if budget < 1 || budget > 32 || parallel < 1 || parallel > maxBatchSize {
		return nil, errors.New("specialist delegation limits are invalid")
	}
	return []agent.Tool{&delegateTool{
		registry: registry, ordered: ordered, remaining: budget, parallel: parallel,
		supervisor: options.Supervisor, now: now, onEvent: options.OnEvent, onRecord: options.OnRecord,
	}}, nil
}

// LLMSpecialist adapts Gator's existing provider-neutral agent runner into a
// fresh-context specialist.
func LLMSpecialist(name, description string, model agent.Model, tools []agent.Tool, system string, maxSteps int, now func() time.Time) Specialist {
	return Specialist{
		Name: name, Description: description,
		Run: func(ctx context.Context, invocation Invocation) (Result, error) {
			usage := &agent.Budget{Limits: agent.Limits{ModelRequests: 4096}}
			result, err := (agent.Runner{Model: agent.WithBudget(model, usage), Tools: tools, Now: now}).Run(ctx, agent.RunOptions{
				Task: invocation.Task, System: system, MaxSteps: maxSteps, OnEvent: invocation.OnEvent,
			})
			return Result{Summary: result.FinalText, Steps: result.Steps, Usage: usage.Usage()}, err
		},
	}
}

type delegateTool struct {
	supervisor *Supervisor
	mu         sync.Mutex
	registry   map[string]Specialist
	ordered    []Specialist
	remaining  int
	nextID     int
	parallel   int
	now        func() time.Time
	onEvent    agent.EventSink
	onRecord   func(Record)
}

func (t *delegateTool) Definition() agent.ToolDefinition {
	names := make([]string, 0, len(t.ordered))
	lines := make([]string, 0, len(t.ordered))
	for _, specialist := range t.ordered {
		names = append(names, specialist.Name)
		lines = append(lines, specialist.Name+": "+specialist.Description)
	}
	schema, _ := json.Marshal(map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"tasks"},
		"properties": map[string]any{"tasks": map[string]any{
			"type": "array", "minItems": 1, "maxItems": t.parallel,
			"items": map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"agent", "task"},
				"properties": map[string]any{
					"agent":          map[string]any{"type": "string", "enum": names},
					"baseline_patch": map[string]any{"type": "string"},
					"task":           map[string]any{"type": "string", "minLength": 1, "maxLength": maxTaskBytes},
				},
			},
		}},
	})
	return agent.ToolDefinition{
		Name: "delegate_agents",
		Description: "Delegate one bounded task, or several independent tasks in parallel, to fresh-context specialists. " +
			"The manager remains responsible for the user-facing answer and must verify material results. Available specialists: " + strings.Join(lines, "; "),
		Parameters: schema,
	}
}

type assignment struct {
	agent string
	task  string
	id    string
}

type delegatedResult struct {
	ID             string `json:"id"`
	Agent          string `json:"agent"`
	Status         string `json:"status"`
	Summary        string `json:"summary,omitempty"`
	Error          string `json:"error,omitempty"`
	Steps          int    `json:"steps,omitempty"`
	ArtifactPath   string `json:"artifact_path,omitempty"`
	ArtifactSHA256 string `json:"artifact_sha256,omitempty"`
}

func (t *delegateTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var input struct {
		Tasks []struct {
			Agent    string `json:"agent"`
			Task     string `json:"task"`
			Baseline string `json:"baseline_patch,omitempty"`
		} `json:"tasks"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return agent.ToolResult{}, fmt.Errorf("decode delegate_agents arguments: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return agent.ToolResult{}, errors.New("decode delegate_agents arguments: multiple JSON values")
		}
		return agent.ToolResult{}, fmt.Errorf("decode delegate_agents arguments: %w", err)
	}
	if len(input.Tasks) < 1 || len(input.Tasks) > t.parallel {
		return agent.ToolResult{}, fmt.Errorf("delegate_agents requires between 1 and %d tasks", t.parallel)
	}

	if t.supervisor != nil {
		requests := make([]StartRequest, 0, len(input.Tasks))
		for _, item := range input.Tasks {
			requests = append(requests, StartRequest{Agent: item.Agent, Task: item.Task, Baseline: item.Baseline})
		}
		tasks, err := t.supervisor.StartBatch(requests)
		if err != nil {
			return agent.ToolResult{}, err
		}

		ok := true
		for i, task := range tasks {
			var err error
			tasks[i], err = t.supervisor.Await(ctx, task.ID)
			if err != nil {
				return agent.ToolResult{}, err
			}
			ok = ok && tasks[i].Status == "completed"
		}
		data, err := json.Marshal(struct {
			OK      bool   `json:"ok"`
			Results []Task `json:"results"`
		}{ok, tasks})
		return agent.ToolResult{Content: string(data)}, err
	}
	t.mu.Lock()
	if len(input.Tasks) > t.remaining {
		t.mu.Unlock()
		return agent.ToolResult{}, fmt.Errorf("delegate_agents run budget exceeded; %d delegation(s) remain", t.remaining)
	}
	assignments := make([]assignment, len(input.Tasks))
	for index, item := range input.Tasks {
		name := strings.TrimSpace(item.Agent)
		task := strings.TrimSpace(item.Task)
		if _, exists := t.registry[name]; !exists {
			t.mu.Unlock()
			return agent.ToolResult{}, fmt.Errorf("unknown specialist %q", name)
		}
		if task == "" || len(task) > maxTaskBytes || strings.ContainsRune(task, 0) || !utf8.ValidString(task) {
			t.mu.Unlock()
			return agent.ToolResult{}, fmt.Errorf("specialist task %d is invalid", index+1)
		}
		t.nextID++
		assignments[index] = assignment{agent: name, task: task, id: fmt.Sprintf("subagent-%03d", t.nextID)}
	}
	t.remaining -= len(assignments)
	t.mu.Unlock()

	t.emit(fmt.Sprintf("starting %d specialist task(s): %s", len(assignments), assignmentNames(assignments)))
	results := make([]delegatedResult, len(assignments))
	records := make([]Record, len(assignments))
	var wait sync.WaitGroup
	for index, item := range assignments {
		wait.Add(1)
		go func(index int, item assignment) {
			defer wait.Done()
			started := t.now().UTC()
			result, err := t.registry[item.agent].Run(ctx, Invocation{ID: item.id, Task: item.task})
			finished := t.now().UTC()
			summary := strings.TrimSpace(result.Summary)
			if len(summary) > maxSummaryBytes {
				summary = summary[:maxSummaryBytes]
				for !utf8.ValidString(summary) {
					summary = summary[:len(summary)-1]
				}
			}
			status := "completed"
			errorText := ""
			if err != nil {
				status = "failed"
				errorText = boundedText(err.Error(), 2048)
			}
			results[index] = delegatedResult{
				ID: item.id, Agent: item.agent, Status: status, Summary: summary, Error: errorText,
				Steps: result.Steps, ArtifactPath: result.ArtifactPath, ArtifactSHA256: result.ArtifactSHA256,
			}
			records[index] = Record{
				ID: item.id, Agent: item.agent, TaskSHA256: digest(item.task), OutputSHA256: digest(summary),
				Status: status, Steps: result.Steps, ArtifactPath: result.ArtifactPath, ArtifactSHA256: result.ArtifactSHA256,
				StartedAt: started, FinishedAt: finished,
			}
		}(index, item)
	}
	wait.Wait()
	completed := 0
	for _, record := range records {
		if record.Status == "completed" {
			completed++
		}
		if t.onRecord != nil {
			t.onRecord(record)
		}
	}
	t.emit(fmt.Sprintf("completed %d of %d specialist task(s)", completed, len(assignments)))
	payload, err := json.Marshal(struct {
		OK      bool              `json:"ok"`
		Results []delegatedResult `json:"results"`
		Notice  string            `json:"notice"`
	}{
		OK: completed == len(assignments), Results: results,
		Notice: "Specialist summaries are bounded, fresh-context results. The manager must verify material claims before using them.",
	})
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("encode specialist results: %w", err)
	}
	return agent.ToolResult{Content: string(payload)}, nil
}

func (t *delegateTool) emit(text string) {
	if t.onEvent != nil {
		t.onEvent(agent.Event{Kind: agent.EventSubagent, At: t.now().UTC(), Text: text})
	}
}

func assignmentNames(assignments []assignment) string {
	names := make([]string, len(assignments))
	for index, item := range assignments {
		names[index] = item.agent
	}
	return strings.Join(names, ", ")
}

func digest(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func boundedText(value string, maximum int) string {
	value = strings.TrimSpace(value)
	if len(value) <= maximum {
		return value
	}
	value = value[:maximum]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
