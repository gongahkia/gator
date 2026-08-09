// Package core contains the UI-independent standalone Gator domain.
package core

import "time"

const (
	SchemaVersion       = 1
	StandaloneStatePath = ".gator/standalone/v1"
)

type Workspace struct {
	Root        string `json:"root"`
	Branch      string `json:"branch"`
	Head        string `json:"head"`
	Dirty       bool   `json:"dirty"`
	ChangeCount int    `json:"change_count"`
	Kind        string `json:"kind"` // project or worktree
}

type ProviderCapabilities struct {
	Terminal       bool `json:"terminal"`
	Structured     bool `json:"structured"`
	Resume         bool `json:"resume"`
	Fork           bool `json:"fork"`
	Approvals      bool `json:"approvals"`
	UsageReporting bool `json:"usage_reporting"`
	SessionList    bool `json:"session_list"`
}

type ProviderStatus struct {
	ID           string               `json:"id"`
	Available    bool                 `json:"available"`
	Version      string               `json:"version,omitempty"`
	Executable   string               `json:"executable"`
	Capabilities ProviderCapabilities `json:"capabilities"`
	Reason       string               `json:"reason,omitempty"`
}

type ContextEntry struct {
	Kind       string `json:"kind"` // file, range, git_diff, handoff
	Path       string `json:"path,omitempty"`
	StartLine  int    `json:"start_line,omitempty"`
	EndLine    int    `json:"end_line,omitempty"`
	Bytes      int    `json:"bytes"`
	SHA256     string `json:"sha256,omitempty"`
	Redactions int    `json:"redactions"`
}

// ContextManifest stores delivery metadata only. It never stores source text.
type ContextManifest struct {
	Entries     []ContextEntry `json:"entries"`
	TotalBytes  int            `json:"total_bytes"`
	Redactions  int            `json:"redactions"`
	PreparedAt  time.Time      `json:"prepared_at"`
	Description string         `json:"description,omitempty"`
}

type PolicyRef struct {
	SchemaVersion int    `json:"schema_version"`
	Version       string `json:"version"`
	Source        string `json:"source"`
}

type Usage struct {
	State        string `json:"state"` // unknown or reported
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	TotalTokens  int    `json:"total_tokens,omitempty"`
}

type Activity struct {
	State       string    `json:"state"` // active, stalled, inactive
	LastEventAt time.Time `json:"last_event_at,omitempty"`
	StalledAt   time.Time `json:"stalled_at,omitempty"`
}

type Verification struct {
	State       string   `json:"state"` // unknown, passed, failed
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
	DiffSHA256  string   `json:"diff_sha256,omitempty"`
	BaseSHA     string   `json:"base_sha,omitempty"`
}

type Run struct {
	SchemaVersion int             `json:"schema_version"`
	ID            string          `json:"id"`
	TaskID        string          `json:"task_id"`
	Objective     string          `json:"objective"`
	Provider      string          `json:"provider"`
	Role          string          `json:"role"`
	Transport     string          `json:"transport"`
	State         string          `json:"state"`
	Workspace     Workspace       `json:"workspace"`
	SessionID     string          `json:"session_id,omitempty"`
	ParentRunID   string          `json:"parent_run_id,omitempty"`
	Dependencies  []string        `json:"dependencies,omitempty"`
	Context       ContextManifest `json:"context"`
	Policy        PolicyRef       `json:"policy"`
	Usage         Usage           `json:"usage"`
	Activity      Activity        `json:"activity"`
	Verification  Verification    `json:"verification"`
	StartedAt     time.Time       `json:"started_at,omitempty"`
	FinishedAt    time.Time       `json:"finished_at,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	ProviderError string          `json:"provider_error,omitempty"`
}

type Event struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	RunID         string    `json:"run_id"`
	At            time.Time `json:"at"`
	Type          string    `json:"type"`
	Provider      string    `json:"provider,omitempty"`
	State         string    `json:"state,omitempty"`
	Message       string    `json:"message,omitempty"`
	ExitCode      *int      `json:"exit_code,omitempty"`
	EvidenceID    string    `json:"evidence_id,omitempty"`
}

type Evidence struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	RunID         string    `json:"run_id"`
	Kind          string    `json:"kind"` // verification_command, review, provider_failure
	CommandID     string    `json:"command_id,omitempty"`
	Argv          []string  `json:"argv,omitempty"`
	ExitCode      int       `json:"exit_code"`
	Passed        bool      `json:"passed"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	OutputSHA256  string    `json:"output_sha256,omitempty"`
	BaseSHA       string    `json:"base_sha,omitempty"`
	DiffSHA256    string    `json:"diff_sha256,omitempty"`
}

type Episode struct {
	SchemaVersion     int       `json:"schema_version"`
	ID                string    `json:"id"`
	RunID             string    `json:"run_id"`
	TaskFingerprint   string    `json:"task_fingerprint"`
	Provider          string    `json:"provider"`
	PolicyVersion     string    `json:"policy_version"`
	WorkflowTopology  string    `json:"workflow_topology"`
	StartingSHA       string    `json:"starting_sha"`
	EndingSHA         string    `json:"ending_sha,omitempty"`
	EndingDiffSHA256  string    `json:"ending_diff_sha256,omitempty"`
	DurationMillis    int64     `json:"duration_millis"`
	RunState          string    `json:"run_state"`
	VerificationState string    `json:"verification_state"`
	Usage             Usage     `json:"usage"`
	ObservedAt        time.Time `json:"observed_at"`
}

type ExperimentResult struct {
	SchemaVersion   int       `json:"schema_version"`
	ID              string    `json:"id"`
	Objective       string    `json:"objective"`
	Provider        string    `json:"provider"`
	BaselineRunID   string    `json:"baseline_run_id"`
	CandidateRunID  string    `json:"candidate_run_id"`
	BaselinePolicy  string    `json:"baseline_policy"`
	CandidatePolicy string    `json:"candidate_policy"`
	Verdict         string    `json:"verdict"` // candidate_better, baseline_better, tie, inconclusive
	Reason          string    `json:"reason"`
	CreatedAt       time.Time `json:"created_at"`
}
