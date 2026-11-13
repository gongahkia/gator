package worksession

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestInputEvolutionPreservesAllOtherAuthorityAndOriginalBytes(t *testing.T) {
	oldID := strings.Repeat("a", 32) + "/original.txt"
	newID := strings.Repeat("b", 32) + "/revised.txt"
	original := json.RawMessage(`{"PlanFirst":true,"Uploads":["` + oldID + `"],"ConnectorIDs":["docs"],"MaxSteps":24,"Code":{"Sandbox":"strict","MaxSteps":16},"ModelProvider":"original"}`)
	after, added, _, err := mergePlanInputs(original, []string{newID}, nil)
	if err != nil || len(added) != 1 {
		t.Fatal("input evolution unavailable", err)
	}
	var beforeFields, afterFields map[string]json.RawMessage
	_ = json.Unmarshal(original, &beforeFields)
	_ = json.Unmarshal(after, &afterFields)
	for key, value := range beforeFields {
		if key != "Uploads" && string(value) != string(afterFields[key]) {
			t.Fatal("input addition changed original authority", key)
		}
	}
	change := PlanInputChange{ParentID: "chat", TurnID: strings.Repeat("c", 64), BeforeOptions: original, AddedUploads: added, AfterSHA256: optionsHash(after)}
	r := Request{Options: after, Plan: &TaskPlan{InputChanges: []PlanInputChange{change}}}
	if err := validatePlanInputChanges(r); err != nil {
		t.Fatal(err)
	}
	changed := append(json.RawMessage(nil), after...)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(changed, &fields)
	fields["ModelProvider"] = json.RawMessage(`"other"`)
	changed, _ = json.Marshal(fields)
	r.Options = changed
	if validatePlanInputChanges(r) == nil {
		t.Fatal("input transition widened processing destination")
	}
	r.Options = after
	r.Plan.InputChanges[0].AddedUploads[0] = oldID
	if validatePlanInputChanges(r) == nil {
		t.Fatal("input identity changed without invalidating history")
	}
	if !strings.Contains(string(original), oldID) || strings.Contains(string(original), newID) {
		t.Fatal("original options were rewritten")
	}
}
func TestInputEvolutionRejectsUnsafeOrOversizedSelections(t *testing.T) {
	for _, variant := range []string{"absolute", "traversal", "invalid upload", "folded alias", "authority alias", "too many"} {
		t.Run(variant, func(t *testing.T) {
			original := json.RawMessage(`{"PlanFirst":true}`)
			uploads, paths := []string{}, []string{}
			switch variant {
			case "absolute":
				paths = []string{"/private/file"}
			case "traversal":
				paths = []string{"../file"}
			case "invalid upload":
				uploads = []string{"foreign/file"}
			case "folded alias":
				original = json.RawMessage(`{"Uploads":[],"uploads":[]}`)
			case "authority alias":
				original = json.RawMessage(`{"Mode":"inspect","mode":"act"}`)
			case "too many":
				for i := 0; i < 13; i++ {
					uploads = append(uploads, strings.Repeat(string(rune('a'+i%6)), 32)+"/file"+string(rune('a'+i))+".txt")
				}
			}
			if _, _, _, err := mergePlanInputs(original, uploads, paths); err == nil {
				t.Fatal("unsafe selection accepted")
			}
		})
	}
}

func TestRemovingPlanInputsRetainsOriginalAdmissionAndRequiresFreshApproval(t *testing.T) {
	s, id, _ := requestTestStore(t)
	oldID := strings.Repeat("a", 32) + "/old.txt"
	keptID := strings.Repeat("b", 32) + "/keep.txt"
	raw := json.RawMessage(`{"PlanFirst":true,"Uploads":["` + oldID + `","` + keptID + `"],"ConnectorIDs":["docs"],"MaxSteps":24,"Code":{"MaxSteps":16}}`)
	_, r, err := s.AdmitPlanRequest(id, "Compare files", raw, "original-admission", []string{"docs"}, PlanBudget{MaxSteps: 24, CodeMaxSteps: 16}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviseTaskPlan(id, r.ID, 0, "", proposedPlan()); err != nil {
		t.Fatal(err)
	}
	original := retainedPlan(t, s, id, r.ID)
	if _, _, err := s.ApproveTaskPlan(id, r.ID, original.Plan.Revision, original.Plan.Digest); err != nil {
		t.Fatal(err)
	}
	j, err := s.RemoveTaskPlanInputs(id, r.ID, original.Plan.Revision, original.Plan.Digest, "remove-one", []PlanInputRef{{Kind: "upload", ID: oldID}})
	if err != nil {
		t.Fatal(err)
	}
	changed := j.Requests[0]
	var selected struct{ Uploads []string }
	_ = json.Unmarshal(changed.Options, &selected)
	if len(selected.Uploads) != 1 || selected.Uploads[0] != keptID || changed.ID != original.ID || changed.Plan.Approval != nil || changed.Plan.Phase != "review" || optionsHash(changed.OriginalAdmissionOptions()) != optionsHash(raw) {
		t.Fatal("input removal lost identity, originals, or fresh review")
	}
	if _, _, err := s.ApproveTaskPlan(id, r.ID, original.Plan.Revision, original.Plan.Digest); err == nil {
		t.Fatal("old approval executed changed inputs")
	}
	if _, _, err := s.AdmitPlanRequest(id, r.Text, raw, "original-admission", []string{"docs"}, r.Plan.Budget, time.Now()); err != nil {
		t.Fatal("original admission identity changed", err)
	}
	if _, err := s.RemoveTaskPlanInputs(id, r.ID, changed.Plan.Revision, changed.Plan.Digest, "wrong-input", []PlanInputRef{{Kind: "upload", ID: strings.Repeat("c", 32) + "/foreign.txt"}}); err == nil {
		t.Fatal("unselected input acquired authority")
	}
	if _, _, err := s.ApproveTaskPlan(id, r.ID, changed.Plan.Revision, changed.Plan.Digest); err != nil {
		t.Fatal("fresh exact narrowed review rejected", err)
	}
}
