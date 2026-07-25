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
	StatusQueued      Status = "queued"
	StatusRunning     Status = "running"
	StatusAwaiting    Status = "awaiting_approval"
	StatusFailed      Status = "failed"
	StatusInterrupted Status = "interrupted"
	StatusAbandoned   Status = "abandoned"
	StatusCompleted   Status = "completed"
)

func (s Status) Terminal() bool {
	return s == StatusFailed || s == StatusInterrupted || s == StatusAbandoned || s == StatusCompleted
}

type Profile string

const (
	ProfileFrontend  Profile = "frontend-only"
	ProfileFullStack Profile = "full-stack"
	ProfileAgentic   Profile = "agentic"
)

func (p Profile) Valid() bool {
	return p == ProfileFrontend || p == ProfileFullStack || p == ProfileAgentic
}

type DeploymentTarget string

const (
	DeploymentDocker     DeploymentTarget = "docker"
	DeploymentKubernetes DeploymentTarget = "kubernetes"
)

func (t DeploymentTarget) Valid() bool { return t == DeploymentDocker || t == DeploymentKubernetes }

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
	adjacency := make(map[string][]string, len(g.Nodes))
	reverse := make(map[string][]string, len(g.Nodes))
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
		if edge.Source == edge.Target {
			return fmt.Errorf("edge %q cannot self-reference", edge.ID)
		}
		adjacency[edge.Source] = append(adjacency[edge.Source], edge.Target)
		reverse[edge.Target] = append(reverse[edge.Target], edge.Source)
	}
	starts, outputs := []string{}, []string{}
	for _, node := range g.Nodes {
		if node.Kind == "input" {
			starts = append(starts, node.ID)
		}
		if node.Kind == "output" {
			outputs = append(outputs, node.ID)
		}
	}
	if hasCycle(nodes, adjacency) {
		return fmt.Errorf("graph cannot contain cycles")
	}
	if !allReachable(starts, adjacency, nodes) {
		return fmt.Errorf("every node must be reachable from an input")
	}
	if !allReachable(outputs, reverse, nodes) {
		return fmt.Errorf("every node must reach an output")
	}
	return nil
}

func allReachable(starts []string, adjacency map[string][]string, nodes map[string]struct{}) bool {
	seen := map[string]bool{}
	queue := append([]string(nil), starts...)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if seen[current] {
			continue
		}
		seen[current] = true
		queue = append(queue, adjacency[current]...)
	}
	return len(seen) == len(nodes)
}

