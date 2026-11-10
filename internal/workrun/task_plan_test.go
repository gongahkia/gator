package workrun

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/worksession"
)

type taskPlanModel func(context.Context, agent.TurnRequest) (agent.Turn, error)

func (f taskPlanModel) Complete(ctx context.Context, r agent.TurnRequest) (agent.Turn, error) {
	return f(ctx, r)
}
func planningRequest() worksession.Request {
	return worksession.Request{ID: "task", Text: "Implement a parser with tests", Plan: &worksession.TaskPlan{Phase: "preparing", Sources: []string{"docs"}, Budget: worksession.PlanBudget{MaxSteps: 24, CodeMaxSteps: 16}}}
}

const validPlan = `{"outcome":"A tested parser","steps":[{"id":"step-1","text":"Inspect and implement"},{"id":"step-2","text":"Run checks"}],"sources":["docs"],"effects":["read","code","checks"]}`

func TestTaskPlanPreparationIsOneToolFreeBoundedModelCall(t *testing.T) {
	calls := 0
	model := taskPlanModel(func(ctx context.Context, r agent.TurnRequest) (agent.Turn, error) {
		calls++
		if len(r.Tools) > 0 || r.Computer != nil {
			t.Fatal("planning received execution tools")
		}
		raw, _ := json.Marshal(r)
		for _, secret := range []string{"private-argv", "decision-token", "provider-state"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("private replay entered proposal", secret)
			}
		}
		return agent.Turn{Text: validPlan}, nil
	})
	history := []agent.Message{{Role: agent.RoleUser, Content: "Earlier question"}, {Role: agent.RoleAgent, Content: "Public reply", ProviderData: json.RawMessage(`{"secret":"provider-state"}`)}, {Role: agent.RoleAgent, ToolCalls: []agent.ToolCall{{ID: "decision-token", Name: "run_command", Arguments: json.RawMessage(`{"argv":["private-argv"]}`)}}}, {Role: agent.RoleTool, ToolCallID: "decision-token", Content: "private-argv"}}
	plan, err := PrepareTaskPlan(context.Background(), model, planningRequest(), history)
	if err != nil || calls != 1 || plan.Outcome != "A tested parser" {
		t.Fatal("proposal failed", err)
	}
}
func TestTaskPlanPreparationRejectsToolsMalformedAndInventedScopeWithoutRetry(t *testing.T) {
	for _, turn := range []agent.Turn{{Text: validPlan, ToolCalls: []agent.ToolCall{{ID: "call", Name: "run_command", Arguments: json.RawMessage(`{}`)}}}, {Text: strings.Repeat("x", 32769)}, {Text: strings.Replace(validPlan, `"docs"`, `"unselected"`, 1)}, {Text: strings.Replace(validPlan, `"effects":`, `"approved":true,"effects":`, 1)}, {Text: validPlan + ` {}`}, {Text: strings.Replace(validPlan, "Run checks", `Run\u0000checks`, 1)}} {
		calls := 0
		model := taskPlanModel(func(context.Context, agent.TurnRequest) (agent.Turn, error) { calls++; return turn, nil })
		if _, err := PrepareTaskPlan(context.Background(), model, planningRequest(), nil); err == nil || calls != 1 {
			t.Fatal("invalid proposal accepted or automatically repaired", calls, err)
		}
	}
}
func TestTaskPlanPreparationHonorsCancellationAndDoesNotReplanApprovedWork(t *testing.T) {
	called := false
	model := taskPlanModel(func(ctx context.Context, r agent.TurnRequest) (agent.Turn, error) {
		called = true
		<-ctx.Done()
		return agent.Turn{}, ctx.Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := PrepareTaskPlan(ctx, model, planningRequest(), nil); !errors.Is(err, context.DeadlineExceeded) || !called {
		t.Fatal("model cancellation lost", err)
	}
	called = false
	request := planningRequest()
	request.Plan.Phase = "approved"
	if _, err := PrepareTaskPlan(context.Background(), model, request, nil); err == nil || called {
		t.Fatal("approved work silently replanned")
	}
}

func TestTaskPlanRevisionUsesOriginalBudgetAndPublicProposalWithoutControlAuthority(t *testing.T) {
	request := planningRequest()
	request.State, request.Plan.Phase = "queued", "revising"
	var previous worksession.PlanContent
	if json.Unmarshal([]byte(validPlan), &previous) != nil {
		t.Fatal("invalid fixture")
	}
	request.Plan.Content = &previous
	request.Plan.Edit = &worksession.PlanEdit{PlanEditReference: worksession.PlanEditReference{ParentID: "host-private-parent", TurnID: "host-private-control", Instruction: "Keep the parser small"}, State: "preparing"}
	calls := 0
	model := taskPlanModel(func(_ context.Context, turn agent.TurnRequest) (agent.Turn, error) {
		calls++
		if len(turn.Tools) != 0 || turn.Computer != nil {
			t.Fatal("revision received execution tools")
		}
		raw, _ := json.Marshal(turn)
		if strings.Contains(string(raw), "host-private-") {
			t.Fatal("control references entered the model context")
		}
		var meta struct {
			Task       string                   `json:"task"`
			Correction string                   `json:"correction"`
			Previous   *worksession.PlanContent `json:"previous_plan"`
			Budget     worksession.PlanBudget   `json:"work_limits"`
			Sources    []string                 `json:"selected_source_ids"`
		}
		if json.Unmarshal([]byte(turn.Messages[len(turn.Messages)-1].Content), &meta) != nil || meta.Task != request.Text || meta.Correction != request.Plan.Edit.Instruction || meta.Previous == nil || meta.Previous.Outcome != previous.Outcome || meta.Budget != request.Plan.Budget || len(meta.Sources) != 1 || meta.Sources[0] != "docs" {
			t.Fatal("original proposal or frozen scope lost")
		}
		return agent.Turn{Text: validPlan}, nil
	})
	if _, err := ReviseTaskPlan(context.Background(), model, request, nil); err != nil || calls != 1 {
		t.Fatal("tool-free correction failed", err)
	}
	request.Plan.Edit.State = "applied"
	if _, err := ReviseTaskPlan(context.Background(), model, request, nil); err == nil || calls != 1 {
		t.Fatal("completed edit was proposed again")
	}
	text := taskPlanPrompt(request.Plan)
	if !strings.Contains(text, "Keep the parser small") || !strings.Contains(text, "original task text is retained") {
		t.Fatal("execution lost the actual user correction")
	}
	request.Plan.Edit.State = "failed"
	if strings.Contains(taskPlanPrompt(request.Plan), "Keep the parser small") {
		t.Fatal("an unapplied correction changed execution intent")
	}
}
