package worksession

import (
	"reflect"
	"testing"
)

func TestWorkEffortPreferencesPreserveConnectionsAndRejectStaleOrInvalidWrites(t *testing.T) {
	s, id, _ := requestTestStore(t)
	prior, err := s.LoadPreferences(id)
	if err != nil {
		t.Fatal(err)
	}
	connections := []string{"reports"}
	effort := "high"
	next := prior
	next.ConnectorIDs = &connections
	next.WorkEffort = &effort
	if err := s.SavePreferences(id, next, &prior); err != nil {
		t.Fatal(err)
	}
	current, err := s.LoadPreferences(id)
	if err != nil || !reflect.DeepEqual(current, next) {
		t.Fatal("lost canonical preferences", err, current)
	}
	if err := s.SavePreferences(id, prior, &prior); err == nil {
		t.Fatal("stale source write removed effort")
	}
	for _, value := range []string{"", "HIGH", "maximum"} {
		bad := current
		bad.WorkEffort = &value
		if err := s.SavePreferences(id, bad, &current); err == nil {
			t.Fatal("invalid effort persisted", value)
		}
	}
	after, _ := s.LoadPreferences(id)
	if !reflect.DeepEqual(after, current) {
		t.Fatal("rejected writes changed settings")
	}
	for effort, want := range map[string]PlanBudget{"low": {MaxSteps: 12, CodeMaxSteps: 8}, "standard": {MaxSteps: 24, CodeMaxSteps: 16}, "high": {MaxSteps: 48, CodeMaxSteps: 32}} {
		actual, err := WorkEffortBudget(effort)
		if err != nil || actual != want {
			t.Fatal("preset differs from established budgets", effort, actual, err)
		}
	}
}
