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

func TestPlanRejectsUnsupportedActionKind(t *testing.T) {
	srv := faketest.NewServer()
	defer srv.Close()
	srv.RespondOpenAI("", `{"done":false,"reasoning":"run","next_action":{"kind":"run_command","description":"run tests","command":"go test ./..."}}`)

	_, err := New(llm.NewOpenAIClient(srv.URL, "key", "brain")).Run(context.Background(), planEnvelope())
	if err == nil || !strings.Contains(err.Error(), "edit_file") {
		t.Fatalf("expected unsupported action error, got %v", err)
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

func TestPlanRawContextModeSendsRawContext(t *testing.T) {
	srv := faketest.NewServer()
	defer srv.Close()
	srv.RespondOpenAI("", `{"done":false,"reasoning":"edit","next_action":{"kind":"edit_file","description":"fix","target_path":"calc.go"}}`)

	stage := New(llm.NewOpenAIClient(srv.URL, "key", "brain"))
	stage.UseRawContext = true
	if _, err := stage.Run(context.Background(), planEnvelope()); err != nil {
		t.Fatalf("plan: %v", err)
	}
	body := srv.LastRequest().Body
	if !strings.Contains(body, "SECRET_RAW") {
		t.Fatalf("request missing raw context: %s", body)
	}
}

func TestPlanAddsBrainCacheUsage(t *testing.T) {
	fake := &fakePlanClient{usage: llm.Usage{
		InputTokens:              1,
		OutputTokens:             2,
		CacheCreationInputTokens: 3,
		CacheReadInputTokens:     4,
	}}
	stage := New(fake)
	got, err := stage.Run(context.Background(), planEnvelope())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if got.Budget.BrainInputTokens != 1 || got.Budget.BrainOutputTokens != 2 || got.Budget.BrainCacheCreationTokens != 3 || got.Budget.BrainCacheReadTokens != 4 {
		t.Fatalf("budget = %#v", got.Budget)
	}
	if fake.request.MaxTokens != maxPlanOutputTokens {
		t.Fatalf("max tokens = %d", fake.request.MaxTokens)
	}
}

type fakePlanClient struct {
	usage   llm.Usage
	request llm.ChatRequest
}

func (f *fakePlanClient) Chat(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	f.request = req
	return &llm.ChatResponse{
		Content: `{"done":true,"reasoning":"done"}`,
		Usage:   f.usage,
	}, nil
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
