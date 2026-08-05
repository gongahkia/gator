package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
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

type Architecture struct {
	AppName          string             `json:"app_name"`
	AppType          string             `json:"app_type"`
	Stack            []string           `json:"stack"`
	Integrations     []string           `json:"integrations"`
	CoreFeatures     []Feature          `json:"core_features"`
	OptionalFeatures []Feature          `json:"optional_features"`
	Workflow         Graph              `json:"workflow"`
	Acceptance       AcceptanceContract `json:"acceptance"`
}

// acceptance contract is the operator-editable semantic contract compiled from
// the approved architecture. It is immutable once planner approval completes.
type AcceptanceContract struct {
	Version       int                     `json:"version"`
	Flows         []AcceptanceFlow        `json:"flows"`
	APIContracts  []APIContract           `json:"api_contracts"`
	SeedData      []SeedData              `json:"seed_data"`
	Accessibility []AccessibilityCheck    `json:"accessibility"`
	Screenshots   []ScreenshotExpectation `json:"screenshots"`
}

type AcceptanceFlow struct {
	ID    string           `json:"id"`
	Name  string           `json:"name"`
	Steps []AcceptanceStep `json:"steps"`
}

// supported step kinds are goto, click, fill, set_value, expect_text,
// expect_value,
// expect_visible, expect_count, expect_attribute, expect_url, reload, focus,
// press_key, and local_storage. Targets use selector or role/name; press_key
// may omit a target to use the active page element. local_storage supports
// clear or keyed set operations.
type AcceptanceStep struct {
	Kind      string `json:"kind"`
	Op        string `json:"op,omitempty"`
	Role      string `json:"role,omitempty"`
	Name      string `json:"name,omitempty"`
	Selector  string `json:"selector,omitempty"`
	Text      string `json:"text,omitempty"`
	URL       string `json:"url,omitempty"`
	Value     string `json:"value,omitempty"`
	Key       string `json:"key,omitempty"`
	Attribute string `json:"attribute,omitempty"`
	Count     *int   `json:"count,omitempty"`
}

type APIContract struct {
	ID           string `json:"id"`
	Method       string `json:"method"`
	Path         string `json:"path"`
	Status       int    `json:"status"`
	BodyIncludes string `json:"body_includes,omitempty"`
}

type SeedData struct {
	Kind   string `json:"kind"`
	Key    string `json:"key,omitempty"`
	Value  string `json:"value,omitempty"`
	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
	Body   string `json:"body,omitempty"`
}

type AccessibilityCheck struct {
	ID       string `json:"id"`
	FlowID   string `json:"flow_id,omitempty"`
	Selector string `json:"selector,omitempty"`
}

type ScreenshotExpectation struct {
	ID     string `json:"id"`
	FlowID string `json:"flow_id,omitempty"`
	Path   string `json:"path,omitempty"`
}

type Feature struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Role        string `json:"role"`
	Selected    bool   `json:"selected"`
}

func DefaultArchitecture(profile Profile, graph Graph) Architecture {
	appType := string(profile)
	return Architecture{
		AppName: "Generated app", AppType: appType,
		CoreFeatures:     []Feature{{ID: "core-request", Name: "Requested application", Description: "Deliver the approved user request.", Role: "app_logic", Selected: true}},
		OptionalFeatures: []Feature{}, Stack: []string{}, Integrations: []string{}, Workflow: graph,
	}
}

func (a Architecture) Validate() error {
	if a.AppName == "" || a.AppType == "" {
		return fmt.Errorf("architecture requires app_name and app_type")
	}
	if err := a.Workflow.Validate(); err != nil {
		return fmt.Errorf("architecture workflow: %w", err)
	}
	for _, feature := range append(append([]Feature{}, a.CoreFeatures...), a.OptionalFeatures...) {
		if feature.ID == "" || feature.Name == "" || feature.Role == "" {
			return fmt.Errorf("architecture features require id, name, and role")
		}
	}
	if err := a.Acceptance.Validate(); err != nil {
		return fmt.Errorf("architecture acceptance: %w", err)
	}
	return nil
}

