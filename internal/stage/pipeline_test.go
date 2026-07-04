package stage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/paw/internal/compress"
	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/edit"
	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/gather"
	"github.com/gongahkia/paw/internal/llm"
	"github.com/gongahkia/paw/internal/plan"
	"github.com/gongahkia/paw/internal/verify"
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

func TestRunLoopRealStagesFakeLLM(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/integration\n\ngo 1.23\n")
	writeFile(t, dir, "calc.go", strings.Join([]string{
		"package fixture",
		"",
		"func Add(a int, b int) int {",
		"\treturn a - b",
		"}",
		"",
	}, "\n"))
	writeFile(t, dir, "calc_test.go", strings.Join([]string{
		"package fixture",
		"",
		"import \"testing\"",
		"",
		"func TestAdd(t *testing.T) {",
		"\tif got := Add(2, 3); got != 5 {",
		"\t\tt.Fatalf(\"Add = %d\", got)",
		"\t}",
		"}",
		"",
	}, "\n"))

	fake := &realStageFakeLLM{t: t}
	p, err := NewPipeline(
		gather.New(config.GatherConfig{MaxDepth: 2, MaxFileBytes: 4096}),
		compress.New(fake),
		plan.New(fake),
		edit.New(fake),
		verify.New("go test ./..."),
	)
	if err != nil {
		t.Fatalf("new pipeline: %v", err)
	}

	env := envelope.NewEnvelope("task", "fix Add in calc.go so TestAdd passes", dir)
	got, err := p.RunLoop(context.Background(), env)
	if err != nil {
		t.Fatalf("run loop: %v", err)
	}
	if !got.Done || got.Verify == nil || !got.Verify.Passed {
		t.Fatalf("expected verified done env, got %#v", got)
	}
	if source := readFile(t, dir, "calc.go"); !strings.Contains(source, "return a + b") {
		t.Fatalf("patch not applied:\n%s", source)
	}
	if got.Raw == nil || got.Raw.TotalBytes == 0 {
		t.Fatalf("raw context missing: %#v", got.Raw)
	}
	if got.Budget.BrainInputTokens >= got.Raw.TotalBytes/4 {
		t.Fatalf("brain input tokens %d >= raw bytes/4 %d", got.Budget.BrainInputTokens, got.Raw.TotalBytes/4)
	}
	fake.assertCalls(t, []string{"compress", "plan", "edit"})
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

type realStageFakeLLM struct {
	t     *testing.T
	calls []string
}

func (f *realStageFakeLLM) Chat(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	body := messageBody(req.Messages)
	switch {
	case strings.Contains(body, "context compression drone"):
		f.calls = append(f.calls, "compress")
		return &llm.ChatResponse{Content: f.digest(body), Usage: llm.Usage{InputTokens: 13, OutputTokens: 5}}, nil
	case strings.Contains(body, "brain planning stage"):
		f.calls = append(f.calls, "plan")
		if strings.Contains(body, "RawContext JSON") {
			f.t.Fatalf("plan prompt included raw context")
		}
		return &llm.ChatResponse{
			Content: `{"done":false,"reasoning":"fix failing Add implementation","next_action":{"kind":"edit_file","description":"change Add to return the sum","target_path":"calc.go"}}`,
			Usage:   llm.Usage{InputTokens: 9, OutputTokens: 4},
		}, nil
	case strings.Contains(body, "Emit only a unified diff."):
		f.calls = append(f.calls, "edit")
		return &llm.ChatResponse{
			Content: strings.Join([]string{
				"--- a/calc.go",
				"+++ b/calc.go",
				"@@ -1,5 +1,5 @@",
				" package fixture",
				" ",
				" func Add(a int, b int) int {",
				"-\treturn a - b",
				"+\treturn a + b",
				" }",
				"",
			}, "\n"),
			Usage: llm.Usage{InputTokens: 9, OutputTokens: 4},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected llm prompt: %.120s", body)
	}
}

func (f *realStageFakeLLM) digest(prompt string) string {
	const marker = "RawContext JSON:\n"
	idx := strings.LastIndex(prompt, marker)
	if idx == -1 {
		f.t.Fatalf("compress prompt missing raw context")
	}
	var raw envelope.RawContext
	if err := json.Unmarshal([]byte(prompt[idx+len(marker):]), &raw); err != nil {
		f.t.Fatalf("decode raw context: %v", err)
	}
	for _, unit := range raw.Units {
		if unit.Path == "calc.go" && unit.Kind == "file_slice" && strings.Contains(unit.Text, "return a - b") {
			out := envelope.ContextDigest{
				Summary: "Add subtracts instead of adding.",
				Items: []envelope.DigestItem{{
					UnitID:    unit.ID,
					Path:      unit.Path,
					Relevance: 100,
					Spans: []envelope.DigestSpan{{
						StartLine: unit.StartLine,
						EndLine:   unit.EndLine,
						Quote:     "return a - b",
					}},
				}},
			}
			b, err := json.Marshal(out)
			if err != nil {
				f.t.Fatalf("encode digest: %v", err)
			}
			return string(b)
		}
	}
	f.t.Fatalf("raw context missing calc.go file slice: %#v", raw)
	return ""
}

func (f *realStageFakeLLM) assertCalls(t *testing.T, want []string) {
	t.Helper()
	if len(f.calls) != len(want) {
		t.Fatalf("llm calls got %v want %v", f.calls, want)
	}
	for i := range want {
		if f.calls[i] != want[i] {
			t.Fatalf("llm calls got %v want %v", f.calls, want)
		}
	}
}

func messageBody(messages []llm.ChatMessage) string {
	var b strings.Builder
	for _, msg := range messages {
		b.WriteString(msg.Content)
		b.WriteByte('\n')
	}
	return b.String()
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func readFile(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}
