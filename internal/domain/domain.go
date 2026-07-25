package domain

import (
	"fmt"
	"time"
)

type Stage string

const (
	StagePlanner  Stage = "planner"
	StageBuilder  Stage = "builder"
	StageVerifier Stage = "verifier"
	StageDeployer Stage = "deployer"
)

var Stages = []Stage{StagePlanner, StageBuilder, StageVerifier, StageDeployer}

func (s Stage) Valid() bool {
	for _, candidate := range Stages {
		if s == candidate {
			return true
		}
	}
	return false
}

func (s Stage) Next() (Stage, bool) {
	for i, candidate := range Stages {
		if candidate == s && i+1 < len(Stages) {
			return Stages[i+1], true
		}
	}
	return "", false
}

type Status string

const (
	StatusQueued   Status = "queued"
	StatusRunning  Status = "running"
	StatusAwaiting Status = "awaiting_approval"
	StatusFailed   Status = "failed"
	StatusAbandoned Status = "abandoned"
	StatusCompleted Status = "completed"
)

func (s Status) Terminal() bool {
	return s == StatusFailed || s == StatusAbandoned || s == StatusCompleted
}

type Profile string

const (
	ProfileFrontend Profile = "frontend-only"
	ProfileFullStack Profile = "full-stack"
	ProfileAgentic Profile = "agentic"
)

func (p Profile) Valid() bool {
	return p == ProfileFrontend || p == ProfileFullStack || p == ProfileAgentic
}

type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

type GraphNode struct {
	ID       string         `json:"id"`
	Label    string         `json:"label"`
	Kind     string         `json:"kind"`
	Optional bool           `json:"optional"`
	Data     map[string]any `json:"data,omitempty"`
}

type GraphEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

func (g Graph) Validate() error {
	if len(g.Nodes) < 2 {
		return fmt.Errorf("graph requires at least two nodes")
	}
	nodes := make(map[string]struct{}, len(g.Nodes))
	hasInput, hasOutput := false, false
	for _, node := range g.Nodes {
		if node.ID == "" || node.Label == "" || node.Kind == "" {
			return fmt.Errorf("every node requires id, label, and kind")
		}
		if _, exists := nodes[node.ID]; exists {
			return fmt.Errorf("duplicate node id %q", node.ID)
		}
		nodes[node.ID] = struct{}{}
		hasInput = hasInput || node.Kind == "input"
		hasOutput = hasOutput || node.Kind == "output"
	}
	if !hasInput || !hasOutput {
		return fmt.Errorf("graph requires input and output nodes")
	}
	edges := map[string]struct{}{}
	for _, edge := range g.Edges {
		if edge.ID == "" || edge.Source == "" || edge.Target == "" {
			return fmt.Errorf("every edge requires id, source, and target")
		}
		if _, exists := nodes[edge.Source]; !exists {
			return fmt.Errorf("edge %q source is unknown", edge.ID)
		}
		if _, exists := nodes[edge.Target]; !exists {
			return fmt.Errorf("edge %q target is unknown", edge.ID)
		}
		if _, exists := edges[edge.ID]; exists {
			return fmt.Errorf("duplicate edge id %q", edge.ID)
		}
		edges[edge.ID] = struct{}{}
	}
	return nil
}

func DefaultGraph() Graph {
	return Graph{
		Nodes: []GraphNode{
			{ID: "input-request", Label: "User request", Kind: "input"},
			{ID: "logic-build", Label: "Build app", Kind: "logic"},
			{ID: "output-deployment", Label: "Deployed app", Kind: "output"},
		},
		Edges: []GraphEdge{
			{ID: "input-request-to-logic-build", Source: "input-request", Target: "logic-build"},
			{ID: "logic-build-to-output-deployment", Source: "logic-build", Target: "output-deployment"},
		},
	}
}

type Run struct {
	ID            string           `json:"id"`
	Prompt        string           `json:"prompt"`
	Profile       Profile          `json:"profile"`
	Stage         Stage            `json:"stage"`
	Status        Status           `json:"status"`
	Providers     map[Stage]string `json:"providers"`
	Graph         Graph            `json:"graph"`
	Feedback      string           `json:"feedback,omitempty"`
	FailureReason string           `json:"failure_reason,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

type Event struct {
	ID        int64          `json:"id"`
	RunID     string         `json:"run_id"`
	Type      string         `json:"type"`
	Message   string         `json:"message"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
}

type Job struct {
	ID      int64  `json:"id"`
	RunID   string `json:"run_id"`
	Stage   Stage  `json:"stage"`
	Attempt int    `json:"attempt"`
}

type ApprovalAction string

const (
	ApprovalApprove ApprovalAction = "approve"
	ApprovalRevise  ApprovalAction = "revise"
	ApprovalRetry   ApprovalAction = "retry"
	ApprovalAbandon ApprovalAction = "abandon"
)
