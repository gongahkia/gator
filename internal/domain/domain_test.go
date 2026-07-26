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

func TestGraphRejectsCycleAndDisconnectedNode(t *testing.T) {
	cycle := DefaultGraph()
	cycle.Edges = append(cycle.Edges, GraphEdge{ID: "output-to-input", Source: "output-deployment", Target: "input-request"})
	if err := cycle.Validate(); err == nil { t.Fatal("expected cycle error") }
	disconnected := DefaultGraph()
	disconnected.Nodes = append(disconnected.Nodes, GraphNode{ID:"orphan",Label:"Orphan",Kind:"tool",Optional:true})
	if err := disconnected.Validate(); err == nil { t.Fatal("expected disconnected node error") }
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

func TestDefaultArchitectureIsValid(t *testing.T) {
	architecture := DefaultArchitecture(ProfileFullStack, DefaultGraph())
	if err := architecture.Validate(); err != nil {
		t.Fatal(err)
	}
}
