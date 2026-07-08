package gather

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gongahkia/paw/internal/config"
	"github.com/gongahkia/paw/internal/envelope"
)

func BenchmarkGatherFixtureRepo(b *testing.B) {
	dir, err := filepath.Abs("testdata/repo")
	if err != nil {
		b.Fatalf("fixture dir: %v", err)
	}
	stage := New(config.GatherConfig{MaxDepth: 3, MaxFileBytes: 4096})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		env := envelope.NewEnvelope("bench", "inspect KnownSymbol Add", dir)
		if _, err := stage.Run(context.Background(), env); err != nil {
			b.Fatalf("gather: %v", err)
		}
	}
}
