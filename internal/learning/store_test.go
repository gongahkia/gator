package learning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserAuthoredLearningRoundTripsAndProjects(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	created, err := store.Create(Create{
		ID: "learning-user", Type: Preference, Key: "package-manager", Content: "Use pnpm for package commands.",
		Scope: Scope{Kind: Project, Value: project}, Origin: UserAuthored,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != Active || created.Origin != UserAuthored || created.Confidence != 0 {
		t.Fatalf("created learning = %#v", created)
	}
	loaded, err := store.Load(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Content != created.Content || loaded.Scope != created.Scope {
		t.Fatalf("round trip = %#v", loaded)
	}
	projected, err := store.Projection(Context{Project: project})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected) != 1 || projected[0].ID != created.ID || !strings.Contains(RenderProjection(projected), "Use pnpm") {
		t.Fatalf("projection = %#v", projected)
	}
}

func TestCandidateIsSeparateAndNeverProjectsUntilUserEnablesIt(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := store.Create(Create{
		ID: "learning-candidate", Type: Procedure, Key: "test-command", Content: "Run the focused test first.",
		Scope: Scope{Kind: Global}, Origin: Inferred, Confidence: 72,
		Provenance: Provenance{WorkIDs: []string{"work-42"}, EvidenceRefs: []string{"work-history/work-42.json"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Status != Candidate {
		t.Fatalf("candidate status = %q", candidate.Status)
	}
	projected, err := store.Projection(Context{})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected) != 0 {
		t.Fatalf("candidate leaked into projection: %#v", projected)
	}
	active, err := store.Enable(candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if active.Status != Active || active.Provenance.UserConfirmedAt.IsZero() {
		t.Fatalf("enabled candidate = %#v", active)
	}
	projected, err = store.Projection(Context{})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected) != 1 || projected[0].ID != candidate.ID {
		t.Fatalf("enabled projection = %#v", projected)
	}
}

func TestDisableStopsSelectionButRetainsProvenance(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Create(Create{
		ID: "learning-disable", Type: FailurePrevention, Key: "verify", Content: "Run the repository check.", Scope: Scope{Kind: Global},
		Origin: UserAuthored, Provenance: Provenance{WorkIDs: []string{"work-99"}, EvidenceRefs: []string{"work-history/work-99.json"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Disable(record.ID); err != nil {
		t.Fatal(err)
	}
	projected, err := store.Projection(Context{})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected) != 0 {
		t.Fatalf("disabled learning projected: %#v", projected)
	}
	stored, err := store.Load(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != Disabled || len(stored.Provenance.WorkIDs) != 1 || stored.Provenance.WorkIDs[0] != "work-99" {
		t.Fatalf("disabled learning lost provenance: %#v", stored)
	}
}

func TestProjectScopeCannotAffectAnotherProject(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projectA, projectB := t.TempDir(), t.TempDir()
	if _, err := store.Create(Create{
		ID: "learning-project-a", Type: Preference, Key: "formatter", Content: "Use the project formatter.", Scope: Scope{Kind: Project, Value: projectA}, Origin: UserAuthored,
	}); err != nil {
		t.Fatal(err)
	}
	for _, context := range []Context{{Project: projectB}, {}} {
		projected, err := store.Projection(context)
		if err != nil {
			t.Fatal(err)
		}
		if len(projected) != 0 {
			t.Fatalf("project-scoped learning leaked for %#v: %#v", context, projected)
		}
	}
}

func TestExplicitUserRuleOutranksConflictingInferredRule(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	inferred, err := store.Create(Create{
		ID: "learning-inferred", Type: Preference, Key: "package-manager", Content: "Use npm.", Scope: Scope{Kind: Project, Value: project}, Origin: Inferred, Confidence: 90,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enable(inferred.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(Create{
		ID: "learning-explicit", Type: Preference, Key: "package-manager", Content: "Use pnpm.", Scope: Scope{Kind: Global}, Origin: UserAuthored,
	}); err != nil {
		t.Fatal(err)
	}
	projected, err := store.Projection(Context{Project: project})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected) != 1 || projected[0].ID != "learning-explicit" {
		t.Fatalf("explicit rule did not win: %#v", projected)
	}
}

func TestDifferentKeysRemainDistinctWithoutSemanticConflictResolution(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	for _, input := range []Create{
		{ID: "learning-global-format", Type: Preference, Key: "report-default", Content: "Use Markdown for reports.", Scope: Scope{Kind: Global}, Origin: UserAuthored},
		{ID: "learning-project-format", Type: Preference, Key: "report-exception", Content: "Use HTML for reports.", Scope: Scope{Kind: Project, Value: project}, Origin: UserAuthored},
	} {
		if _, err := store.Create(input); err != nil {
			t.Fatal(err)
		}
	}
	projected, err := store.Projection(Context{Project: project})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected) != 2 {
		t.Fatalf("different keys unexpectedly received semantic conflict resolution: %#v", projected)
	}
	projection := RenderProjection(projected)
	if !strings.Contains(projection, "Use Markdown for reports.") || !strings.Contains(projection, "Use HTML for reports.") {
		t.Fatalf("conflicting different-key records were not both visible to Work: %q", projection)
	}
}

func TestRejectAndRemoveKeepRecordsInspectable(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := store.Create(Create{ID: "learning-reject", Type: Procedure, Key: "build", Content: "Run build.", Scope: Scope{Kind: Global}, Origin: Inferred, Confidence: 50})
	if err != nil {
		t.Fatal(err)
	}
	if rejected, err := store.Reject(candidate.ID); err != nil || rejected.Status != Rejected {
		t.Fatalf("reject = %#v, %v", rejected, err)
	}
	active, err := store.Create(Create{ID: "learning-remove", Type: Preference, Key: "output", Content: "Use Markdown.", Scope: Scope{Kind: Global}, Origin: UserAuthored})
	if err != nil {
		t.Fatal(err)
	}
	if removed, err := store.Remove(active.ID); err != nil || removed.Status != Disabled {
		t.Fatalf("remove = %#v, %v", removed, err)
	}
	records, err := store.List()
	if err != nil || len(records) != 2 {
		t.Fatalf("records = %#v, %v", records, err)
	}
}

func TestInvalidDirectEditFailsClosedAndDoesNotProject(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Root(), "learning-invalid.json"), []byte(`{"version":1,"id":"learning-invalid","content":"unvalidated"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Projection(Context{}); err == nil {
		t.Fatal("invalid direct edit was accepted for projection")
	}
}

func TestProvenancePersistsReferencesWithoutPayloadCopies(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(Create{
		ID: "learning-provenance", Type: EnvironmentFact, Key: "toolchain", Content: "The repository uses Go.", Scope: Scope{Kind: Global}, Origin: UserAuthored,
		Provenance: Provenance{WorkIDs: []string{"work-1"}, EvidenceRefs: []string{"work-history/work-1.json"}},
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(store.Root(), "learning-provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "work-history/work-1.json") || strings.Contains(string(data), "raw transaction payload") {
		t.Fatalf("stored provenance = %s", data)
	}
}
