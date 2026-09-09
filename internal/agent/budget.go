package agent

import (
	"context"
	"errors"
	"sync"
)

type Usage struct {
	UnknownRequests int      `json:"unknown_requests"`
	ModelRequests   int      `json:"model_requests"`
	InputTokens     int64    `json:"input_tokens"`
	OutputTokens    int64    `json:"output_tokens"`
	Reported        bool     `json:"reported"`
	Estimated       bool     `json:"estimated"`
	KnownCost       *float64 `json:"known_cost,omitempty"`
}
type Limits struct {
	ModelRequests int   `json:"model_requests"`
	Tokens        int64 `json:"tokens"`
	WallSeconds   int   `json:"wall_seconds"`
}
type Budget struct {
	Parent *Budget
	mu     sync.Mutex
	Limits Limits
	usage  Usage
}

var ErrBudget = errors.New("aggregate model budget exhausted")

func (b *Budget) reserve() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.usage.ModelRequests >= b.Limits.ModelRequests || (b.Limits.Tokens > 0 && b.usage.InputTokens+b.usage.OutputTokens >= b.Limits.Tokens) {
		return ErrBudget
	}
	if b.Parent != nil {
		if err := b.Parent.reserve(); err != nil {
			return err
		}
	}
	b.usage.ModelRequests++
	b.usage.UnknownRequests++
	return nil
}
func (b *Budget) release() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.usage.ModelRequests--
	b.usage.UnknownRequests--
	if b.Parent != nil {
		b.Parent.release()
	}
}

func (b *Budget) record(usage Usage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if usage.Reported || usage.Estimated {
		b.usage.UnknownRequests--
	}
	b.usage.InputTokens += usage.InputTokens
	b.usage.OutputTokens += usage.OutputTokens
	b.usage.Reported = b.usage.Reported || usage.Reported
	b.usage.Estimated = b.usage.Estimated || usage.Estimated
	if b.Parent != nil {
		b.Parent.record(usage)
	}
}
func (b *Budget) Usage() Usage { b.mu.Lock(); defer b.mu.Unlock(); return b.usage }

type budgetModel struct {
	model  Model
	budget *Budget
}

func WithBudget(model Model, budget *Budget) Model {
	if budget == nil {
		return model
	}
	return &budgetModel{model, budget}
}
func (m *budgetModel) Complete(ctx context.Context, request TurnRequest) (Turn, error) {
	if err := ctx.Err(); err != nil {
		return Turn{}, err
	}
	if err := m.budget.reserve(); err != nil {
		return Turn{}, err
	}
	turn, err := m.model.Complete(ctx, request)
	if errors.Is(err, ErrBudget) {
		m.budget.release()
	} else {
		m.budget.record(turn.Usage)
	}
	return turn, err
}
func (m *budgetModel) CompleteStream(ctx context.Context, request TurnRequest, delta func(string)) (Turn, error) {
	if model, ok := m.model.(StreamingModel); ok {
		if err := ctx.Err(); err != nil {
			return Turn{}, err
		}
		if err := m.budget.reserve(); err != nil {
			return Turn{}, err
		}
		turn, err := model.CompleteStream(ctx, request, delta)
		if errors.Is(err, ErrBudget) {
			m.budget.release()
		} else {
			m.budget.record(turn.Usage)
		}
		return turn, err
	}
	return m.Complete(ctx, request)
}
func (m *budgetModel) SupportsVisualInput() bool {
	if model, ok := m.model.(VisualInputModel); ok {
		return model.SupportsVisualInput()
	}
	return true
}
