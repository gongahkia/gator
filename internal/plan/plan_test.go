package plan

import (
	"context"
	"strings"
	"testing"

	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/llm"
	"github.com/gongahkia/paw/internal/llm/faketest"
)

func TestPlanDoneShortCircuits(t *testing.T) {
	env := planEnvelope()
	env.Done = true
	got, err := New(nil).Run(context.Background(), env)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if got.Plan == nil || !got.Plan.Done {
		t.Fatalf("plan = %#v", got.Plan)
	}
}

func TestPlanRequiresNextActionWhenNotDone(t *testing.T) {
	srv := faketest.NewServer()
	defer srv.Close()
	srv.RespondOpenAI("", `{"done":false,"reasoning":"need action"}`)

	_, err := New(llm.NewOpenAIClient(srv.URL, "key", "brain")).Run(context.Background(), planEnvelope())
	if err == nil || !strings.Contains(err.Error(), "next_action") {
		t.Fatalf("expected next_action error, got %v", err)
	}
}

func TestPlanSendsDigestNotRawContext(t *testing.T) {
	srv := faketest.NewServer()
	defer srv.Close()
	srv.RespondOpenAI("", `{"done":false,"reasoning":"edit","next_action":{"kind":"edit_file","description":"fix","target_path":"calc.go"}}`)

	got, err := New(llm.NewOpenAIClient(srv.URL, "key", "brain")).Run(context.Background(), planEnvelope())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if got.Budget.BrainInputTokens != 11 || got.Budget.BrainOutputTokens != 7 {
		t.Fatalf("budget = %#v", got.Budget)
	}
	body := srv.LastRequest().Body
	if !strings.Contains(body, "digest summary") {
		t.Fatalf("request missing digest summary: %s", body)
	}
	if strings.Contains(body, "SECRET_RAW") {
		t.Fatalf("request leaked raw context: %s", body)
	}
}

func planEnvelope() *envelope.Envelope {
	env := envelope.NewEnvelope("task", "fix Add", "/repo")
	env.Digest = &envelope.ContextDigest{
		Summary: "digest summary",
		Items: []envelope.DigestItem{
			{
				UnitID:    "u001",
				Path:      "calc.go",
				Relevance: 90,
				Spans:     []envelope.DigestSpan{{StartLine: 1, EndLine: 1, Quote: "return a - b"}},
			},
		},
	}
	env.Raw = &envelope.RawContext{
		Units: []envelope.RawUnit{{ID: "u001", Kind: "file_slice", Path: "calc.go", Text: "SECRET_RAW"}},
	}
	return env
}