// ValidateArchitectureForProfile rejects incomplete planner contracts while
// retaining Architecture.Validate for pre-planner draft state.
func ValidateArchitectureForProfile(profile Profile, architecture Architecture) error {
	if err := architecture.Validate(); err != nil {
		return err
	}
	if architecture.AppType != string(profile) {
		return fmt.Errorf("architecture app_type %q does not match profile %q", architecture.AppType, profile)
	}
	if len(architecture.Stack) == 0 {
		return fmt.Errorf("architecture requires a non-empty stack")
	}
	selected := 0
	placeholderOnly := len(architecture.CoreFeatures) == 1 && architecture.CoreFeatures[0].ID == "core-request"
	for _, feature := range architecture.CoreFeatures {
		if feature.Selected {
			selected++
		}
	}
	if selected == 0 || placeholderOnly {
		return fmt.Errorf("architecture requires concrete selected core features")
	}
	if architecture.Acceptance.Empty() || len(architecture.Acceptance.Flows) == 0 {
		return fmt.Errorf("architecture requires executable acceptance flows")
	}
	return ValidateAcceptanceForProfile(profile, architecture.Acceptance)
}

func CompileAcceptance(architecture Architecture) AcceptanceContract {
	flows := make([]AcceptanceFlow, 0, len(architecture.CoreFeatures))
	for _, feature := range architecture.CoreFeatures {
		if !feature.Selected {
			continue
		}
		flows = append(flows, AcceptanceFlow{ID: feature.ID, Name: feature.Name, Steps: []AcceptanceStep{{Kind: "goto", URL: "/"}, {Kind: "expect_text", Text: feature.Name}}})
	}
	return AcceptanceContract{Version: 1, Flows: flows, APIContracts: []APIContract{}, SeedData: []SeedData{}, Accessibility: []AccessibilityCheck{{ID: "home"}}, Screenshots: []ScreenshotExpectation{{ID: "home", Path: "/"}}}
}

func (c AcceptanceContract) Empty() bool {
	return c.Version == 0 && len(c.Flows) == 0 && len(c.APIContracts) == 0 && len(c.SeedData) == 0 && len(c.Accessibility) == 0 && len(c.Screenshots) == 0
}

func (c AcceptanceContract) Validate() error {
	if c.Empty() {
		return nil
	}
	if c.Version != 1 {
		return fmt.Errorf("acceptance version must be 1")
	}
	if len(c.Flows) > 20 || len(c.APIContracts) > 20 || len(c.SeedData) > 50 || len(c.Accessibility) > 20 || len(c.Screenshots) > 10 {
		return fmt.Errorf("acceptance contract exceeds execution limits")
	}
	seen := map[string]struct{}{}
	for _, flow := range c.Flows {
		if flow.ID == "" || flow.Name == "" || len(flow.Steps) == 0 {
			return fmt.Errorf("acceptance flows require id, name, and steps")
		}
		if len(flow.ID) > 128 || len(flow.Name) > 512 || len(flow.Steps) > 40 {
			return fmt.Errorf("acceptance flow exceeds execution limits")
		}
		if _, ok := seen[flow.ID]; ok {
			return fmt.Errorf("duplicate acceptance flow %q", flow.ID)
		}
		seen[flow.ID] = struct{}{}
		for _, step := range flow.Steps {
			if err := step.Validate(); err != nil {
				return fmt.Errorf("flow %q: %w", flow.ID, err)
			}
		}
	}
	apiIDs := map[string]struct{}{}
	for _, api := range c.APIContracts {
		if api.ID == "" || len(api.ID) > 128 || !validHTTPMethod(api.Method) || !validLocalPath(api.Path) || api.Status < 100 || api.Status > 599 || len(api.BodyIncludes) > 8192 {
			return fmt.Errorf("invalid API contract")
		}
		if _, ok := apiIDs[api.ID]; ok {
			return fmt.Errorf("duplicate API contract %q", api.ID)
		}
		apiIDs[api.ID] = struct{}{}
	}
	for _, seed := range c.SeedData {
		if seed.Kind == "local_storage" && (seed.Key == "" || seed.Value == "" || len(seed.Key) > 512 || len(seed.Value) > 64<<10) {
			return fmt.Errorf("local_storage seed requires key and value")
		}
		if seed.Kind == "http" && (!validHTTPMethod(seed.Method) || !validLocalPath(seed.Path) || len(seed.Body) > 64<<10) {
			return fmt.Errorf("http seed requires method and absolute path")
		}
		if seed.Kind != "local_storage" && seed.Kind != "http" {
			return fmt.Errorf("unsupported seed kind %q", seed.Kind)
		}
	}
	accessibilityIDs := map[string]struct{}{}
	for _, item := range c.Accessibility {
		if item.ID == "" || len(item.ID) > 128 || len(item.Selector) > 4096 {
			return fmt.Errorf("acceptance artifact ids are required")
		}
		if _, ok := accessibilityIDs[item.ID]; ok {
			return fmt.Errorf("duplicate accessibility check %q", item.ID)
		}
		accessibilityIDs[item.ID] = struct{}{}
		if item.FlowID != "" {
			if _, ok := seen[item.FlowID]; !ok {
				return fmt.Errorf("acceptance flow %q does not exist", item.FlowID)
			}
		}
	}
	screenshotIDs := map[string]struct{}{}
	for _, item := range c.Screenshots {
		if item.ID == "" || len(item.ID) > 128 || item.Path != "" && !validLocalPath(item.Path) {
			return fmt.Errorf("invalid screenshot expectation")
		}
		if _, ok := screenshotIDs[item.ID]; ok {
			return fmt.Errorf("duplicate screenshot expectation %q", item.ID)
		}
		screenshotIDs[item.ID] = struct{}{}
		if item.FlowID != "" {
			if _, ok := seen[item.FlowID]; !ok {
				return fmt.Errorf("acceptance flow %q does not exist", item.FlowID)
			}
		}
	}
	return nil
}

