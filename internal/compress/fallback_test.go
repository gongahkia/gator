package compress

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gongahkia/paw/internal/envelope"
	"github.com/gongahkia/paw/internal/llm"
)

func TestFallbackRankingStable(t *testing.T) {
	raw := fallbackRaw()
	a := fallbackDigest("fix alpha target", raw, 1000)
	b := fallbackDigest("fix alpha target", raw, 1000)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("fallback not stable\n%#v\n%#v", a, b)
	}
	if len(a.Items) == 0 || a.Items[0].UnitID != "u002" {
		t.Fatalf("unexpected ranking: %#v", a.Items)
	}
}

func TestFallbackRespectsTokenBudget(t *testing.T) {
	raw := &envelope.RawContext{
		Units: []envelope.RawUnit{
			{
				ID:        "u001",
				Kind:      "file_slice",
				Path:      "long.go",
				StartLine: 1,
				EndLine:   1,
				Text:      strings.Repeat("alpha ", 400),
			},
		},
	}
	digest := fallbackDigest("alpha", raw, 20)
	total := 0
	for _, item := range digest.Items {
		for _, span := range item.Spans {
			total += llm.Estimate(span.Quote)
		}
	}
	if total > 20 {
		t.Fatalf("token budget exceeded: %d", total)
	}
}

func fallbackRaw() *envelope.RawContext {
	return &envelope.RawContext{
		Units: []envelope.RawUnit{
			{
				ID:        "u001",
				Kind:      "file_slice",
				Path:      "beta.go",
				StartLine: 1,
				EndLine:   2,
				Text:      "beta unrelated text",
			},
			{
				ID:        "u002",
				Kind:      "file_slice",
				Path:      "alpha.go",
				StartLine: 1,
				EndLine:   2,
				Text:      "alpha target alpha target",
			},
		},
	}
}
