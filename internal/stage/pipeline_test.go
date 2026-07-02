package stage

import (
	"context"
	"testing"

	"github.com/gongahkia/paw/internal/envelope"
)

func TestRunLoopOrderingStopsOnVerifyPass(t *testing.T) {
	var order []string
	p := testPipeline(t, &order, map[string]func(*envelope.Envelope){
		"plan": func(env *envelope.Envelope) {
			env.Plan = &envelope.Plan{NextAction: &envelope.NextAction{Kind: "edit_file", Description: "edit", TargetPath: "x"}}
		},
		"verify": func(env *envelope.Envelope) {
			env.Verify = &envelope.VerifyResult{Passed: true}
		},
	})
	env, err := p.RunLoop(context.Background(), envelope.NewEnvelope("task", "fix", "/repo"))
	if err != nil {
		t.Fatalf("run loop: %v", err)
	}
	assertOrder(t, order, []string{"gather", "compress", "plan", "edit", "verify"})
	if !env.Done {
		t.Fatal("expected done after verify pass")
	}
}

func TestRunLoopStopsOnDonePlan(t *testing.T) {
	var order []string
	p := testPipeline(t, &order, map[string]func(*envelope.Envelope){
		"plan": func(env *envelope.Envelope) {
			env.Plan = &envelope.Plan{Done: true, Reasoning: "done"}
		},
	})
	env, err := p.RunLoop(context.Background(), envelope.NewEnvelope("task", "fix", "/repo"))
	if err != nil {
		t.Fatalf("run loop: %v", err)
	}
	assertOrder(t, order, []string{"gather", "compress", "plan"})
	if !env.Done {
		t.Fatal("expected done after plan")
	}
}

func TestRunLoopIncrementsTurnAfterFailedVerify(t *testing.T) {
	var order []string
	verifyCalls := 0
	p := testPipeline(t, &order, map[string]func(*envelope.Envelope){
		"plan": func(env *envelope.Envelope) {
			env.Plan = &envelope.Plan{NextAction: &envelope.NextAction{Kind: "edit_file", Description: "edit", TargetPath: "x"}}
		},
		"verify": func(env *envelope.Envelope) {
			verifyCalls++
			env.Verify = &envelope.VerifyResult{Passed: verifyCalls == 2}
		},
	})
	env, err := p.RunLoop(context.Background(), envelope.NewEnvelope("task", "fix", "/repo"))
	if err != nil {
		t.Fatalf("run loop: %v", err)
	}
	assertOrder(t, order, []string{"gather", "compress", "plan", "edit", "verify", "gather", "compress", "plan", "edit", "verify"})
	if env.Turn != 1 || env.Budget.Turn != 1 {
		t.Fatalf("turns = env:%d budget:%d", env.Turn, env.Budget.Turn)
	}
}

func TestRunLoopStopsOnBudget(t *testing.T) {
	var order []string
	p := testPipeline(t, &order, map[string]func(*envelope.Envelope){
		"plan": func(env *envelope.Envelope) {
			env.Plan = &envelope.Plan{NextAction: &envelope.NextAction{Kind: "edit_file", Description: "edit", TargetPath: "x"}}
		},
		"verify": func(env *envelope.Envelope) {
			env.Verify = &envelope.VerifyResult{Passed: false}
		},
	})
	env := envelope.NewEnvelope("task", "fix", "/repo")
	env.Budget.MaxTurns = 1
	got, err := p.RunLoop(context.Background(), env)
	if err != nil {
		t.Fatalf("run loop: %v", err)
	}
	assertOrder(t, order, []string{"gather", "compress", "plan", "edit", "verify"})
	if got.Turn != 1 || got.Done {
		t.Fatalf("unexpected final env: %#v", got)
	}
}

type fakeStage struct {
	name string
	run  func(*envelope.Envelope)
}

func (s fakeStage) Name() string {
	return s.name
}

func (s fakeStage) Run(_ context.Context, env *envelope.Envelope) (*envelope.Envelope, error) {
	if s.run != nil {
		s.run(env)
	}
	return env, nil
}

func testPipeline(t *testing.T, order *[]string, hooks map[string]func(*envelope.Envelope)) *Pipeline {
	t.Helper()
	names := []string{"gather", "compress", "plan", "edit", "verify"}
	stages := make([]Stage, 0, len(names))
	for _, name := range names {
		name := name
		stages = append(stages, fakeStage{
			name: name,
			run: func(env *envelope.Envelope) {
				*order = append(*order, name)
				if hook := hooks[name]; hook != nil {
					hook(env)
				}
			},
		})
	}
	p, err := NewPipeline(stages...)
	if err != nil {
		t.Fatalf("new pipeline: %v", err)
	}
	return p
}

func assertOrder(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("order length got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order got %v want %v", got, want)
		}
	}
}