func hasCycle(nodes map[string]struct{}, adjacency map[string][]string) bool {
	state := map[string]uint8{}
	var visit func(string) bool
	visit = func(id string) bool {
		if state[id] == 1 {
			return true
		}
		if state[id] == 2 {
			return false
		}
		state[id] = 1
		for _, next := range adjacency[id] {
			if visit(next) {
				return true
			}
		}
		state[id] = 2
		return false
	}
	for id := range nodes {
		if visit(id) {
			return true
		}
	}
	return false
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
	ID               string           `json:"id"`
	Prompt           string           `json:"prompt"`
	Profile          Profile          `json:"profile"`
	DeploymentTarget DeploymentTarget `json:"deployment_target"`
	PublicIngress    bool             `json:"public_ingress"`
	MaxFixes         int              `json:"max_fixes"`
	Stage            Stage            `json:"stage"`
	Status           Status           `json:"status"`
	Providers        map[Stage]string `json:"providers"`
	Graph            Graph            `json:"graph"`
	Feedback         string           `json:"feedback,omitempty"`
	FailureReason    string           `json:"failure_reason,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

type ReviewKind string

const (
	ReviewCode ReviewKind = "code"
	ReviewTest ReviewKind = "test"
	ReviewFix  ReviewKind = "fix"
)

func (k ReviewKind) Valid() bool { return k == ReviewCode || k == ReviewTest || k == ReviewFix }

type Revision struct {
	ID             int64             `json:"id"`
	RunID          string            `json:"run_id"`
	Kind           ReviewKind        `json:"kind"`
	Attempt        int               `json:"attempt"`
	BaselineDigest string            `json:"baseline_digest"`
	PatchDigest    string            `json:"patch_digest"`
	Files          map[string]string `json:"files,omitempty"`
	Report         map[string]any    `json:"report,omitempty"`
	State          string            `json:"state"`
	CreatedAt      time.Time         `json:"created_at"`
	ApprovedAt     *time.Time        `json:"approved_at,omitempty"`
}

type UsageRecord struct {
	ID           int64          `json:"id"`
	RunID        string         `json:"run_id"`
	Stage        Stage          `json:"stage"`
	RevisionID   *int64         `json:"revision_id,omitempty"`
	ProviderID   string         `json:"provider_id"`
	Model        string         `json:"model"`
	InputTokens  int            `json:"input_tokens"`
	OutputTokens int            `json:"output_tokens"`
	CachedTokens int            `json:"cached_tokens"`
	Source       string         `json:"source"`
	Estimator    string         `json:"estimator,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

type SkillPackage struct {
	Digest      string         `json:"digest"`
	ID          string         `json:"id"`
	Version     string         `json:"version"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Manifest    map[string]any `json:"manifest"`
	Path        string         `json:"path"`
	CreatedAt   time.Time      `json:"created_at"`
}

type SkillImport struct {
	ID            int64          `json:"id"`
	SourceType    string         `json:"source_type"`
	SourceURI     string         `json:"source_uri"`
	SourceRef     string         `json:"source_ref"`
	CredentialEnv string         `json:"credential_env,omitempty"`
	Digest        string         `json:"digest,omitempty"`
	State         string         `json:"state"`
	Findings      map[string]any `json:"findings"`
	ActivatedAt   *time.Time     `json:"activated_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
}

type SkillSelection struct {
	RunID       string    `json:"run_id"`
	SkillDigest string    `json:"skill_digest"`
	SelectedAt  time.Time `json:"selected_at"`
}

type HealthState string

const (
	HealthHealthy  HealthState = "healthy"
	HealthDegraded HealthState = "degraded"
	HealthDown     HealthState = "down"
)

type HealthCheck struct {
	ID          string         `json:"id"`
	State       HealthState    `json:"state"`
	Critical    bool           `json:"critical"`
	LatencyMS   int64          `json:"latency_ms"`
	Message     string         `json:"message"`
	Diagnostics map[string]any `json:"diagnostics,omitempty"`
	LogTail     string         `json:"log_tail,omitempty"`
	CheckedAt   time.Time      `json:"checked_at"`
}

type HealthReport struct {
	State     HealthState   `json:"state"`
	Checks    []HealthCheck `json:"checks"`
	CheckedAt time.Time     `json:"checked_at"`
}

type ChannelAccount struct {
	ID         string            `json:"id"`
	RunID      string            `json:"run_id"`
	Adapter    string            `json:"adapter"`
	Name       string            `json:"name"`
	SecretRefs map[string]string `json:"secret_refs"`
	Settings   map[string]any    `json:"settings"`
	Enabled    bool              `json:"enabled"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type ChannelPairing struct {
	AccountID  string     `json:"account_id"`
	ExternalID string     `json:"external_id"`
	PairedAt   time.Time  `json:"paired_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

type ChannelSession struct {
	ID         string    `json:"id"`
	AccountID  string    `json:"account_id"`
	ExternalID string    `json:"external_id"`
	Summary    string    `json:"summary"`
	ExpiresAt  time.Time `json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ChannelMessage struct {
	ID             int64            `json:"id"`
	AccountID      string           `json:"account_id"`
	ExternalID     string           `json:"external_id"`
	Direction      string           `json:"direction"`
	PlatformID     string           `json:"platform_id"`
	IdempotencyKey string           `json:"idempotency_key"`
	Text           string           `json:"text"`
	Attachments    []map[string]any `json:"attachments,omitempty"`
	State          string           `json:"state"`
	Error          string           `json:"error,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	DeliveredAt    *time.Time       `json:"delivered_at,omitempty"`
	Attempts       int              `json:"attempts,omitempty"`
	NextAttemptAt  *time.Time       `json:"next_attempt_at,omitempty"`
}

type AgentTurn struct {
	ID             string           `json:"id"`
	RunID          string           `json:"run_id"`
	SessionID      string           `json:"session_id"`
	ExternalID     string           `json:"external_id"`
	Role           string           `json:"role"`
	IdempotencyKey string           `json:"idempotency_key"`
	Prompt         string           `json:"prompt"`
	History        []map[string]any `json:"history"`
	State          string           `json:"state"`
	Final          string           `json:"final,omitempty"`
	ProviderID     string           `json:"provider_id"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

type AgentAction struct {
	ID         string         `json:"id"`
	TurnID     string         `json:"turn_id"`
	RunID      string         `json:"run_id"`
	Tool       string         `json:"tool"`
	Role       string         `json:"role"`
	Params     map[string]any `json:"params"`
	Digest     string         `json:"digest"`
	State      string         `json:"state"`
	Result     map[string]any `json:"result,omitempty"`
	Error      string         `json:"error,omitempty"`
	ApprovedBy string         `json:"approved_by,omitempty"`
	DecidedAt  *time.Time     `json:"decided_at,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

type ManagedArtifact struct {
	ID          string    `json:"id"`
	RunID       string    `json:"run_id,omitempty"`
	OwnerType   string    `json:"owner_type"`
	OwnerID     string    `json:"owner_id"`
	Key         string    `json:"key"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	Digest      string    `json:"digest"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}

type Event struct {
	ID        int64          `json:"id"`
	RunID     string         `json:"run_id"`
	Type      string         `json:"type"`
	Message   string         `json:"message"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
}

type Deployment struct {
	RunID        string    `json:"run_id"`
	ProjectName  string    `json:"project_name"`
	PublicURL    string    `json:"public_url,omitempty"`
	Status       string    `json:"status"`
	ErrorMessage string    `json:"error_message,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Job struct {
	ID             int64      `json:"id"`
	RunID          string     `json:"run_id"`
	Stage          Stage      `json:"stage"`
	Attempt        int        `json:"attempt"`
	WorkerID       string     `json:"worker_id,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
}

type ApprovalAction string

const (
	ApprovalApprove ApprovalAction = "approve"
	ApprovalRevise  ApprovalAction = "revise"
	ApprovalRetry   ApprovalAction = "retry"
	ApprovalAbandon ApprovalAction = "abandon"
	ApprovalFix     ApprovalAction = "fix"
)
