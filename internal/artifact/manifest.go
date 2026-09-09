package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/connector"
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
	Version          int                    `json:"version"`
	RunID            string                 `json:"run_id"`
	Workflow         string                 `json:"workflow"`
	Objective        string                 `json:"objective"`
	Contract         Contract               `json:"contract"`
	ContractSHA256   string                 `json:"contract_sha256"`
	Source           Source                 `json:"source"`
	ConnectedSources []connector.Provenance `json:"connected_sources,omitempty"`
	Artifacts        []File                 `json:"artifacts"`
	Validations      []ValidationResult     `json:"validations"`
	Actions          []action.Record        `json:"actions,omitempty"`
	Status           Status                 `json:"status"`
	Failure          string                 `json:"failure,omitempty"`
	StartedAt        time.Time              `json:"started_at"`
	FinishedAt       time.Time              `json:"finished_at"`
}

// Source identifies the developer-selected input without claiming that every
// byte was snapshotted. SnapshotSHA256 is set only by a future explicit source
// snapshot mode.
type Source struct {
	Kind           string `json:"kind"`
	Name           string `json:"name"`
	IdentitySHA256 string `json:"identity_sha256"`
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

// Validate checks the portable manifest envelope before it is written or
// exported. It intentionally does not touch artifact bytes; Seal is the only
// constructor that establishes their evidence.
func (m Manifest) Validate() error {
	if m.Version != ManifestVersion || m.Workflow != "work" {
		return errors.New("artifact manifest has an unsupported version or workflow")
	}
	if !runIDPattern.MatchString(m.RunID) || strings.TrimSpace(m.Objective) == "" || len(m.Objective) > 64*1024 || strings.ContainsRune(m.Objective, 0) {
		return errors.New("artifact manifest identity is invalid")
	}
	if !validSHA256(m.ContractSHA256) {
		return errors.New("artifact manifest contract digest is invalid")
	}
	if err := m.Contract.Validate(); err != nil {
		return fmt.Errorf("artifact manifest contract: %w", err)
	}
	digest, err := m.Contract.Digest()
	if err != nil || digest != m.ContractSHA256 {
		return errors.New("artifact manifest contract does not match its digest")
	}
	if m.Source.Kind != "directory" || strings.TrimSpace(m.Source.Name) == "" || filepath.Base(m.Source.Name) != m.Source.Name || !validSHA256(m.Source.IdentitySHA256) {
		return errors.New("artifact manifest source is invalid")
	}
	if m.Source.SnapshotSHA256 != "" && !validSHA256(m.Source.SnapshotSHA256) {
		return errors.New("artifact manifest source snapshot digest is invalid")
	}
	if len(m.ConnectedSources) > 128 {
		return errors.New("artifact manifest has too many connected sources")
	}
	for index, source := range m.ConnectedSources {
		if err := source.Validate(); err != nil {
			return fmt.Errorf("artifact manifest connected source %d: %w", index+1, err)
		}
	}
	seen := make(map[string]struct{}, len(m.Artifacts))
	for _, file := range m.Artifacts {
		if err := validateArtifactPath(file.Path); err != nil || strings.TrimSpace(file.MediaType) == "" || file.Bytes < 0 || !validSHA256(file.SHA256) {
			return fmt.Errorf("artifact manifest file %q is invalid", file.Path)
		}
		if _, duplicate := seen[file.Path]; duplicate {
			return fmt.Errorf("artifact manifest repeats file %q", file.Path)
		}
		seen[file.Path] = struct{}{}
	}
	for _, result := range m.Validations {
		if err := validateArtifactPath(result.Path); err != nil {
			return fmt.Errorf("artifact manifest validation path %q is invalid", result.Path)
		}
		if _, known := validationKinds[result.Kind]; !known {
			return fmt.Errorf("artifact manifest validation kind %q is invalid", result.Kind)
		}
		if len(result.Diagnostic) > maxDiagnosticBytes || strings.ContainsRune(result.Diagnostic, 0) || (result.Passed && result.Diagnostic != "") {
			return errors.New("artifact manifest validation diagnostic is invalid")
		}
	}
	validationsPassed := true
	for _, result := range m.Validations {
		if !result.Passed {
			validationsPassed = false
			break
		}
	}
	for index, record := range m.Actions {
		if err := record.Validate(); err != nil {
			return fmt.Errorf("artifact manifest action %d: %w", index+1, err)
		}
	}
	if m.Status != Completed && m.Status != Failed {
		return fmt.Errorf("artifact manifest status %q is invalid", m.Status)
	}
	if m.Status == Completed && m.Failure != "" {
		return errors.New("completed artifact manifest must not include a failure")
	}
	if m.Status == Completed && !validationsPassed {
		return errors.New("completed artifact manifest contains failed validation evidence")
	}
	if m.Status == Failed && m.Failure == "" && validationsPassed {
		return errors.New("failed artifact manifest has no failed evidence")
	}
	if m.Failure != "" && (strings.TrimSpace(m.Failure) != m.Failure || len(m.Failure) > maxDiagnosticBytes || strings.ContainsRune(m.Failure, 0) || !utf8.ValidString(m.Failure)) {
		return errors.New("artifact manifest failure is invalid")
	}
	if m.StartedAt.IsZero() || m.FinishedAt.IsZero() || m.FinishedAt.Before(m.StartedAt) {
		return errors.New("artifact manifest timestamps are invalid")
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
