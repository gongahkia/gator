package workrun

import (
	"testing"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/workhistory"
)

func TestDecisionReflectsEffectiveAuthority(t *testing.T) {
	request := Request{Mode: action.Draft, GoalID: "goal-1", ConnectorIDs: []string{"calendar"}, Contract: artifact.Contract{Artifacts: []artifact.Requirement{{Path: "brief.md"}}}, DecisionOrigin: "model", DecisionReason: "The request asks for a brief."}
	decision, err := resolveDecision(request)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Mode != action.Draft || decision.Origin != "model" || decision.GoalID != "goal-1" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if len(decision.Artifacts) != 1 || decision.Artifacts[0] != "brief.md" {
		t.Fatalf("artifact route: %#v", decision.Artifacts)
	}
	if len(decision.Connectors) != 1 || decision.Connectors[0] != "calendar" {
		t.Fatalf("connector route: %#v", decision.Connectors)
	}
	request.Decision = &workhistory.Decision{Mode: action.Act, GoalID: "goal-1", Artifacts: []string{"brief.md"}, Connectors: []string{"calendar"}}
	if _, err := resolveDecision(request); err == nil {
		t.Fatal("model decision widened authority")
	}
}
