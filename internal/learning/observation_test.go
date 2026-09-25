package learning

import (
	"strings"
	"testing"
)

func TestExplicitCorrectionCreatesObservationAndInactiveCandidate(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	scope := Scope{Kind: Project, Value: project}
	observation, err := store.RecordObservation(ObservationInput{
		Signal: UserCorrected, WorkID: "work-correction", Scope: &scope,
		Summary: "User corrected the package manager.", EvidenceRefs: []string{"gator/work-history/work-correction.json"},
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := store.Propose(Proposal{
		Observation: observation, Type: Preference, Key: "package-manager", Content: "Use pnpm.", Scope: scope,
	})
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Status != Candidate || candidate.Origin != Inferred || candidate.Confidence != 100 {
		t.Fatalf("candidate = %#v", candidate)
	}
	if _, err := store.LinkObservation(observation.ID, candidate.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadObservation(observation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.WorkID != "work-correction" || len(loaded.LearningIDs) != 1 || loaded.LearningIDs[0] != candidate.ID {
		t.Fatalf("observation linkage = %#v", loaded)
	}
	projected, err := store.Projection(Context{Project: project})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected) != 0 {
		t.Fatalf("candidate entered Work context: %#v", projected)
	}
}

func TestCandidateApprovalRejectionAndRepeatedEvidenceRemainInspectable(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{Kind: Global}
	first, err := store.RecordObservation(ObservationInput{Signal: UserCorrected, WorkID: "work-one", Scope: &scope, Summary: "Use a focused test."})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := store.Propose(Proposal{Observation: first, Type: FailurePrevention, Key: "focused-test", Content: "Run the focused test before the broad suite.", Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.RecordObservation(ObservationInput{Signal: UserCorrected, WorkID: "work-two", Scope: &scope, Summary: "Repeated focused-test correction."})
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := store.Propose(Proposal{Observation: second, Type: FailurePrevention, Key: "focused-test", Content: "Run the focused test before the broad suite.", Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	if repeated.ID != candidate.ID || len(repeated.Provenance.WorkIDs) != 2 || len(repeated.Provenance.EvidenceRefs) != 2 {
		t.Fatalf("repeated evidence created uncontrolled candidates: %#v", repeated)
	}
	if _, err := store.Enable(candidate.ID); err != nil {
		t.Fatal(err)
	}
	active, err := store.Projection(Context{})
	if err != nil || len(active) != 1 || active[0].ID != candidate.ID {
		t.Fatalf("approved candidate = %#v, %v", active, err)
	}
	if _, err := store.Disable(candidate.ID); err != nil {
		t.Fatal(err)
	}
	if projected, err := store.Projection(Context{}); err != nil || len(projected) != 0 {
		t.Fatalf("disabled candidate still affects Work: %#v, %v", projected, err)
	}
	rejected, err := store.Create(Create{ID: "learning-rejected", Type: Preference, Key: "format", Content: "Use PDF.", Scope: scope, Origin: Inferred, Confidence: 80})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reject(rejected.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enable(rejected.ID); err == nil {
		t.Fatal("rejected candidate was enabled")
	}
}

func TestFailureAndUnknownSignalsRemainDistinctAndNeverCreateRetryRule(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []ObservationInput{
		{ID: "observation-work-failed", Signal: WorkFailed, WorkID: "work-failed", Summary: "Work execution failed.", EvidenceRefs: []string{"gator/work-history/work-failed.json"}},
		{ID: "observation-delivery-failed", Signal: DeliveryFailed, WorkID: "work-failed", Summary: "Local delivery failed.", EvidenceRefs: []string{"gator/delivery/work-failed/delivery-one.json"}},
		{ID: "observation-delivery-unknown", Signal: DeliveryUnknown, WorkID: "work-failed", Summary: "Delivery outcome is unknown.", EvidenceRefs: []string{"gator/delivery/work-failed/delivery-two.json"}},
		{ID: "observation-external-unknown", Signal: ExternalOutcomeUnknown, WorkID: "work-failed", Summary: "External action outcome is unknown.", EvidenceRefs: []string{"gator/work/work-failed/manifest.json"}},
	} {
		if _, err := store.RecordObservation(input); err != nil {
			t.Fatal(err)
		}
	}
	observations, err := store.ListObservations("work-failed")
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 4 {
		t.Fatalf("observations = %#v", observations)
	}
	for _, observation := range observations {
		if strings.Contains(string(observation.Signal), "retry") || len(observation.LearningIDs) != 0 {
			t.Fatalf("unsafe retry learning was derived: %#v", observation)
		}
	}
	records, err := store.List()
	if err != nil || len(records) != 0 {
		t.Fatalf("operational failure created a learning: %#v, %v", records, err)
	}
}
