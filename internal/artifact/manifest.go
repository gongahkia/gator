package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/patch"
)

var specialistNamePattern = regexp.MustCompile(`\A[a-z][a-z0-9_]{0,63}\z`)

// Status is the terminal state represented by a sealed artifact manifest.
type Status string

const (
	Completed Status = "completed"
	Failed    Status = "failed"
)

// Manifest is trusted, machine-readable evidence about one work run. It is
// generated from filesystem and validator state after model execution.
type Evidence struct {
	ID           string    `json:"id"`
	Locator      string    `json:"locator"`
	SHA256       string    `json:"sha256"`
	SnapshotPath string    `json:"snapshot_path,omitempty"`
	RetrievedAt  time.Time `json:"retrieved_at"`
}

type Manifest struct {
	Usage            agent.Usage            `json:"usage"`
	Evidence         []Evidence             `json:"evidence,omitempty"`
	PolicySHA256     string                 `json:"policy_sha256,omitempty"`
	Candidates       []patch.Candidate      `json:"code_candidates,omitempty"`
	Version          int                    `json:"version"`
	RunID            string                 `json:"run_id"`
	Workflow         string                 `json:"workflow"`
	Objective        string                 `json:"objective"`
	Contract         Contract               `json:"contract"`
	ContractSHA256   string                 `json:"contract_sha256"`
	Source           Source                 `json:"source"`
	ConnectedSources []connector.Provenance `json:"connected_sources,omitempty"`
	Renderers        []RendererEvidence     `json:"renderers,omitempty"`
	Subagents        []SubagentEvidence     `json:"subagents,omitempty"`
	Artifacts        []File                 `json:"artifacts"`
	Validations      []ValidationResult     `json:"validations"`
	Actions          []action.Record        `json:"actions,omitempty"`
	Status           Status                 `json:"status"`
	Failure          string                 `json:"failure,omitempty"`
	StartedAt        time.Time              `json:"started_at"`
	FinishedAt       time.Time              `json:"finished_at"`
}

// Source identifies the developer-selected input and its immutable snapshot.
// Version-1 manifests predate SnapshotSHA256 and remain readable.
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

// RendererEvidence binds semantic input and an optional frozen template to
// the exact bytes emitted by a trusted rich-artifact renderer.
type RendererEvidence struct {
	Path           string `json:"path"`
	Format         string `json:"format"`
	Renderer       string `json:"renderer"`
	ArtifactSHA256 string `json:"artifact_sha256"`
	SpecSHA256     string `json:"spec_sha256"`
	TemplateSHA256 string `json:"template_sha256,omitempty"`
}

