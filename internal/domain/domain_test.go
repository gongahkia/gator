package domain

import "testing"

func TestDefaultGraphIsValid(t *testing.T) {
	if err := DefaultGraph().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestGraphRejectsUnknownEdgeEndpoint(t *testing.T) {
	graph := DefaultGraph()
	graph.Edges[0].Target = "missing"
	if err := graph.Validate(); err == nil {
		t.Fatal("expected edge validation error")
	}
}

func TestStageOrder(t *testing.T) {
	next, ok := StageVerifier.Next()
	if !ok || next != StageDeployer {
		t.Fatalf("got %q %t", next, ok)
	}
	if _, ok := StageDeployer.Next(); ok {
		t.Fatal("deployer must be final")
	}
}
