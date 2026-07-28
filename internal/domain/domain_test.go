package domain

import (
	"strings"
	"testing"
)

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
	if err := cycle.Validate(); err == nil {
		t.Fatal("expected cycle error")
	}
	disconnected := DefaultGraph()
	disconnected.Nodes = append(disconnected.Nodes, GraphNode{ID: "orphan", Label: "Orphan", Kind: "tool", Optional: true})
	if err := disconnected.Validate(); err == nil {
		t.Fatal("expected disconnected node error")
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

func TestDefaultArchitectureIsValid(t *testing.T) {
	architecture := DefaultArchitecture(ProfileFullStack, DefaultGraph())
	if err := architecture.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAcceptanceContractBoundsAndProfile(t *testing.T) {
	contract := AcceptanceContract{Version: 1, Flows: []AcceptanceFlow{{ID: "home", Name: "Home", Steps: []AcceptanceStep{{Kind: "goto", URL: "/"}}}}, Screenshots: []ScreenshotExpectation{{ID: "home", Path: "/"}}}
	if err := contract.Validate(); err != nil {
		t.Fatal(err)
	}
	contract.Screenshots = append(contract.Screenshots, ScreenshotExpectation{ID: "home", Path: "/"})
	if err := contract.Validate(); err == nil {
		t.Fatal("duplicate screenshot accepted")
	}
	contract.Screenshots = contract.Screenshots[:1]
	contract.APIContracts = []APIContract{{ID: "health", Method: "GET", Path: "/api/health", Status: 200}}
	if err := ValidateAcceptanceForProfile(ProfileFrontend, contract); err == nil {
		t.Fatal("frontend API contract accepted")
	}
}

func TestAcceptanceSupportsSelectorWorkflow(t *testing.T) {
	count := 0
	contract := AcceptanceContract{Version: 1, Flows: []AcceptanceFlow{{ID: "crud", Name: "CRUD", Steps: []AcceptanceStep{
		{Kind: "goto", URL: "/"},
		{Kind: "set_value", Selector: "[data-testid=\"new-task\"]", Value: "Buy milk"},
		{Kind: "press_key", Selector: "[data-testid=\"new-task\"]", Key: "Enter"},
		{Kind: "press_key", Key: "Escape"},
		{Kind: "expect_value", Selector: "[data-testid=\"new-task\"]", Value: ""},
		{Kind: "expect_text", Selector: "[data-testid=\"task-title\"]", Text: "Buy milk"},
		{Kind: "expect_attribute", Selector: "[data-testid=\"task\"]", Attribute: "data-completed", Value: "true"},
		{Kind: "expect_attribute", Selector: "[data-testid=\"task\"]", Name: "data-selected", Value: "true"},
		{Kind: "expect_count", Selector: "[data-testid=\"task\"]", Count: &count},
		{Kind: "reload"},
	}}}}
	if err := contract.Validate(); err != nil {
		t.Fatal(err)
	}
	contract.Flows[0].Steps[3].Key = ""
	if err := contract.Validate(); err == nil || !strings.Contains(err.Error(), "press_key requires key") {
		t.Fatalf("err=%v", err)
	}
}

func TestAcceptanceSupportsLocalStorageOperations(t *testing.T) {
	for _, step := range []AcceptanceStep{
		{Kind: "local_storage", Op: "clear"},
		{Kind: "local_storage", Op: "set", Key: "task-tracker:tasks", Value: "[]"},
		{Kind: "local_storage", Key: "task-tracker:tasks", Value: "[]"},
	} {
		if err := step.Validate(); err != nil {
			t.Fatalf("step=%#v err=%v", step, err)
		}
	}
	for _, step := range []AcceptanceStep{
		{Kind: "local_storage", Op: "clear", Key: "unexpected"},
		{Kind: "local_storage", Op: "set", Key: "task-tracker:tasks"},
		{Kind: "local_storage", Op: "delete", Key: "task-tracker:tasks"},
	} {
		if err := step.Validate(); err == nil {
			t.Fatalf("invalid step accepted: %#v", step)
		}
	}
}

func TestPlannerArchitectureRequiresConcreteProfileContract(t *testing.T) {
	architecture := DefaultArchitecture(ProfileFrontend, DefaultGraph())
	architecture.Acceptance = CompileAcceptance(architecture)
	if err := ValidateArchitectureForProfile(ProfileFrontend, architecture); err == nil {
		t.Fatal("placeholder planner contract accepted")
	}
	architecture.AppName = "Task Tracker"
	architecture.Stack = []string{"HTML", "JavaScript"}
	architecture.CoreFeatures = []Feature{{ID: "tasks", Name: "Task CRUD", Description: "Create tasks", Role: "app_logic", Selected: true}}
	architecture.Acceptance = AcceptanceContract{Version: 1, Flows: []AcceptanceFlow{{ID: "tasks", Name: "Task CRUD", Steps: []AcceptanceStep{{Kind: "goto", URL: "/"}, {Kind: "expect_text", Text: "Task Tracker"}}}}}
	if err := ValidateArchitectureForProfile(ProfileFrontend, architecture); err != nil {
		t.Fatal(err)
	}
	architecture.AppType = string(ProfileFullStack)
	if err := ValidateArchitectureForProfile(ProfileFrontend, architecture); err == nil {
		t.Fatal("profile mismatch accepted")
	}
}
