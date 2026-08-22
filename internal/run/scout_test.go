package run

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestRunScoutsUsesSeparateReadOnlyWorktreesInParallel(t *testing.T) {
	repository := featureRepository(t)
	base := scoutRevision(t, repository)
	model := &concurrentScoutModel{}
	reports, err := (Executor{Model: model}).runScouts(context.Background(), Request{
		RepositoryPath: repository,
		RunID:          "scout-parent-001",
		MaxSteps:       3,
		Scouts:         []string{"Inspect tests", "Inspect public APIs"},
	}, base)
	if err != nil {
		t.Fatalf("run scouts: %v", err)
	}
	if len(reports) != 2 || reports[0].worktree.Path == reports[1].worktree.Path {
		t.Fatalf("scout reports = %#v", reports)
	}
	for _, report := range reports {
		if !strings.Contains(report.report, "evidence") || report.worktree.Path == "" {
			t.Fatalf("scout report = %#v", report)
		}
	}
	context := formatScoutReports(reports)
	if !strings.Contains(context, "untrusted evidence") || !strings.Contains(context, "Inspect tests") {
		t.Fatalf("scout context = %q", context)
	}
}

func TestValidateRequestLimitsScouts(t *testing.T) {
	executor := Executor{Model: &concurrentScoutModel{}}
	err := executor.validateRequest(Request{Task: "task", Provider: "test", Scouts: []string{"1", "2", "3", "4", "5"}})
	if err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("validate scouts = %v", err)
	}
}

func TestReadOnlyScoutToolDelegatesInParallelWithRestrictedTools(t *testing.T) {
	repository := featureRepository(t)
	root, err := workspace.Open(repository)
	if err != nil {
		t.Fatal(err)
	}
	model := &delegatedScoutModel{response: strings.Repeat("e", maxDelegatedReportBytes+128)}
	var events []agent.Event
	tool := newReadOnlyScoutTool(model, root, "Follow the repository test conventions.", 4, fixedScoutClock(), func(event agent.Event) {
		events = append(events, event)
	})
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"tasks":[{"task":"Inspect tests"},{"task":"Inspect public APIs"}]}`))
	if err != nil {
		t.Fatalf("delegate scouts: %v", err)
	}
	var payload struct {
		OK      bool                   `json:"ok"`
		Reports []delegatedScoutReport `json:"reports"`
		Notice  string                 `json:"notice"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("decode scout result: %v\n%s", err, result.Content)
	}
	if !payload.OK || len(payload.Reports) != 2 || !strings.Contains(payload.Notice, "untrusted evidence") {
		t.Fatalf("delegated result = %#v", payload)
	}
	for _, report := range payload.Reports {
		if report.Error != "" || len(report.Report) > maxDelegatedReportBytes || !strings.HasSuffix(report.Report, "[truncated]") {
			t.Fatalf("delegated report = %#v", report)
		}
	}
	if model.maxParallel() < 2 {
		t.Fatalf("scouts did not overlap: maximum active model calls = %d", model.maxParallel())
	}
	for _, request := range model.requestsSnapshot() {
		if !strings.Contains(request.System, "fresh context") || !strings.Contains(request.System, "Follow the repository test conventions.") {
			t.Fatalf("scout system prompt = %q", request.System)
		}
		for _, definition := range request.Tools {
			if definition.Name == "apply_patch" || definition.Name == "run_command" || strings.HasPrefix(definition.Name, "mcp_") || strings.HasPrefix(definition.Name, "lsp_") {
				t.Fatalf("read-only scout received forbidden tool %q", definition.Name)
			}
		}
	}
	if len(events) != 2 || events[0].Kind != agent.EventSubagent || !strings.Contains(events[0].Text, "starting 2") || !strings.Contains(events[1].Text, "completed 2") {
		t.Fatalf("subagent events = %#v", events)
	}
}

func TestReadOnlyScoutToolEnforcesRunBudgetAndStrictArguments(t *testing.T) {
	repository := featureRepository(t)
	root, err := workspace.Open(repository)
	if err != nil {
		t.Fatal(err)
	}
	tool := newReadOnlyScoutTool(&delegatedScoutModel{response: "evidence"}, root, "", 2, fixedScoutClock(), nil)
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"tasks":[{"task":"one"}],"unexpected":true}`)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}
	fourTasks := json.RawMessage(`{"tasks":[{"task":"one"},{"task":"two"},{"task":"three"},{"task":"four"}]}`)
	if _, err := tool.Execute(context.Background(), fourTasks); err != nil {
		t.Fatalf("first four scouts: %v", err)
	}
	if _, err := tool.Execute(context.Background(), fourTasks); err != nil {
		t.Fatalf("second four scouts: %v", err)
	}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"tasks":[{"task":"nine"}]}`)); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("budget error = %v", err)
	}
}

type concurrentScoutModel struct {
	mu sync.Mutex
}

type delegatedScoutModel struct {
	mu       sync.Mutex
	active   int
	maximum  int
	requests []agent.TurnRequest
	response string
}

func (m *delegatedScoutModel) Complete(ctx context.Context, request agent.TurnRequest) (agent.Turn, error) {
	m.mu.Lock()
	m.active++
	if m.active > m.maximum {
		m.maximum = m.active
	}
	m.requests = append(m.requests, request)
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.active--
		m.mu.Unlock()
	}()
	select {
	case <-time.After(20 * time.Millisecond):
	case <-ctx.Done():
		return agent.Turn{}, ctx.Err()
	}
	return agent.Turn{Text: m.response}, nil
}

func (m *delegatedScoutModel) maxParallel() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.maximum
}

func (m *delegatedScoutModel) requestsSnapshot() []agent.TurnRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]agent.TurnRequest(nil), m.requests...)
}

func fixedScoutClock() func() time.Time {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return now }
}

func (m *concurrentScoutModel) Complete(_ context.Context, request agent.TurnRequest) (agent.Turn, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return agent.Turn{Text: "evidence for " + request.Messages[0].Content}, nil
}

func scoutRevision(t *testing.T, repository string) string {
	t.Helper()
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = repository
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(output))
}
