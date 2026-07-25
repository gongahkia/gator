package tui

import (
	"github.com/gongahkia/norbot/internal/domain"
	"testing"
)

func TestLinearEdges(t *testing.T) {
	nodes := []domain.GraphNode{{ID: "input", Kind: "input", Label: "Input"}, {ID: "output", Kind: "output", Label: "Output"}}
	edges := linearEdges(nodes)
	if len(edges) != 1 || edges[0].Source != "input" || edges[0].Target != "output" {
		t.Fatalf("bad edges %#v", edges)
	}
}
