package usage

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gongahkia/gator/internal/agent"
	"time"
)

type categoryKey struct{}
type stateKey struct{}

func WithCategory(ctx context.Context, category string) context.Context {
	return context.WithValue(ctx, categoryKey{}, category)
}
func WithState(ctx context.Context, stateDir string) context.Context {
	return context.WithValue(ctx, stateKey{}, stateDir)
}
func Category(ctx context.Context) string {
	v, _ := ctx.Value(categoryKey{}).(string)
	if v == "" {
		v = "work"
	}
	return v
}

type model struct {
	backend         agent.Model
	state, provider string
}

func Model(backend agent.Model, stateDir, provider string) agent.Model {
	return &model{backend, stateDir, provider}
}
func (m *model) begin(ctx context.Context, request agent.TurnRequest) (*Reservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root := m.state
	if selected, _ := ctx.Value(stateKey{}).(string); selected != "" {
		root = selected
	}
	s, err := Open(root)
	if err != nil {
		return nil, err
	}
	p, err := s.Policy()
	if err != nil {
		return nil, err
	}
	reserve := p.ResponseReserve
	if reserve == 0 {
		reserve = 8192
	}
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	r, err := s.Reserve(time.Now(), Category(ctx), m.provider, int64(len(data))+reserve)
	if errors.Is(err, ErrLimit) {
		return nil, errors.Join(agent.ErrBudget, err)
	}
	return r, err
}
func (m *model) Complete(ctx context.Context, request agent.TurnRequest) (agent.Turn, error) {
	r, err := m.begin(ctx, request)
	if err != nil {
		return agent.Turn{}, err
	}
	t, err := m.backend.Complete(ctx, request)
	finish := r.Finish(t.Usage.InputTokens+t.Usage.OutputTokens, t.Usage.Reported, err != nil, t.Usage.KnownCost)
	return t, errors.Join(err, finish)
}
func (m *model) CompleteStream(ctx context.Context, request agent.TurnRequest, delta func(string)) (agent.Turn, error) {
	stream, ok := m.backend.(agent.StreamingModel)
	if !ok {
		return m.Complete(ctx, request)
	}
	r, err := m.begin(ctx, request)
	if err != nil {
		return agent.Turn{}, err
	}
	t, err := stream.CompleteStream(ctx, request, delta)
	finish := r.Finish(t.Usage.InputTokens+t.Usage.OutputTokens, t.Usage.Reported, err != nil, t.Usage.KnownCost)
	return t, errors.Join(err, finish)
}
func (m *model) SupportsVisualInput() bool {
	v, ok := m.backend.(agent.VisualInputModel)
	return !ok || v.SupportsVisualInput()
}
func (m *model) ComputerUse() (agent.ComputerUse, bool) {
	if v, ok := m.backend.(agent.ComputerUseModel); ok {
		return v.ComputerUse()
	}
	return agent.ComputerUse{}, false
}
