package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/suggestion"
)

func TestMainChatPlanChangesRequireInspectionAndRespectGoalIsolation(t *testing.T) {
	root := t.TempDir()
	goals, _ := goal.Open(root)
	queue, _ := suggestion.Open(root)
	first, _ := goals.Add("Workshop", "", "")
	other, _ := goals.Add("Private second goal", "", "")
	plan, _ := goals.InitializePlan(first.ID, "Prepare", []string{"Draft"})
	plans := &LocalPlans{Store: goals, AllowGlobal: true}
	propose := SuggestChange{Store: queue, Plans: plans, WorkID: "test"}
	raw, _ := json.Marshal(map[string]any{"kind": "plan", "goal_id": first.ID, "plan_change": goal.PlanChange{Kind: "update_step", ExpectedRevision: plan.Revision, StepID: plan.Steps[0].ID, Status: goal.StepDoing}})
	if _, err := propose.Execute(context.Background(), raw); err == nil {
		t.Fatal("uninspected plan change accepted")
	}
	read, _ := json.Marshal(map[string]string{"query": "Workshop"})
	if _, err := plans.Tools()[0].Execute(context.Background(), read); err != nil {
		t.Fatal(err)
	}
	if _, err := propose.Execute(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	current, _ := goals.LoadPlan(first.ID)
	if current.Revision != plan.Revision {
		t.Fatal("proposed change applied without review")
	}
	plans.GoalID, propose.GoalID = other.ID, other.ID
	read, _ = json.Marshal(map[string]string{"goal_id": first.ID})
	if _, err := plans.Tools()[0].Execute(context.Background(), read); err == nil {
		t.Fatal("cross-goal plan read allowed")
	}
	if _, err := propose.Execute(context.Background(), raw); err == nil {
		t.Fatal("cross-goal proposal allowed")
	}
	result, err := plans.Tools()[1].Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil || strings.Contains(result.Content, "Workshop") {
		t.Fatalf("scoped agenda leaked other goal: %v", err)
	}
}
