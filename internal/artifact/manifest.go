package artifact

import (
	"time"

	"github.com/gongahkia/gator/internal/action"
)

// Status is the terminal state represented by a sealed artifact manifest.
type Status string

const (
	Completed Status = "completed"
	Failed    Status = "failed"
)

// Manifest is trusted, machine-readable evidence about one work run. It is
// generated from filesystem and validator state after model execution.
type Manifest struct {
	Version        int                `json:"version"`
	RunID          string             `json:"run_id"`
	Workflow       string             `json:"workflow"`
	Objective      string             `json:"objective"`
	ContractSHA256 string             `json:"contract_sha256"`
	Source         Source             `json:"source"`
	Artifacts      []File             `json:"artifacts"`
	Validations    []ValidationResult `json:"validations"`
	Actions        []action.Record    `json:"actions,omitempty"`
	Status         Status             `json:"status"`
	StartedAt      time.Time          `json:"started_at"`
	FinishedAt     time.Time          `json:"finished_at"`
}

// Source identifies the developer-selected input without claiming that every
// byte was snapshotted. SnapshotSHA256 is set only by a future explicit source
// snapshot mode.
type Source struct {
	Kind           string `json:"kind"`
	Path           string `json:"path"`
	SnapshotSHA256 string `json:"snapshot_sha256,omitempty"`
}

// File is an observed regular output file.
type File struct {
	Path      string `json:"path"`
	MediaType string `json:"media_type"`
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
}

// ValidationResult is one trusted deterministic check. Diagnostic is bounded
// and intended for review; it must not include full artifact contents.
type ValidationResult struct {
	Path       string         `json:"path"`
	Kind       ValidationKind `json:"kind"`
	Passed     bool           `json:"passed"`
	Diagnostic string         `json:"diagnostic,omitempty"`
}
