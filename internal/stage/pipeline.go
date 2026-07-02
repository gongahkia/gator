package stage

import (
	"context"
	"fmt"

	"github.com/gongahkia/paw/internal/budget"
	"github.com/gongahkia/paw/internal/envelope"
)

type Pipeline struct {
	stages []Stage
	byName map[string]Stage
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

func (p *Pipeline) RunOnce(ctx context.Context, name string, env *envelope.Envelope) (*envelope.Envelope, error) {
	st, ok := p.byName[name]
	if !ok {
		return nil, fmt.Errorf("stage %q not found", name)
	}
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
