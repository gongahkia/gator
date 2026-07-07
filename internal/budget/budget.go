package budget

import "github.com/gongahkia/paw/internal/envelope"

func AddBrain(b *envelope.Budget, in, out int) {
	b.BrainInputTokens += in
	b.BrainOutputTokens += out
}

func AddBrainCache(b *envelope.Budget, creation, read int) {
	b.BrainCacheCreationTokens += creation
	b.BrainCacheReadTokens += read
}

func AddDrone(b *envelope.Budget, n int) {
	b.DroneTokens += n
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
