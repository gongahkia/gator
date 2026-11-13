package worksession

import (
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"strings"
	"testing"
)

func budgetAttempt() PlanAttempt {
	return PlanAttempt{RunID: "original-run", RevisionID: "original-revision", ManifestSHA256: strings.Repeat("a", 64), Execution: artifact.ExecutionAccounting{Limits: agent.Limits{ModelRequests: 10, Tokens: 100, WallSeconds: 60}, MaxSteps: 8, CodeMaxSteps: 6, Steps: 2}, Usage: agent.Usage{ModelRequests: 3, InputTokens: 20, OutputTokens: 5}, CodeSteps: 1, WallSeconds: 7}
}
func TestRemainingPlanExecutionBudgetSurvivesPersistenceWithoutDefaultReset(t *testing.T) {
	p := TaskPlan{Budget: PlanBudget{MaxSteps: 8, CodeMaxSteps: 6}, Attempts: []PlanAttempt{budgetAttempt()}}
	budget, limits, err := p.RemainingExecutionBudget()
	if err != nil || budget.MaxSteps != 6 || budget.CodeMaxSteps != 5 || limits.ModelRequests != 7 || limits.Tokens != 75 || limits.WallSeconds != 53 {
		t.Fatal("remaining budget did not subtract original usage", budget, limits, err)
	}
	for _, kind := range []string{"steps", "code", "model", "tokens", "wall", "duplicate", "overflow"} {
		t.Run(kind, func(t *testing.T) {
			copy := p.Clone()
			a := &copy.Attempts[0]
			switch kind {
			case "steps":
				a.Execution.Steps = 8
			case "code":
				a.CodeSteps = 7
			case "model":
				a.Usage.ModelRequests = 10
			case "tokens":
				a.Usage.InputTokens = 100
			case "wall":
				a.WallSeconds = 60
			case "duplicate":
				copy.Attempts = append(copy.Attempts, *a)
			case "overflow":
				a.Usage.InputTokens = 9223372036854775807
				a.Usage.OutputTokens = 1
			}
			if _, _, err := copy.RemainingExecutionBudget(); err == nil {
				t.Fatal("exhausted/invalid budget became fresh defaults")
			}
		})
	}
}
