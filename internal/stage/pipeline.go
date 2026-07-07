package stage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gongahkia/paw/internal/budget"
	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/ui"
)

type Pipeline struct {
	stages   []Stage
	byName   map[string]Stage
	tracer   *Tracer
	progress *ui.Progress
}

func NewPipeline(stages ...Stage) (*Pipeline, error) {
	p := &Pipeline{
		stages: append([]Stage(nil), stages...),
		byName: map[string]Stage{},
	}
	for _, st := range stages {
		name := st.Name()
		if _, exists := p.byName[name]; exists {
			return nil, fmt.Errorf("duplicate stage %q", name)
		}
		p.byName[name] = st
	}
	return p, nil
}

func (p *Pipeline) SetTracer(tracer *Tracer) {
	p.tracer = tracer
}

func (p *Pipeline) SetProgress(progress *ui.Progress) {
	p.progress = progress
}

func (p *Pipeline) RunOnce(ctx context.Context, name string, env *envelope.Envelope) (*envelope.Envelope, error) {
	st, ok := p.byName[name]
	if !ok {
		return nil, fmt.Errorf("stage %q not found", name)
	}
	start := time.Now()
	inputBytes := envelopeBytes(env)
	before := env.Budget
	out, err := st.Run(ctx, env)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, fmt.Errorf("stage %q returned nil envelope", name)
	}
	if out.Stage == "" {
		out.Stage = name
	}
	duration := time.Since(start)
	if err := p.writeTrace(st, out, before, inputBytes, duration); err != nil {
		return nil, err
	}
	p.writeProgress(name, out, duration)
	return out, nil
}

func (p *Pipeline) RunLoop(ctx context.Context, env *envelope.Envelope) (*envelope.Envelope, error) {
	var err error
	env, err = p.RunOnce(ctx, "gather", env)
	if err != nil || stop(env) {
		return env, err
	}
	for {
		env, err = p.RunOnce(ctx, "compress", env)
		if err != nil || stop(env) {
			return env, err
		}
		env, err = p.RunOnce(ctx, "plan", env)
		if err != nil || done(env) || stop(env) {
			if done(env) {
				env.Done = true
			}
			return env, err
		}
		env, err = p.RunOnce(ctx, "edit", env)
		if err != nil || stop(env) {
			return env, err
		}
		env, err = p.RunOnce(ctx, "verify", env)
		if err != nil || verified(env) {
			if verified(env) {
				env.Done = true
			}
			return env, err
		}
		budget.IncTurn(&env.Budget)
		env.Turn = env.Budget.Turn
		if stop(env) {
			return env, nil
		}
		env, err = p.RunOnce(ctx, "gather", env)
		if err != nil || stop(env) {
			return env, err
		}
	}
}

func done(env *envelope.Envelope) bool {
	return env.Done || env.Plan != nil && env.Plan.Done
}

func verified(env *envelope.Envelope) bool {
	return env.Verify != nil && env.Verify.Passed
}

func stop(env *envelope.Envelope) bool {
	return budget.ExceededTokens(env.Budget) || budget.ExceededTurns(env.Budget)
}

type traceMetadata interface {
	TraceMetadata() (droppedItems int, usedFallback bool)
}

func (p *Pipeline) writeTrace(st Stage, out *envelope.Envelope, before envelope.Budget, inputBytes int, duration time.Duration) error {
	if p.tracer == nil {
		return nil
	}
	dropped, fallback := 0, false
	if meta, ok := st.(traceMetadata); ok {
		dropped, fallback = meta.TraceMetadata()
	}
	after := out.Budget
	return p.tracer.Write(TraceEvent{
		Stage:        out.Stage,
		Turn:         out.Turn,
		InputBytes:   inputBytes,
		OutputBytes:  envelopeBytes(out),
		Tokens:       tokenDelta(before, after),
		DroppedItems: dropped,
		UsedFallback: fallback,
		DurationMS:   duration.Milliseconds(),
	})
}

func (p *Pipeline) writeProgress(stage string, out *envelope.Envelope, duration time.Duration) {
	if p.progress == nil {
		return
	}
	p.progress.StageDone(stage, out, duration)
}

func tokenDelta(before, after envelope.Budget) int {
	return after.BrainInputTokens - before.BrainInputTokens +
		after.BrainOutputTokens - before.BrainOutputTokens +
		after.DroneTokens - before.DroneTokens
}

func envelopeBytes(env *envelope.Envelope) int {
	if env == nil {
		return 0
	}
	data, err := json.Marshal(env)
	if err != nil {
		return 0
	}
	return len(data)
}
