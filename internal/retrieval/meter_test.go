package retrieval

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/usage"
)

type requestEmbedder struct{ calls int }

func (*requestEmbedder) ID() string { return "gemini/test" }
func (e *requestEmbedder) Embed(_ context.Context, input []string) ([][]float32, error) {
	e.calls++
	return [][]float32{{1, 0}}, nil
}
func TestEmbeddingMeterCountsActualPerInputRequestsAgainstSharedBudget(t *testing.T) {
	root := t.TempDir()
	s, _ := usage.Open(root)
	if err := s.SavePolicy(usage.Policy{DailyRequests: 2}); err != nil {
		t.Fatal(err)
	}
	e := &requestEmbedder{}
	_, err := Meter(e, root).Embed(context.Background(), []string{"one", "two", "three"})
	if !errors.Is(err, usage.ErrLimit) || e.calls != 2 {
		t.Fatalf("embedding requests bypassed shared admission: %d %v", e.calls, err)
	}
	view, _ := s.Snapshot(time.Now())
	if view.Requests != 2 || view.Categories["retrieval"] != 2 || view.EstimatedRequests != 2 {
		t.Fatalf("retrieval usage: %#v", view)
	}
}
