package worksession

import (
	"encoding/json"
	"strings"
	"testing"
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
