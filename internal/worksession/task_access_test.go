package worksession

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReviewedAccessEvolutionCannotChangeOtherOptionsOrAccountPins(t *testing.T) {
	_, id, initial := planFixture(t)
	r := initial
	original := append(json.RawMessage(nil), r.Options...)
	// Fixtures use an already planned task; only the reviewed source selection
	// may change in this scope-history entry.
	ids := []string{"docs", "reports"}
	after, err := updatePlanConnections(original, ids)
	if err != nil {
		t.Fatal(err)
	}
	access := &PlanAccessChange{ConnectorIDs: ids, BeforeScope: strings.Repeat("a", 64), AfterScope: strings.Repeat("b", 64), ProcessingDigest: strings.Repeat("c", 64), ReviewID: strings.Repeat("d", 64), CredentialPins: map[string]string{"reports": strings.Repeat("e", 64)}}
	r.Options = after
	r.Plan = r.Plan.Clone()
	r.Plan.InputChanges = []PlanInputChange{{ParentID: id, TurnID: strings.Repeat("f", 64), BeforeOptions: original, AfterSHA256: optionsHash(after), Access: access}}
	if err := validatePlanInputChanges(r); err != nil {
		t.Fatal(err)
	}
	var changed map[string]json.RawMessage
	_ = json.Unmarshal(after, &changed)
	changed["Mode"] = json.RawMessage(`"act"`)
	r.Options, _ = json.Marshal(changed)
	r.Plan.InputChanges[0].AfterSHA256 = optionsHash(r.Options)
	if validatePlanInputChanges(r) == nil {
		t.Fatal("account review widened work mode")
	}
	r.Options = after
	r.Plan.InputChanges[0].AfterSHA256 = optionsHash(after)
	access.CredentialPins["unselected"] = strings.Repeat("e", 64)
	if validatePlanInputChanges(r) == nil {
		t.Fatal("review retained an unselected account pin")
	}
}
