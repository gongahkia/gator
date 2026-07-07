package budget

import (
	"testing"

	"github.com/gongahkia/paw/internal/envelope"
)

func TestBudgetAccumulation(t *testing.T) {
	b := envelope.Budget{}
	AddBrain(&b, 10, 3)
	AddBrain(&b, 5, 2)
	AddBrainCache(&b, 11, 13)
	AddDrone(&b, 7)
	IncTurn(&b)
	IncTurn(&b)

	if b.BrainInputTokens != 15 || b.BrainOutputTokens != 5 || b.BrainCacheCreationTokens != 11 || b.BrainCacheReadTokens != 13 || b.DroneTokens != 7 || b.Turn != 2 {
		t.Fatalf("budget = %#v", b)
	}
}

func TestBudgetCaps(t *testing.T) {
	b := envelope.Budget{MaxBrainTokens: 100, MaxTurns: 3}
	if ExceededTokens(b) || ExceededTurns(b) {
		t.Fatalf("empty budget exceeded: %#v", b)
	}

	AddBrain(&b, 99, 0)
	b.Turn = 2
	if ExceededTokens(b) || ExceededTurns(b) {
		t.Fatalf("below caps exceeded: %#v", b)
	}

	AddBrain(&b, 1, 0)
	if !ExceededTokens(b) {
		t.Fatalf("token cap did not trigger: %#v", b)
	}
	IncTurn(&b)
	if !ExceededTurns(b) {
		t.Fatalf("turn cap did not trigger: %#v", b)
	}
}