func (s AcceptanceStep) Validate() error {
	if len(s.Op) > 32 || len(s.Role) > 512 || len(s.Name) > 4096 || len(s.Selector) > 4096 || len(s.Text) > 8192 || len(s.URL) > 4096 || len(s.Value) > 64<<10 || len(s.Key) > 512 || len(s.Attribute) > 512 {
		return fmt.Errorf("acceptance step exceeds execution limits")
	}
	hasTarget := s.Selector != "" || s.Role != "" && s.Name != ""
	if s.Selector == "" && (s.Role == "") != (s.Name == "") {
		return fmt.Errorf("acceptance role and name must be supplied together")
	}
	switch s.Kind {
	case "goto":
		if !validLocalPath(s.URL) {
			return fmt.Errorf("goto requires a local absolute URL")
		}
	case "click", "fill", "set_value", "expect_visible", "focus":
		if !hasTarget {
			return fmt.Errorf("%s requires selector or role and name", s.Kind)
		}
		if (s.Kind == "fill" || s.Kind == "set_value") && s.Value == "" {
			return fmt.Errorf("%s requires value", s.Kind)
		}
	case "expect_text":
		if s.Text == "" {
			return fmt.Errorf("expect_text requires text")
		}
	case "expect_value":
		if !hasTarget {
			return fmt.Errorf("expect_value requires selector or role and name")
		}
	case "expect_count":
		if !hasTarget || s.Count == nil || *s.Count < 0 {
			return fmt.Errorf("expect_count requires target and non-negative count")
		}
	case "expect_attribute":
		attribute := s.Attribute
		if attribute == "" && s.Selector != "" {
			attribute = s.Name // support pre-schema planner responses
		}
		if !hasTarget || attribute == "" {
			return fmt.Errorf("expect_attribute requires target and attribute")
		}
	case "expect_url":
		if !validLocalPath(s.URL) {
			return fmt.Errorf("expect_url requires a local absolute URL")
		}
	case "reload":
	case "press_key":
		if s.Key == "" {
			return fmt.Errorf("press_key requires key")
		}
	case "local_storage":
		switch s.Op {
		case "", "set":
			if s.Key == "" || s.Value == "" {
				return fmt.Errorf("local_storage set requires key and value")
			}
		case "clear":
			if s.Key != "" || s.Value != "" {
				return fmt.Errorf("local_storage clear does not accept key or value")
			}
		default:
			return fmt.Errorf("unsupported local_storage operation %q", s.Op)
		}
	default:
		return fmt.Errorf("unsupported step kind %q", s.Kind)
	}
	return nil
}

func ValidateAcceptanceForProfile(profile Profile, contract AcceptanceContract) error {
	if err := contract.Validate(); err != nil {
		return err
	}
	if profile == ProfileFrontend && (len(contract.APIContracts) > 0 || hasHTTPSeed(contract.SeedData)) {
		return fmt.Errorf("frontend-only profile cannot declare backend API acceptance checks")
	}
	return nil
}

func validHTTPMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
		return true
	}
	return false
}

func validLocalPath(path string) bool {
	return strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//")
}

func hasHTTPSeed(seeds []SeedData) bool {
	for _, seed := range seeds {
		if seed.Kind == "http" {
			return true
		}
	}
	return false
}

func (c AcceptanceContract) Digest() string {
	encoded, _ := json.Marshal(c)
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
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
	ID                 string           `json:"id"`
	AppID              string           `json:"app_id"`
	ParentRunID        string           `json:"parent_run_id,omitempty"`
	BaseSnapshotDigest string           `json:"base_snapshot_digest,omitempty"`
	Prompt             string           `json:"prompt"`
	Profile            Profile          `json:"profile"`
	DeploymentTarget   DeploymentTarget `json:"deployment_target"`
	PublicIngress      bool             `json:"public_ingress"`
	MaxFixes           int              `json:"max_fixes"`
	Stage              Stage            `json:"stage"`
	Status             Status           `json:"status"`
	WorkspaceStatus    string           `json:"workspace_status"`
	Providers          map[Stage]string `json:"providers"`
	Graph              Graph            `json:"graph"`
	Architecture       Architecture     `json:"architecture"`
	Feedback           string           `json:"feedback,omitempty"`
	FailureReason      string           `json:"failure_reason,omitempty"`
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

// RunAgentPolicy is an immutable-at-creation capability ceiling with operator-only
// monotonic restrictions recorded as later versions.
type RunAgentPolicy struct {
	RunID     string                        `json:"run_id"`
	Version   int                           `json:"version"`
	Stages    map[Stage]InternalAgentPolicy `json:"stages"`
	Tools     map[string]AgentToolPolicy    `json:"tools"`
	Digest    string                        `json:"digest"`
	UpdatedBy string                        `json:"updated_by,omitempty"`
	CreatedAt time.Time                     `json:"created_at"`
	UpdatedAt time.Time                     `json:"updated_at"`
}

// InternalAgentPolicy controls the model/runtime used by a workflow stage.
// A false value is a denial; operators may only turn capabilities off per run.
type InternalAgentPolicy struct {
	Enabled      bool `json:"enabled"`
	AllowModel   bool `json:"allow_model"`
	AllowNetwork bool `json:"allow_network"`
	AllowCLI     bool `json:"allow_cli"`
}

// AgentToolPolicy controls central-agent functions for an agentic app.
// Empty allowlists mean no values are permitted, except allowed_path_prefixes
// where an empty list retains the manifest's safe-relative-path ceiling.
type AgentToolPolicy struct {
	Enabled             bool     `json:"enabled"`
	ApprovalRequired    bool     `json:"approval_required"`
	Roles               []string `json:"roles"`
	AllowedHosts        []string `json:"allowed_hosts"`
	AllowedCommands     []string `json:"allowed_commands"`
	AllowedPathPrefixes []string `json:"allowed_path_prefixes"`
	MaxCalls            int      `json:"max_calls"`
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
	TraceID        string            `json:"trace_id,omitempty"`
	SpanID         string            `json:"span_id,omitempty"`
	Traceparent    string            `json:"traceparent,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	ApprovedAt     *time.Time        `json:"approved_at,omitempty"`
}

type AcceptanceBaseline struct {
	AppID          string     `json:"app_id"`
	ContractDigest string     `json:"contract_digest"`
	ScreenshotID   string     `json:"screenshot_id"`
	RunID          string     `json:"run_id"`
	State          string     `json:"state"`
	Digest         string     `json:"digest"`
	PNG            []byte     `json:"-"`
	CreatedAt      time.Time  `json:"created_at"`
	ApprovedAt     *time.Time `json:"approved_at,omitempty"`
}

type OutboxEvent struct {
	ID             int64          `json:"id"`
	RunID          string         `json:"run_id,omitempty"`
	Type           string         `json:"type"`
	Payload        map[string]any `json:"payload"`
	State          string         `json:"state"`
	Attempts       int            `json:"attempts"`
	WorkerID       string         `json:"worker_id,omitempty"`
	LastError      string         `json:"last_error,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	NextAttemptAt  *time.Time     `json:"next_attempt_at,omitempty"`
	DeliveredAt    *time.Time     `json:"delivered_at,omitempty"`
	DeadLetteredAt *time.Time     `json:"dead_lettered_at,omitempty"`
}

type ApprovalOperation struct {
	ID             int64             `json:"id"`
	RunID          string            `json:"run_id"`
	RevisionID     int64             `json:"revision_id"`
	BaselineDigest string            `json:"baseline_digest"`
	PostDigest     string            `json:"post_digest"`
	BaselineFiles  map[string]string `json:"baseline_files,omitempty"`
	State          string            `json:"state"`
	Error          string            `json:"error,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	CompletedAt    *time.Time        `json:"completed_at,omitempty"`
	TraceID        string            `json:"trace_id,omitempty"`
	SpanID         string            `json:"span_id,omitempty"`
	Traceparent    string            `json:"traceparent,omitempty"`
}

type PlannerRevision struct {
	ID           int64        `json:"id"`
	RunID        string       `json:"run_id"`
	Attempt      int          `json:"attempt"`
	Source       string       `json:"source"`
	ParentID     *int64       `json:"parent_id,omitempty"`
	Architecture Architecture `json:"architecture"`
	Graph        Graph        `json:"graph"`
	Digest       string       `json:"digest"`
	Diff         []any        `json:"diff"`
	State        string       `json:"state"`
	CreatedAt    time.Time    `json:"created_at"`
	ApprovedAt   *time.Time   `json:"approved_at,omitempty"`
	TraceID      string       `json:"trace_id,omitempty"`
	SpanID       string       `json:"span_id,omitempty"`
	Traceparent  string       `json:"traceparent,omitempty"`
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
	TraceID      string         `json:"trace_id,omitempty"`
	SpanID       string         `json:"span_id,omitempty"`
	Traceparent  string         `json:"traceparent,omitempty"`
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
	BundlePath    string         `json:"bundle_path"`
	Mode          string         `json:"mode"`
	Digest        string         `json:"digest,omitempty"`
	State         string         `json:"state"`
	ResolvedRef   string         `json:"resolved_ref,omitempty"`
	TreeDigest    string         `json:"tree_digest,omitempty"`
	TrustLevel    string         `json:"trust_level"`
	ReviewedBy    string         `json:"reviewed_by,omitempty"`
	ReviewedAt    *time.Time     `json:"reviewed_at,omitempty"`
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
	ID              string            `json:"id"`
	RunID           string            `json:"run_id"`
	Adapter         string            `json:"adapter"`
	Name            string            `json:"name"`
	OwnerExternalID string            `json:"owner_external_id"`
	SecretRefs      map[string]string `json:"secret_refs"`
	Settings        map[string]any    `json:"settings"`
	Enabled         bool              `json:"enabled"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type ChannelSession struct {
	ID         string    `json:"id"`
	AccountID  string    `json:"account_id"`
	ExternalID string    `json:"external_id"`
	ReplyID    string    `json:"reply_id"`
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
	TraceID        string           `json:"trace_id,omitempty"`
	SpanID         string           `json:"span_id,omitempty"`
	Traceparent    string           `json:"traceparent,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}

type AgentAction struct {
	ID              string         `json:"id"`
	TurnID          string         `json:"turn_id"`
	RunID           string         `json:"run_id"`
	Tool            string         `json:"tool"`
	Role            string         `json:"role"`
	Params          map[string]any `json:"params"`
	Digest          string         `json:"digest"`
	State           string         `json:"state"`
	Result          map[string]any `json:"result,omitempty"`
	Error           string         `json:"error,omitempty"`
	ApprovedBy      string         `json:"approved_by,omitempty"`
	DecidedAt       *time.Time     `json:"decided_at,omitempty"`
	ExpiresAt       *time.Time     `json:"expires_at,omitempty"`
	ApprovalContext map[string]any `json:"approval_context,omitempty"`
	TraceID         string         `json:"trace_id,omitempty"`
	SpanID          string         `json:"span_id,omitempty"`
	Traceparent     string         `json:"traceparent,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
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

type SandboxExecution struct {
	ID          string     `json:"id"`
	ActionID    string     `json:"action_id,omitempty"`
	RunID       string     `json:"run_id"`
	Target      string     `json:"target"`
	Tool        string     `json:"tool"`
	State       string     `json:"state"`
	ExitCode    int        `json:"exit_code"`
	Output      string     `json:"output,omitempty"`
	TraceID     string     `json:"trace_id,omitempty"`
	SpanID      string     `json:"span_id,omitempty"`
	Traceparent string     `json:"traceparent,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type TraceEvent struct {
	ID           int64             `json:"id"`
	RunID        string            `json:"run_id"`
	Type         string            `json:"type"`
	Severity     string            `json:"severity"`
	Status       string            `json:"status,omitempty"`
	Summary      string            `json:"summary"`
	Stage        string            `json:"stage,omitempty"`
	Attempt      int               `json:"attempt,omitempty"`
	ProviderID   string            `json:"provider_id,omitempty"`
	RevisionID   int64             `json:"revision_id,omitempty"`
	ApprovalID   int64             `json:"approval_id,omitempty"`
	TurnID       string            `json:"turn_id,omitempty"`
	ActionID     string            `json:"action_id,omitempty"`
	SandboxID    string            `json:"sandbox_id,omitempty"`
	Actor        string            `json:"actor,omitempty"`
	TraceID      string            `json:"trace_id,omitempty"`
	SpanID       string            `json:"span_id,omitempty"`
	Traceparent  string            `json:"traceparent,omitempty"`
	EntityRefs   map[string]string `json:"entity_refs,omitempty"`
	Payload      map[string]any    `json:"payload,omitempty"`
	RawAvailable bool              `json:"raw_available,omitempty"`
	OccurredAt   time.Time         `json:"occurred_at"`
	RecordedAt   time.Time         `json:"recorded_at"`
}

type TracePage struct {
	Items      []TraceEvent `json:"items"`
	NextCursor string       `json:"next_cursor,omitempty"`
}
type TraceFilter struct {
	Cursor, Stage, Type, Severity, ProviderID, Tool, Actor, EntityID, Query string
	From, To                                                                *time.Time
}
type AgentTurnPage struct {
	Items      []AgentTurn `json:"items"`
	NextCursor string      `json:"next_cursor,omitempty"`
}
type RunAgentPolicyEvent struct {
	ID        int64          `json:"id"`
	RunID     string         `json:"run_id"`
	Version   int            `json:"version"`
	Policy    RunAgentPolicy `json:"policy"`
	Digest    string         `json:"digest"`
	Actor     string         `json:"actor"`
	CreatedAt time.Time      `json:"created_at"`
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
	AppID        string    `json:"app_id"`
	ProjectName  string    `json:"project_name"`
	PublicURL    string    `json:"public_url,omitempty"`
	Status       string    `json:"status"`
	ErrorMessage string    `json:"error_message,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type App struct {
	AppID       string     `json:"app_id"`
	RunID       string     `json:"run_id"`
	ParentRunID string     `json:"parent_run_id,omitempty"`
	Prompt      string     `json:"prompt"`
	Profile     Profile    `json:"profile"`
	RunStatus   Status     `json:"run_status"`
	Deployment  Deployment `json:"deployment"`
	CreatedAt   time.Time  `json:"created_at"`
}

type RunPage struct {
	Items      []Run  `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type AppPage struct {
	Items      []App  `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type EventPage struct {
	Items      []Event `json:"items"`
	NextCursor string  `json:"next_cursor,omitempty"`
}

type AppSnapshot struct {
	Digest      string    `json:"digest"`
	SourceRunID string    `json:"source_run_id"`
	AppID       string    `json:"app_id"`
	Path        string    `json:"path"`
	FileCount   int       `json:"file_count"`
	ByteCount   int64     `json:"byte_count"`
	CreatedAt   time.Time `json:"created_at"`
}

type Job struct {
	ID             int64      `json:"id"`
	RunID          string     `json:"run_id"`
	Stage          Stage      `json:"stage"`
	Attempt        int        `json:"attempt"`
	WorkerID       string     `json:"worker_id,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
	Traceparent    string     `json:"traceparent,omitempty"`
}

type ApprovalAction string

const (
	ApprovalApprove ApprovalAction = "approve"
	ApprovalRevise  ApprovalAction = "revise"
	ApprovalRetry   ApprovalAction = "retry"
	ApprovalAbandon ApprovalAction = "abandon"
	ApprovalFix     ApprovalAction = "fix"
)
