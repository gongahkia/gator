package agent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

type budgetFixture struct{ calls atomic.Int32 }

func (m *budgetFixture) Complete(context.Context, TurnRequest) (Turn, error) {
	m.calls.Add(1)
	return Turn{Text: "done", Usage: Usage{Reported: true, InputTokens: 2, OutputTokens: 1}}, nil
}
func TestSharedBudgetContentionAndUnknownUsage(t *testing.T) {
	provider := &budgetFixture{}
	budget := &Budget{Limits: Limits{ModelRequests: 3}}
	model := WithBudget(provider, budget)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := model.Complete(context.Background(), TurnRequest{})
			if err != nil && !errors.Is(err, ErrBudget) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	usage := budget.Usage()
	if provider.calls.Load() != 3 || usage.ModelRequests != 3 || usage.UnknownRequests != 0 || usage.InputTokens != 6 || usage.OutputTokens != 3 || usage.KnownCost != nil {
		t.Fatalf("aggregate usage: %+v calls=%d", usage, provider.calls.Load())
	}
	unknown := &Budget{Limits: Limits{ModelRequests: 1}}
	if err := unknown.reserve(); err != nil {
		t.Fatal(err)
	}
	unknown.record(Usage{})
	if unknown.Usage().UnknownRequests != 1 || unknown.Usage().Reported {
		t.Fatal("unknown usage represented as reported zero")
	}
	limited := &Budget{Limits: Limits{ModelRequests: 10, Tokens: 3}}
	if _, err := WithBudget(provider, limited).Complete(context.Background(), TurnRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := WithBudget(provider, limited).Complete(context.Background(), TurnRequest{}); !errors.Is(err, ErrBudget) {
		t.Fatal("reported token boundary ignored")
	}
}

func TestBudgetMeetsParentLimitAndDoesNotCountDeniedChildAttempts(t *testing.T) {
	provider := &budgetFixture{}
	global := &Budget{Limits: Limits{ModelRequests: 3}}
	perRun := &Budget{Limits: Limits{ModelRequests: 1}, Parent: global}
	child := &Budget{Limits: Limits{ModelRequests: 8}}
	model := WithBudget(WithBudget(provider, perRun), child)
	if _, err := model.Complete(context.Background(), TurnRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.Complete(context.Background(), TurnRequest{}); !errors.Is(err, ErrBudget) {
		t.Fatal("local limit widened by parent")
	}
	if global.Usage().ModelRequests != 1 || perRun.Usage().ModelRequests != 1 || child.Usage().ModelRequests != 1 || provider.calls.Load() != 1 {
		t.Fatal("denied call counted as provider usage")
	}
}
