package cmd

import (
	"path/filepath"
	"testing"

	"github.com/gongahkia/paw/internal/stage"
)

func BenchmarkStatsSummaryCompactTrace(b *testing.B) {
	path := filepath.Join("testdata", "trace", "stats.ndjson")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		events, err := stage.ReadTraceEvents(path)
		if err != nil {
			b.Fatalf("read trace: %v", err)
		}
		if _, err := summarizeTrace(path, "bench", events); err != nil {
			b.Fatalf("summarize trace: %v", err)
		}
	}
}
