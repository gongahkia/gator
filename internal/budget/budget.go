package budget

import (
	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/llm"
)

const (
	TokenSourceProvider = llm.TokenSourceProvider
	TokenSourceEstimate = llm.TokenSourceEstimate
	TokenSourceMixed    = "mixed"
	TokenSourceNone     = "none"
)

func AddBrain(b *envelope.Budget, in, out int, source string) {
	b.BrainInputTokens += in
	b.BrainOutputTokens += out
	if in+out > 0 {
		b.BrainTokenSource = MergeTokenSource(b.BrainTokenSource, source)
	}
}

func AddBrainCache(b *envelope.Budget, creation, read int, source string) {
	b.BrainCacheCreationTokens += creation
	b.BrainCacheReadTokens += read
	if creation+read > 0 {
		b.BrainTokenSource = MergeTokenSource(b.BrainTokenSource, source)
	}
}

func AddDrone(b *envelope.Budget, n int, source string) {
	b.DroneTokens += n
	if n > 0 {
		b.DroneTokenSource = MergeTokenSource(b.DroneTokenSource, source)
	}
}

func IncTurn(b *envelope.Budget) {
	b.Turn++
}

func ExceededTokens(b envelope.Budget) bool {
	return b.MaxBrainTokens > 0 && b.BrainInputTokens >= b.MaxBrainTokens
}

func ExceededTurns(b envelope.Budget) bool {
	return b.MaxTurns > 0 && b.Turn >= b.MaxTurns
}

func MergeTokenSource(existing, next string) string {
	existing = NormalizeTokenSource(existing)
	next = NormalizeTokenSource(next)
	if next == TokenSourceNone {
		return existing
	}
	if existing == TokenSourceNone {
		return next
	}
	if existing == next {
		return existing
	}
	return TokenSourceMixed
}

func NormalizeTokenSource(source string) string {
	switch source {
	case TokenSourceProvider, TokenSourceEstimate, TokenSourceMixed:
		return source
	default:
		return TokenSourceNone
	}
}

func SourceForTokens(tokens int, source string) string {
	if tokens <= 0 {
		return TokenSourceNone
	}
	return NormalizeTokenSource(source)
}
