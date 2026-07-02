package schema

import (
	"encoding/json"
	"testing"

	"github.com/gongahkia/paw/internal/envelope"
)

func TestValidateEnvelopeStructsAgainstSchemas(t *testing.T) {
	digest := fullDigest()
	rawDigest, err := json.Marshal(digest)
	if err != nil {
		t.Fatalf("marshal digest: %v", err)
	}
	if err := ValidateContextDigest(rawDigest); err != nil {
		t.Fatalf("validate digest: %v", err)
	}

	plan := envelope.Plan{
		Done:      false,
		Reasoning: "edit the target file",
		NextAction: &envelope.NextAction{
			Kind:        "edit_file",
			Description: "replace the bad return",
			TargetPath:  "main.go",
			Command:     "go test ./...",
		},
	}
	rawPlan, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	if err := ValidatePlan(rawPlan); err != nil {
		t.Fatalf("validate plan: %v", err)
	}
}

func TestValidateContextDigestRejectsBadRelevance(t *testing.T) {
	digest := fullDigest()
	digest.Items[0].Relevance = 200
	raw, err := json.Marshal(digest)
	if err != nil {
		t.Fatalf("marshal digest: %v", err)
	}
	if err := ValidateContextDigest(raw); err == nil {
		t.Fatal("expected relevance validation error")
	}
}

func fullDigest() envelope.ContextDigest {
	return envelope.ContextDigest{
		Summary: "main.go contains the relevant return path",
		Items: []envelope.DigestItem{
			{
				UnitID:    "u001",
				Path:      "main.go",
				Relevance: 90,
				Spans: []envelope.DigestSpan{
					{
						StartLine: 4,
						EndLine:   6,
						Quote:     "return nil",
					},
				},
			},
		},
	}
}
