package budget

import (
	"testing"

	"github.com/gongahkia/paw/internal/envelope"
)

func TestBudgetAccumulation(t *testing.T) {
	b := envelope.Budget{}
	AddBrain(&b, 10, 3, TokenSourceProvider)
	AddBrain(&b, 5, 2, TokenSourceProvider)
	AddBrainCache(&b, 11, 13, TokenSourceProvider)
	AddDrone(&b, 7, TokenSourceEstimate)
	IncTurn(&b)
	IncTurn(&b)

	if b.BrainInputTokens != 15 || b.BrainOutputTokens != 5 || b.BrainCacheCreationTokens != 11 || b.BrainCacheReadTokens != 13 || b.DroneTokens != 7 || b.Turn != 2 {
		t.Fatalf("budget = %#v", b)
	}
	if b.BrainTokenSource != TokenSourceProvider || b.DroneTokenSource != TokenSourceEstimate {
		t.Fatalf("token sources = brain:%q drone:%q", b.BrainTokenSource, b.DroneTokenSource)
	}
}

func TestBudgetCaps(t *testing.T) {
	b := envelope.Budget{MaxBrainTokens: 100, MaxTurns: 3}
	if ExceededTokens(b) || ExceededTurns(b) {
		t.Fatalf("empty budget exceeded: %#v", b)
	}

	AddBrain(&b, 99, 0, TokenSourceProvider)
	b.Turn = 2
	if ExceededTokens(b) || ExceededTurns(b) {
		t.Fatalf("below caps exceeded: %#v", b)
	}

	AddBrain(&b, 1, 0, TokenSourceProvider)
	if !ExceededTokens(b) {
		t.Fatalf("token cap did not trigger: %#v", b)
	}
	IncTurn(&b)
	if !ExceededTurns(b) {
		t.Fatalf("turn cap did not trigger: %#v", b)
	}
}

func TestMergeTokenSource(t *testing.T) {
	if got := MergeTokenSource("", TokenSourceProvider); got != TokenSourceProvider {
		t.Fatalf("empty/provider = %q", got)
	}
	if got := MergeTokenSource(TokenSourceProvider, TokenSourceProvider); got != TokenSourceProvider {
		t.Fatalf("provider/provider = %q", got)
	}
	if got := MergeTokenSource(TokenSourceProvider, TokenSourceEstimate); got != TokenSourceMixed {
		t.Fatalf("provider/estimate = %q", got)
	}
	if got := SourceForTokens(0, TokenSourceProvider); got != TokenSourceNone {
		t.Fatalf("zero source = %q", got)
	}
}
