package store

import (
	"testing"

	"github.com/gongahkia/norbot/internal/domain"
)

func TestPlannerDigestAndDiffAreDeterministic(t *testing.T) {
	graph := domain.DefaultGraph()
	architecture := domain.DefaultArchitecture(domain.ProfileFrontend, graph)
	first, err := plannerDigest(architecture, graph)
	if err != nil {
		t.Fatal(err)
	}
	second, err := plannerDigest(architecture, graph)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("digest changed: %s != %s", first, second)
	}
	diff := jsonDiff(map[string]any{"architecture": architecture}, map[string]any{"architecture": architecture, "graph": graph}, "")
	if len(diff) != 1 {
		t.Fatalf("diff=%#v", diff)
	}
	entry, ok := diff[0].(map[string]any)
	if !ok || entry["op"] != "add" || entry["path"] != "/graph" {
		t.Fatalf("entry=%#v", diff[0])
	}
}
