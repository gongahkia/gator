package worksession

import (
	"encoding/json"
	"testing"
	"time"
)

func TestOrdinaryExecutionBoundaryRejectsLiveEditsButPreservesExplicitRetry(t *testing.T) {
	s, id, _ := requestTestStore(t)
	r, err := NewRequest("Original task", json.RawMessage(`{"MaxSteps":24}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := s.ExecutionBoundary(id, r)
	if err != nil {
		t.Fatal(err)
	}
	r.ExecutionBoundary = &boundary
	r.State = "running"
	if err := validateRequest(r); err != nil {
		t.Fatal(err)
	}
	r.Text = "Explicitly edited retry"
	if validateRequest(r) == nil {
		t.Fatal("live task changed without a scoped transition")
	}
	r.State = "interrupted"
	if err := validateRequest(r); err != nil {
		t.Fatal("an explicit interrupted edit was removed", err)
	}
	fresh, err := s.ExecutionBoundary(id, r)
	if err != nil {
		t.Fatal(err)
	}
	r.ExecutionBoundary = &fresh
	r.State = "running"
	if err := validateRequest(r); err != nil {
		t.Fatal("retry could not capture its actual new boundary", err)
	}
}
func TestReviewPromotionChangesOnlyReviewIntent(t *testing.T) {
	original := json.RawMessage(`{"Mode":"inspect","ConnectorIDs":["account-original"],"MaxSteps":24,"Uploads":[]}`)
	gated, err := promotePlanOptions(original)
	if err != nil {
		t.Fatal(err)
	}
	var a, b map[string]json.RawMessage
	_ = json.Unmarshal(original, &a)
	_ = json.Unmarshal(gated, &b)
	for key, value := range a {
		if string(b[key]) != string(value) {
			t.Fatal("promotion changed captured authority", key)
		}
	}
	if string(b["PlanFirst"]) != "true" {
		t.Fatal("promotion did not require review")
	}
	if _, err := promotePlanOptions(gated); err == nil {
		t.Fatal("already-reviewed task was promoted twice")
	}
}