// SubagentEvidence records a bounded fresh-context specialist invocation. It
// contains digests and lifecycle metadata rather than the specialist's full
// private transcript. Code specialists can additionally bind a patch artifact
// to its exact output bytes.
type SubagentEvidence struct {
	BaselineSHA256 string `json:"baseline_sha256,omitempty"`
	Usage          agent.Usage `json:"usage"`
	ID             string    `json:"id"`
	Agent          string    `json:"agent"`
	TaskSHA256     string    `json:"task_sha256"`
	OutputSHA256   string    `json:"output_sha256"`
	Status         string    `json:"status"`
	Steps          int       `json:"steps,omitempty"`
	ArtifactPath   string    `json:"artifact_path,omitempty"`
	ArtifactSHA256 string    `json:"artifact_sha256,omitempty"`
	StartedAt      time.Time `json:"started_at"`
	FinishedAt     time.Time `json:"finished_at"`
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
	if (m.Version < 1 || m.Version > ManifestVersion) || m.Workflow != "work" {
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
	if m.Version >= 2 && !validSHA256(m.Source.SnapshotSHA256) {
		return errors.New("artifact manifest v2 requires a source snapshot digest")
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
	artifactDigests := make(map[string]string, len(m.Artifacts))
	for _, file := range m.Artifacts {
		if err := validateArtifactPath(file.Path); err != nil || strings.TrimSpace(file.MediaType) == "" || file.Bytes < 0 || !validSHA256(file.SHA256) {
			return fmt.Errorf("artifact manifest file %q is invalid", file.Path)
		}
		if _, duplicate := seen[file.Path]; duplicate {
			return fmt.Errorf("artifact manifest repeats file %q", file.Path)
		}
		seen[file.Path] = struct{}{}
		artifactDigests[file.Path] = file.SHA256
	}
	seenRenderers := make(map[string]struct{}, len(m.Renderers))
	if m.PolicySHA256 != "" && !validSHA256(m.PolicySHA256) {
		return errors.New("artifact policy digest is invalid")
	}
	for _, candidate := range m.Candidates {
		if candidate.Version != 1 || (candidate.Status != "verified" && candidate.Status != "failed") {
			return errors.New("invalid code candidate version or status")
		}
		if candidate.SHA256 != "" && artifactDigests[candidate.PatchPath] != candidate.SHA256 {
			return errors.New("code candidate patch does not match retained output")
		}
		if candidate.Status == "verified" {
			if !validSHA256(candidate.SHA256) || candidate.Error != "" || len(candidate.Verification) == 0 {
				return errors.New("verified code candidate lacks verification evidence")
			}
			for _, check := range candidate.Verification {
				if !check.Passed || len(check.Argv) == 0 {
					return errors.New("verified code candidate contains failed verification")
				}
			}
			for _, path := range candidate.ChangedPaths {
				if err := validateArtifactPath(path); err != nil || (candidate.Before[path] != "absent" && !validSHA256(candidate.Before[path])) {
					return errors.New("code candidate changed-path baseline is invalid")
				}
			}
		}
	}
	for _, renderer := range m.Renderers {
		if err := renderer.Validate(); err != nil {
			return fmt.Errorf("artifact renderer evidence: %w", err)
		}
		if digest, ok := artifactDigests[renderer.Path]; !ok || digest != renderer.ArtifactSHA256 {
			return fmt.Errorf("artifact renderer evidence for %q does not match output", renderer.Path)
		}
		if _, duplicate := seenRenderers[renderer.Path]; duplicate {
			return fmt.Errorf("artifact manifest repeats renderer evidence for %q", renderer.Path)
		}
		seenRenderers[renderer.Path] = struct{}{}
	}
	if len(m.Subagents) > 64 {
		return errors.New("artifact manifest has too many subagent records")
	}
	seenSubagents := make(map[string]struct{}, len(m.Subagents))
	for _, subagent := range m.Subagents {
		if err := subagent.Validate(); err != nil {
			return fmt.Errorf("artifact subagent evidence: %w", err)
		}
		if subagent.ArtifactPath != "" {
			if digest, ok := artifactDigests[subagent.ArtifactPath]; !ok || digest != subagent.ArtifactSHA256 {
				return fmt.Errorf("artifact subagent evidence for %q does not match output", subagent.ArtifactPath)
			}
		}
		if _, duplicate := seenSubagents[subagent.ID]; duplicate {
			return fmt.Errorf("artifact manifest repeats subagent evidence for %q", subagent.ID)
		}
		seenSubagents[subagent.ID] = struct{}{}
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
	actionsPassed := true
	for _, record := range m.Actions {
		if record.Status == action.Failed || record.Status == action.Unknown {
			actionsPassed = false
			break
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
	if m.Status == Completed && !actionsPassed {
		return errors.New("completed artifact manifest contains failed external action evidence")
	}
	if m.Status == Failed && m.Failure == "" && validationsPassed && actionsPassed {
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

func (e RendererEvidence) Validate() error {
	if err := validateArtifactPath(e.Path); err != nil {
		return err
	}
	if e.Format != "docx" && e.Format != "pdf" && e.Format != "xlsx" {
		return errors.New("renderer format is invalid")
	}
	if e.Renderer != "gator.document.v1" && e.Renderer != "gator.workbook.v1" {
		return errors.New("renderer identity is invalid")
	}
	if !validSHA256(e.ArtifactSHA256) || !validSHA256(e.SpecSHA256) || e.TemplateSHA256 != "" && !validSHA256(e.TemplateSHA256) {
		return errors.New("renderer digest is invalid")
	}
	return nil
}

// Validate checks the portable, bounded subagent evidence envelope.
func (e SubagentEvidence) Validate() error {
	if !runIDPattern.MatchString(e.ID) || !specialistNamePattern.MatchString(e.Agent) {
		return errors.New("subagent identity is invalid")
	}
	if !validSHA256(e.TaskSHA256) || !validSHA256(e.OutputSHA256) {
		return errors.New("subagent digest is invalid")
	}
	if e.Status != "completed" && e.Status != "failed" && e.Status != "cancelled" {
		return errors.New("subagent status is invalid")
	}
	if e.Steps < 0 || e.Steps > 1024 {
		return errors.New("subagent step count is invalid")
	}
	if (e.ArtifactPath == "") != (e.ArtifactSHA256 == "") {
		return errors.New("subagent artifact evidence is incomplete")
	}
	if e.ArtifactPath != "" {
		if err := validateArtifactPath(e.ArtifactPath); err != nil || !validSHA256(e.ArtifactSHA256) {
			return errors.New("subagent artifact evidence is invalid")
		}
	}
	if (e.StartedAt.IsZero() && e.Status != "cancelled") || e.FinishedAt.IsZero() || e.FinishedAt.Before(e.StartedAt) {
		return errors.New("subagent timestamps are invalid")
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
